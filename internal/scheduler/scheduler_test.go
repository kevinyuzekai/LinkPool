package scheduler

import (
	"testing"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
)

func TestWeightedDistribution(t *testing.T) {
	s := New()
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "a", IPv4: "1.1.1.1"}, Weight: 1},
		{Adapter: adapter.Adapter{ID: "b", IPv4: "2.2.2.2"}, Weight: 3},
	})

	counts := map[string]int{}
	const N = 400
	for i := 0; i < N; i++ {
		a, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		counts[a.ID]++
	}
	// Expect roughly 1:3 → ~100 : ~300
	if counts["a"] < 80 || counts["a"] > 120 {
		t.Fatalf("weight-1 count out of range: %d", counts["a"])
	}
	if counts["b"] < 280 || counts["b"] > 320 {
		t.Fatalf("weight-3 count out of range: %d", counts["b"])
	}
}

func TestNoAdapters(t *testing.T) {
	s := New()
	_, err := s.Next()
	if err != ErrNoAdapters {
		t.Fatalf("want ErrNoAdapters, got %v", err)
	}
}

func TestSkipsEmptyIPv4(t *testing.T) {
	s := New()
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "empty", IPv4: ""}, Weight: 5},
		{Adapter: adapter.Adapter{ID: "ok", IPv4: "10.0.0.1"}, Weight: 1},
	})
	if s.Len() != 1 {
		t.Fatalf("len=%d", s.Len())
	}
	a, err := s.Next()
	if err != nil || a.ID != "ok" {
		t.Fatalf("got %+v err=%v", a, err)
	}
}

func TestEqualWeightsAlternate(t *testing.T) {
	s := New()
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "x", IPv4: "1.1.1.1"}, Weight: 1},
		{Adapter: adapter.Adapter{ID: "y", IPv4: "2.2.2.2"}, Weight: 1},
	})
	seq := []string{}
	for i := 0; i < 4; i++ {
		a, _ := s.Next()
		seq = append(seq, a.ID)
	}
	// Smooth WRR with equal weights: x,y,x,y or similar alternating
	if seq[0] == seq[1] {
		t.Fatalf("expected alternating, got %v", seq)
	}
}
