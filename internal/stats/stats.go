package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot is a point-in-time view of per-adapter counters.
type Snapshot struct {
	AdapterID   string `json:"adapterId"`
	Connections uint64 `json:"connections"`
	BytesSent   uint64 `json:"bytesSent"`
	BytesRecv   uint64 `json:"bytesRecv"`
	Errors      uint64 `json:"errors"`
	Active      int64  `json:"active"`
}

type counters struct {
	connections atomic.Uint64
	bytesSent   atomic.Uint64
	bytesRecv   atomic.Uint64
	errors      atomic.Uint64
	active      atomic.Int64
}

// Registry tracks per-adapter traffic stats.
type Registry struct {
	mu    sync.Mutex
	byID  map[string]*counters
	start time.Time
}

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
		c = &counters{}
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

func (r *Registry) All() []Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Snapshot, 0, len(r.byID))
	for id, c := range r.byID {
		out = append(out, Snapshot{
			AdapterID:   id,
			Connections: c.connections.Load(),
			BytesSent:   c.bytesSent.Load(),
			BytesRecv:   c.bytesRecv.Load(),
			Errors:      c.errors.Load(),
			Active:      c.active.Load(),
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
