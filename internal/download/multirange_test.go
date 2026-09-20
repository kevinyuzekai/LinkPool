package download

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
	"github.com/kevinyuzekai/LinkPool/internal/proxy"
	"github.com/kevinyuzekai/LinkPool/internal/scheduler"
	"github.com/kevinyuzekai/LinkPool/internal/stats"
)

func TestMultiRangeAssemblesFile(t *testing.T) {
	payload := make([]byte, 64*1024)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/file.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, "file.bin", time.Now(), &bytesReaderAt{b: payload})
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	sched := scheduler.New()
	sched.SetMode(scheduler.ModeStatic)
	sched.SetEntries([]scheduler.Entry{
		{Adapter: adapter.Adapter{ID: "lo", IPv4: "127.0.0.1"}, Weight: 1},
	})
	st := stats.New()
	d := &proxy.Dialer{Sched: sched, Stats: st, Timeout: 5 * time.Second}
	dir := t.TempDir()
	m := NewManager(d, sched, dir)

	url := fmt.Sprintf("http://%s/file.bin", ln.Addr().String())
	job, err := m.Start(context.Background(), url, 4)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap := job.Snapshot()
		if snap.Status == StatusDone || snap.Status == StatusFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	snap := job.Snapshot()
	if snap.Status != StatusDone {
		t.Fatalf("status=%s err=%s parts=%+v", snap.Status, snap.Error, snap.Parts)
	}
	got, err := os.ReadFile(snap.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(payload) {
		t.Fatalf("len got=%d want=%d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("mismatch at %d", i)
		}
	}
	_ = filepath.Base(snap.FilePath)
}

// bytesReaderAt adapts a byte slice to io.ReadSeeker for ServeContent.
type bytesReaderAt struct {
	b []byte
	i int64
}

func (r *bytesReaderAt) Read(p []byte) (int, error) {
	if r.i >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += int64(n)
	return n, nil
}

func (r *bytesReaderAt) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.i + offset
	case io.SeekEnd:
		abs = int64(len(r.b)) + offset
	default:
		return 0, fmt.Errorf("bad whence")
	}
	if abs < 0 {
		return 0, fmt.Errorf("negative")
	}
	r.i = abs
	return abs, nil
}
