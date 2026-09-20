package stats

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot is a point-in-time view of per-adapter counters.
type Snapshot struct {
	AdapterID   string  `json:"adapterId"`
	Connections uint64  `json:"connections"`
	BytesSent   uint64  `json:"bytesSent"`
	BytesRecv   uint64  `json:"bytesRecv"`
	Errors      uint64  `json:"errors"`
	Active      int64   `json:"active"`
	RateBps     float64 `json:"rateBps"` // EMA of (recv+sent) bytes/sec
}

type counters struct {
	connections atomic.Uint64
	bytesSent   atomic.Uint64
	bytesRecv   atomic.Uint64
	errors      atomic.Uint64
	active      atomic.Int64

	// rate sampling
	mu         sync.Mutex
	lastTotal  uint64
	lastSample time.Time
	emaRate    float64 // bytes/sec
}

// Registry tracks per-adapter traffic stats and short-window rates.
type Registry struct {
	mu    sync.Mutex
	byID  map[string]*counters
	start time.Time
}

const emaAlpha = 0.45 // ~2–5s responsiveness with 1–2s sample interval

func New() *Registry {
	return &Registry{
		byID:  map[string]*counters{},
		start: time.Now(),
	}
}

func (r *Registry) get(id string) *counters {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[id]
	if !ok {
		c = &counters{lastSample: time.Now()}
		r.byID[id] = c
	}
	return c
}

func (r *Registry) ConnOpen(id string) {
	c := r.get(id)
	c.connections.Add(1)
	c.active.Add(1)
}

func (r *Registry) ConnClose(id string) {
	c := r.get(id)
	c.active.Add(-1)
}

func (r *Registry) AddSent(id string, n int64) {
	if n <= 0 {
		return
	}
	r.get(id).bytesSent.Add(uint64(n))
}

func (r *Registry) AddRecv(id string, n int64) {
	if n <= 0 {
		return
	}
	r.get(id).bytesRecv.Add(uint64(n))
}

func (r *Registry) AddError(id string) {
	r.get(id).errors.Add(1)
}

// Errors returns the error counter for an adapter.
func (r *Registry) Errors(id string) uint64 {
	return r.get(id).errors.Load()
}

// Active returns current active connections for an adapter.
func (r *Registry) Active(id string) int64 {
	return r.get(id).active.Load()
}

// RateBps returns EMA throughput (recv+sent) for an adapter.
func (r *Registry) RateBps(id string) float64 {
	c := r.get(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emaRate
}

// Sample updates EMA rates for all adapters from byte counters.
// Call periodically (e.g. on status poll).
func (r *Registry) Sample() {
	r.mu.Lock()
	ids := make([]*counters, 0, len(r.byID))
	for _, c := range r.byID {
		ids = append(ids, c)
	}
	r.mu.Unlock()
	now := time.Now()
	for _, c := range ids {
		c.sample(now)
	}
}

func (c *counters) sample(now time.Time) {
	total := c.bytesSent.Load() + c.bytesRecv.Load()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastSample.IsZero() {
		c.lastTotal = total
		c.lastSample = now
		return
	}
	dt := now.Sub(c.lastSample).Seconds()
	if dt < 0.2 {
		return
	}
	delta := float64(total - c.lastTotal)
	inst := delta / dt
	if c.emaRate == 0 {
		c.emaRate = inst
	} else {
		c.emaRate = emaAlpha*inst + (1-emaAlpha)*c.emaRate
	}
	c.lastTotal = total
	c.lastSample = now
}

func (r *Registry) All() []Snapshot {
	r.Sample()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Snapshot, 0, len(r.byID))
	for id, c := range r.byID {
		c.mu.Lock()
		rate := c.emaRate
		c.mu.Unlock()
		out = append(out, Snapshot{
			AdapterID:   id,
			Connections: c.connections.Load(),
			BytesSent:   c.bytesSent.Load(),
			BytesRecv:   c.bytesRecv.Load(),
			Errors:      c.errors.Load(),
			Active:      c.active.Load(),
			RateBps:     math.Round(rate*10) / 10,
		})
	}
	return out
}

func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID = map[string]*counters{}
	r.start = time.Now()
}

func (r *Registry) StartedAt() time.Time {
	return r.start
}
