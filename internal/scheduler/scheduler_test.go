package scheduler

import (
	"testing"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
	"github.com/kevinyuzekai/LinkPool/internal/stats"
)

func TestWeightedDistribution(t *testing.T) {
	s := New()
	s.SetMode(ModeStatic)
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
	s.SetMode(ModeStatic)
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "x", IPv4: "1.1.1.1"}, Weight: 1},
		{Adapter: adapter.Adapter{ID: "y", IPv4: "2.2.2.2"}, Weight: 1},
	})
	seq := []string{}
	for i := 0; i < 4; i++ {
		a, _ := s.Next()
		seq = append(seq, a.ID)
	}
	if seq[0] == seq[1] {
		t.Fatalf("expected alternating, got %v", seq)
	}
}

func TestAdaptivePrefersFasterNIC(t *testing.T) {
	st := stats.New()
	s := New()
	s.SetStats(st)
	s.SetMode(ModeAdaptive)
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "slow", IPv4: "1.1.1.1"}, Weight: 1},
		{Adapter: adapter.Adapter{ID: "fast", IPv4: "2.2.2.2"}, Weight: 1},
	})

	// Seed traffic so Sample can compute rates.
	st.AddRecv("slow", 10*1024)
	st.AddRecv("fast", 200*1024)
	time.Sleep(250 * time.Millisecond)
	st.AddRecv("slow", 10*1024)
	st.AddRecv("fast", 400*1024)
	st.Sample()
	time.Sleep(250 * time.Millisecond)
	st.AddRecv("slow", 5*1024)
	st.AddRecv("fast", 400*1024)
	st.Sample()

	ew := s.EffectiveWeights()
	if ew["fast"] <= ew["slow"] {
		t.Fatalf("expected fast effective weight > slow, got %#v rates slow=%v fast=%v",
			ew, st.RateBps("slow"), st.RateBps("fast"))
	}

	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		a, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		counts[a.ID]++
	}
	if counts["fast"] <= counts["slow"] {
		t.Fatalf("adaptive should favor fast NIC: %#v weights=%#v", counts, ew)
	}
}

func TestAdaptiveEqualSpeedFair(t *testing.T) {
	st := stats.New()
	s := New()
	s.SetStats(st)
	s.SetMode(ModeAdaptive)
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "a", IPv4: "1.1.1.1"}, Weight: 1},
		{Adapter: adapter.Adapter{ID: "b", IPv4: "2.2.2.2"}, Weight: 1},
	})
	for i := 0; i < 3; i++ {
		st.AddRecv("a", 100*1024)
		st.AddRecv("b", 100*1024)
		time.Sleep(220 * time.Millisecond)
		st.Sample()
	}
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		ad, _ := s.Next()
		counts[ad.ID]++
	}
	// Fair under equal speed: within ~35–65 each
	if counts["a"] < 35 || counts["a"] > 65 {
		t.Fatalf("not fair under equal speed: %#v ew=%#v", counts, s.EffectiveWeights())
	}
}

func TestNextExcluding(t *testing.T) {
	s := New()
	s.SetMode(ModeStatic)
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "a", IPv4: "1.1.1.1"}, Weight: 1},
		{Adapter: adapter.Adapter{ID: "b", IPv4: "2.2.2.2"}, Weight: 1},
	})
	ad, err := s.NextExcluding(map[string]bool{"a": true})
	if err != nil || ad.ID != "b" {
		t.Fatalf("got %+v err=%v", ad, err)
	}
}

func TestDialFailureDemote(t *testing.T) {
	s := New()
	s.SetMode(ModeStatic)
	s.SetEntries([]Entry{
		{Adapter: adapter.Adapter{ID: "bad", IPv4: "1.1.1.1"}, Weight: 10},
		{Adapter: adapter.Adapter{ID: "good", IPv4: "2.2.2.2"}, Weight: 1},
	})
	for i := 0; i < 3; i++ {
		s.RecordDialFailure("bad")
	}
	// After demotion, picks should heavily favor good when excluding isn't used —
	// demoted still has weight 1 but hard-skip when consecFails>=3.
	counts := map[string]int{}
	for i := 0; i < 20; i++ {
		ad, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		counts[ad.ID]++
	}
	if counts["good"] < 15 {
		t.Fatalf("expected demoted NIC skipped, got %#v", counts)
	}
	s.RecordDialSuccess("bad")
}
