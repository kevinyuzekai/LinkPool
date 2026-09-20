package scheduler

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
)

var (
	ErrNoAdapters = errors.New("no selected adapters available")
)

// Entry is a weighted selectable adapter.
type Entry struct {
	Adapter adapter.Adapter
	Weight  int
}

// Scheduler picks the next adapter using smooth weighted round-robin
// (Nginx-style) so new outbound connections are spread by weight.
type Scheduler struct {
	mu      sync.Mutex
	entries []Entry
	current []int // current weights
	total   int

	picks atomic.Uint64
}

// New creates an empty scheduler.
func New() *Scheduler {
	return &Scheduler{}
}

// SetEntries replaces the selectable set. Weight < 1 is treated as 1.
// Only adapters with a non-empty IPv4 are kept.
func (s *Scheduler) SetEntries(entries []Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.current = nil
	s.total = 0
	for _, e := range entries {
		if e.Adapter.IPv4 == "" {
			continue
		}
		w := e.Weight
		if w < 1 {
			w = 1
		}
		e.Weight = w
		s.entries = append(s.entries, e)
		s.current = append(s.current, 0)
		s.total += w
	}
}

// Entries returns a copy of current entries.
func (s *Scheduler) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Next picks the next adapter. Thread-safe.
func (s *Scheduler) Next() (adapter.Adapter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 || s.total == 0 {
		return adapter.Adapter{}, ErrNoAdapters
	}
	best := -1
	for i := range s.entries {
		s.current[i] += s.entries[i].Weight
		if best < 0 || s.current[i] > s.current[best] {
			best = i
		}
	}
	s.current[best] -= s.total
	s.picks.Add(1)
	return s.entries[best].Adapter, nil
}

// PickCount returns how many successful Next() calls occurred.
func (s *Scheduler) PickCount() uint64 {
	return s.picks.Load()
}

// Len returns number of selectable adapters.
func (s *Scheduler) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}
