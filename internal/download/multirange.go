package download

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
	"github.com/kevinyuzekai/LinkPool/internal/proxy"
	"github.com/kevinyuzekai/LinkPool/internal/scheduler"
)

// Status of a multi-range job.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusRunning    Status = "running"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
	StatusCanceled   Status = "canceled"
)

// Part describes one Range chunk.
type Part struct {
	Index     int    `json:"index"`
	Start     int64  `json:"start"`
	End       int64  `json:"end"` // inclusive
	Done      int64  `json:"done"`
	AdapterID string `json:"adapterId"`
	Error     string `json:"error,omitempty"`
}

// Job is a multi-connection Range download.
type Job struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	FileName    string  `json:"fileName"`
	FilePath    string  `json:"filePath,omitempty"`
	TotalSize   int64   `json:"totalSize"`
	Downloaded  int64   `json:"downloaded"`
	Concurrency int     `json:"concurrency"`
	Status      Status  `json:"status"`
	Error       string  `json:"error,omitempty"`
	Parts       []Part  `json:"parts"`
	CreatedAt   time.Time `json:"createdAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`

	mu     sync.Mutex
	cancel context.CancelFunc
}

// Snapshot returns a copy safe for JSON.
func (j *Job) Snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := Job{
		ID:          j.ID,
		URL:         j.URL,
		FileName:    j.FileName,
		FilePath:    j.FilePath,
		TotalSize:   j.TotalSize,
		Downloaded:  j.Downloaded,
		Concurrency: j.Concurrency,
		Status:      j.Status,
		Error:       j.Error,
		CreatedAt:   j.CreatedAt,
		FinishedAt:  j.FinishedAt,
	}
	out.Parts = make([]Part, len(j.Parts))
	copy(out.Parts, j.Parts)
	return out
}

// Manager runs multi-range downloads bound to scheduler NICs.
type Manager struct {
	Dialer *proxy.Dialer
	Sched  *scheduler.Scheduler
	Dir    string

	mu   sync.Mutex
	jobs map[string]*Job
	seq  atomic.Uint64
}

func NewManager(d *proxy.Dialer, sched *scheduler.Scheduler, dir string) *Manager {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "linkpool-downloads")
	}
	_ = os.MkdirAll(dir, 0o755)
	return &Manager{
		Dialer: d,
		Sched:  sched,
		Dir:    dir,
		jobs:   map[string]*Job{},
	}
}

// Start begins a multi-range download. concurrency <= 0 means 2× selected adapters (min 2).
func (m *Manager) Start(ctx context.Context, rawURL string, concurrency int) (*Job, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("需要 http(s) URL")
	}
	nAdapters := 1
	if m.Sched != nil {
		nAdapters = m.Sched.Len()
	}
	if nAdapters < 1 {
		nAdapters = 1
	}
	if concurrency <= 0 {
		concurrency = nAdapters * 2
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 32 {
		concurrency = 32
	}

	id := fmt.Sprintf("%d", m.seq.Add(1))
	name := filepath.Base(u.Path)
	if name == "" || name == "/" || name == "." {
		name = "download-" + id
	}
	// strip query junk from name
	if i := strings.IndexByte(name, '?'); i >= 0 {
		name = name[:i]
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job := &Job{
		ID:          id,
		URL:         rawURL,
		FileName:    name,
		Concurrency: concurrency,
		Status:      StatusQueued,
		CreatedAt:   time.Now(),
		cancel:      cancel,
	}
	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()

	go m.run(jobCtx, job)
	return job, nil
}

func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j.Snapshot())
	}
	return out
}

func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	j.mu.Lock()
	if j.cancel != nil {
		j.cancel()
	}
	if j.Status == StatusRunning || j.Status == StatusQueued {
		j.Status = StatusCanceled
	}
	j.mu.Unlock()
	return true
}

