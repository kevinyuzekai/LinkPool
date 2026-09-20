package scheduler

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
	"github.com/kevinyuzekai/LinkPool/internal/stats"
)

var (
	ErrNoAdapters = errors.New("no selected adapters available")
)

// Mode controls how effective weights are computed.
type Mode string

const (
	ModeStatic   Mode = "static"
	ModeAdaptive Mode = "adaptive"
)

// Entry is a weighted selectable adapter.
type Entry struct {
	Adapter adapter.Adapter
	Weight  int // user-configured base weight
}

type health struct {
	consecFails int
	demoteUntil time.Time
}

// Scheduler picks the next adapter using smooth weighted round-robin
// (Nginx-style). In adaptive mode, effective weights track measured
// throughput so faster NICs receive more new connections.
type Scheduler struct {
	mu      sync.Mutex
	entries []Entry
	current []int // current WRR weights (use effective)
	total   int
	mode    Mode

	stats *stats.Registry

	health map[string]*health

	picks atomic.Uint64
}

// New creates an empty scheduler in adaptive mode.
func New() *Scheduler {
	return &Scheduler{
		mode:   ModeAdaptive,
		health: map[string]*health{},
	}
}

// SetStats attaches a stats registry used for adaptive weighting / least-conn.
func (s *Scheduler) SetStats(st *stats.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = st
}

// SetMode switches static vs adaptive weighting.
func (s *Scheduler) SetMode(m Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m != ModeStatic && m != ModeAdaptive {
		m = ModeAdaptive
	}
	s.mode = m
	s.recomputeLocked()
}

// Mode returns the current scheduling mode.
func (s *Scheduler) Mode() Mode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
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
	}
	s.recomputeLocked()
}

func (s *Scheduler) recomputeLocked() {
	s.total = 0
	if len(s.current) != len(s.entries) {
		s.current = make([]int, len(s.entries))
	}
	for i := range s.entries {
		s.total += s.effectiveWeightLocked(i)
	}
}

// effectiveWeightLocked computes the WRR weight for entry i.
func (s *Scheduler) effectiveWeightLocked(i int) int {
	e := s.entries[i]
	userW := e.Weight
	if userW < 1 {
		userW = 1
	}
	id := e.Adapter.ID

	// Temporary demotion after consecutive dial failures.
	if h := s.health[id]; h != nil && time.Now().Before(h.demoteUntil) {
		return 1
	}

	useAdaptive := s.mode == ModeAdaptive && s.stats != nil && len(s.entries) >= 2
	if !useAdaptive {
		return userW
	}

	rate := s.stats.RateBps(id)
	active := s.stats.Active(id)
	errCount := s.stats.Errors(id)

	// Cold start: little traffic yet → keep user weights.
	var maxRate float64
	for j := range s.entries {
		r := s.stats.RateBps(s.entries[j].Adapter.ID)
		if r > maxRate {
			maxRate = r
		}
	}
	if maxRate < 2048 { // < 2 KB/s aggregate peak → cold
		return userW
	}

	// Relative throughput share (floor so slow NICs still get some).
	rel := rate / maxRate
	if rel < 0.05 {
		rel = 0.05
	}
	// Mild least-conn preference when saturated.
	activePenalty := 1.0 / (1.0 + float64(active)*0.08)
	errPenalty := 1.0 / (1.0 + float64(errCount)*0.15)
	factor := rel * activePenalty * errPenalty
	// Bound so one NIC cannot starve others completely.
	if factor < 0.25 {
		factor = 0.25
	}
	if factor > 4 {
		factor = 4
	}
	// Scale by 10 so integer WRR can express ratios when userW==1.
	w := int(math.Round(float64(userW) * 10 * factor))
	if w < 1 {
		w = 1
	}
	return w
}

// EffectiveWeights returns a copy of id → effective weight (for UI/status).
func (s *Scheduler) EffectiveWeights() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.entries))
	for i, e := range s.entries {
		out[e.Adapter.ID] = s.effectiveWeightLocked(i)
	}
	return out
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
	return s.NextExcluding(nil)
}

// NextExcluding picks the next adapter, skipping IDs in exclude.
func (s *Scheduler) NextExcluding(exclude map[string]bool) (adapter.Adapter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 {
		return adapter.Adapter{}, ErrNoAdapters
	}

	// Refresh totals from adaptive weights each pick (cheap; few NICs).
	s.recomputeLocked()
	if s.total == 0 {
		return adapter.Adapter{}, ErrNoAdapters
	}

	type cand struct {
		idx int
		cw  int
		ew  int
	}
	var candidates []cand
	for i := range s.entries {
		id := s.entries[i].Adapter.ID
		if exclude != nil && exclude[id] {
			continue
		}
		// Skip hard-demoted unless every non-excluded is demoted.
		if h := s.health[id]; h != nil && time.Now().Before(h.demoteUntil) && h.consecFails >= 3 {
			continue
		}
		ew := s.effectiveWeightLocked(i)
		s.current[i] += ew
		candidates = append(candidates, cand{idx: i, cw: s.current[i], ew: ew})
	}

	// If all were hard-skipped, fall back to any non-excluded.
	if len(candidates) == 0 {
		for i := range s.entries {
			id := s.entries[i].Adapter.ID
			if exclude != nil && exclude[id] {
				continue
			}
			ew := s.effectiveWeightLocked(i)
			s.current[i] += ew
			candidates = append(candidates, cand{idx: i, cw: s.current[i], ew: ew})
		}
	}
	if len(candidates) == 0 {
		return adapter.Adapter{}, ErrNoAdapters
	}

	best := 0
	for i := 1; i < len(candidates); i++ {
		if candidates[i].cw > candidates[best].cw {
			best = i
			continue
		}
		// Soft least-conn tie-break when scores are close.
		if candidates[i].cw == candidates[best].cw ||
			abs(candidates[i].cw-candidates[best].cw) <= max(1, candidates[best].ew/4) {
			if s.stats != nil {
				ai := s.stats.Active(s.entries[candidates[i].idx].Adapter.ID)
				ab := s.stats.Active(s.entries[candidates[best].idx].Adapter.ID)
				if ai < ab {
					best = i
				}
			}
		}
	}

	idx := candidates[best].idx
	// Subtract total of *considered* effective weights among non-excluded.
	subTotal := 0
	for i := range s.entries {
		id := s.entries[i].Adapter.ID
		if exclude != nil && exclude[id] {
			continue
		}
		subTotal += s.effectiveWeightLocked(i)
	}
	if subTotal < 1 {
		subTotal = s.total
	}
	s.current[idx] -= subTotal
	s.picks.Add(1)
	return s.entries[idx].Adapter, nil
}

// RecordDialFailure increments consecutive failures and may demote the NIC.
func (s *Scheduler) RecordDialFailure(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.health[id]
	if h == nil {
		h = &health{}
		s.health[id] = h
	}
	h.consecFails++
	if h.consecFails >= 3 {
		h.demoteUntil = time.Now().Add(8 * time.Second)
	} else if h.consecFails >= 2 {
		h.demoteUntil = time.Now().Add(3 * time.Second)
	}
}

// RecordDialSuccess clears consecutive dial failures for an adapter.
func (s *Scheduler) RecordDialSuccess(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h := s.health[id]; h != nil {
		h.consecFails = 0
		h.demoteUntil = time.Time{}
	}
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