func (m *Manager) run(ctx context.Context, job *Job) {
	setFail := func(msg string) {
		job.mu.Lock()
		job.Status = StatusFailed
		job.Error = msg
		now := time.Now()
		job.FinishedAt = &now
		job.mu.Unlock()
	}

	job.mu.Lock()
	job.Status = StatusRunning
	job.mu.Unlock()

	size, acceptRanges, err := m.probe(ctx, job.URL)
	if err != nil {
		setFail(err.Error())
		return
	}

	outPath := filepath.Join(m.Dir, job.ID+"-"+sanitizeName(job.FileName))
	f, err := os.Create(outPath)
	if err != nil {
		setFail(err.Error())
		return
	}

	job.mu.Lock()
	job.TotalSize = size
	job.FilePath = outPath
	job.mu.Unlock()

	if size <= 0 || !acceptRanges || job.Concurrency <= 1 {
		// Single-stream fallback
		err := m.downloadSingle(ctx, job, f)
		_ = f.Close()
		if err != nil {
			setFail(err.Error())
			return
		}
		job.mu.Lock()
		job.Status = StatusDone
		job.Downloaded = job.TotalSize
		now := time.Now()
		job.FinishedAt = &now
		job.mu.Unlock()
		return
	}

	if err := f.Truncate(size); err != nil {
		_ = f.Close()
		setFail(err.Error())
		return
	}

	n := job.Concurrency
	chunk := size / int64(n)
	parts := make([]Part, n)
	for i := 0; i < n; i++ {
		start := int64(i) * chunk
		end := start + chunk - 1
		if i == n-1 {
			end = size - 1
		}
		parts[i] = Part{Index: i, Start: start, End: end}
	}
	job.mu.Lock()
	job.Parts = parts
	job.mu.Unlock()

	var wg sync.WaitGroup
	var failOnce sync.Once
	var firstErr error

	for i := range parts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := parts[idx]
			ad := m.pickAdapter(idx)
			adapterID := ""
			if ad.ID != "" {
				adapterID = ad.ID
			}
			job.mu.Lock()
			job.Parts[idx].AdapterID = adapterID
			job.mu.Unlock()

			nread, err := m.downloadPart(ctx, job.URL, f, p.Start, p.End, ad, func(dn int64) {
				job.mu.Lock()
				job.Parts[idx].Done = dn
				var sum int64
				for _, pp := range job.Parts {
					sum += pp.Done
				}
				job.Downloaded = sum
				job.mu.Unlock()
			})
			job.mu.Lock()
			job.Parts[idx].Done = nread
			if err != nil {
				job.Parts[idx].Error = err.Error()
			}
			job.mu.Unlock()
			if err != nil {
				failOnce.Do(func() { firstErr = err })
			}
		}(i)
	}
	wg.Wait()
	_ = f.Close()

	if ctx.Err() != nil {
		job.mu.Lock()
		job.Status = StatusCanceled
		now := time.Now()
		job.FinishedAt = &now
		job.mu.Unlock()
		return
	}
	if firstErr != nil {
		setFail(firstErr.Error())
		return
	}
	job.mu.Lock()
	job.Status = StatusDone
	job.Downloaded = size
	now := time.Now()
	job.FinishedAt = &now
	job.mu.Unlock()
}

func (m *Manager) pickAdapter(i int) adapter.Adapter {
	if m.Sched == nil || m.Sched.Len() == 0 {
		return adapter.Adapter{}
	}
	// Round-robin across entries for even NIC spread on parallel parts.
	entries := m.Sched.Entries()
	if len(entries) == 0 {
		return adapter.Adapter{}
	}
	return entries[i%len(entries)].Adapter
}

func (m *Manager) clientFor(ad adapter.Adapter) *http.Client {
	timeout := 60 * time.Second
	transport := &http.Transport{
		Proxy:                 nil,
		MaxIdleConnsPerHost:   2,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if ad.IPv4 != "" && m.Dialer != nil {
		a := ad
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return m.Dialer.DialBoundOn(ctx, network, address, a)
		}
	} else if m.Dialer != nil {
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			c, _, err := m.Dialer.DialContext(ctx, network, address)
			return c, err
		}
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func (m *Manager) probe(ctx context.Context, rawURL string) (size int64, acceptRanges bool, err error) {
	ad := m.pickAdapter(0)
	client := m.clientFor(ad)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			cl := resp.Header.Get("Content-Length")
			if cl != "" {
				size, _ = strconv.ParseInt(cl, 10, 64)
			}
			ar := strings.ToLower(resp.Header.Get("Accept-Ranges"))
			acceptRanges = strings.Contains(ar, "bytes") && size > 0
			if size > 0 {
				return size, acceptRanges, nil
			}
		}
	}

	// Fallback: Range probe
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, false, err
	}
	req2.Header.Set("Range", "bytes=0-0")
	resp2, err := client.Do(req2)
	if err != nil {
		return 0, false, err
	}
	defer resp2.Body.Close()
	io.Copy(io.Discard, resp2.Body)

	if resp2.StatusCode == http.StatusPartialContent {
		cr := resp2.Header.Get("Content-Range") // bytes 0-0/12345
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			size, _ = strconv.ParseInt(cr[i+1:], 10, 64)
		}
		return size, size > 0, nil
	}
	if cl := resp2.Header.Get("Content-Length"); cl != "" {
		size, _ = strconv.ParseInt(cl, 10, 64)
	}
	return size, false, nil
}

func (m *Manager) downloadSingle(ctx context.Context, job *Job, f *os.File) error {
	ad := m.pickAdapter(0)
	client := m.clientFor(ad)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			job.mu.Lock()
			job.Downloaded = written
			if job.TotalSize == 0 && resp.ContentLength > 0 {
				job.TotalSize = resp.ContentLength
			}
			job.mu.Unlock()
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return nil
}

func (m *Manager) downloadPart(ctx context.Context, rawURL string, f *os.File, start, end int64, ad adapter.Adapter, onProgress func(int64)) (int64, error) {
	client := m.clientFor(ad)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d for range %d-%d", resp.StatusCode, start, end)
	}

	buf := make([]byte, 32*1024)
	var got int64
	need := end - start + 1
	for got < need {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.WriteAt(buf[:n], start+got); werr != nil {
				return got, werr
			}
			got += int64(n)
			if onProgress != nil {
				onProgress(got)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return got, rerr
		}
	}
	if got < need {
		return got, fmt.Errorf("short read: got %d want %d", got, need)
	}
	return got, nil
}

func sanitizeName(name string) string {
	name = filepath.Base(name)
	repl := strings.NewReplacer("/", "_", "\\", "_", "..", "_", "\x00", "")
	name = repl.Replace(name)
	if name == "" || name == "." {
		return "file"
	}
	return name
}
