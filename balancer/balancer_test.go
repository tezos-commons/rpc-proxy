package balancer

import (
	"sync"
	"testing"

	"github.com/tezos-commons/rpc-proxy/tracker"
)

func TestPick_NoHealthyNodes(t *testing.T) {
	// All nodes unhealthy (default state, never Updated).
	n1 := tracker.NewNodeStatus("http://a:8545", "http://a:8545", false)
	n2 := tracker.NewNodeStatus("http://b:8545", "http://b:8545", false)
	b := New([]*tracker.NodeStatus{n1, n2})

	if got := b.Pick(false); got != nil {
		t.Fatalf("expected nil, got %v", got.URL)
	}
}

func TestPick_NoHealthyNodes_Stale(t *testing.T) {
	n1 := tracker.NewNodeStatus("http://a:8545", "http://a:8545", false)
	// Never updated — node has no head, should be considered unhealthy
	b := New([]*tracker.NodeStatus{n1})
	if got := b.Pick(false); got != nil {
		t.Fatalf("expected nil, got %v", got.URL)
	}
}

func TestPick_RoundRobinSameHead(t *testing.T) {
	n1 := tracker.NewNodeStatus("http://a:8545", "http://a:8545", false)
	n2 := tracker.NewNodeStatus("http://b:8545", "http://b:8545", false)
	n3 := tracker.NewNodeStatus("http://c:8545", "http://c:8545", false)
	for _, n := range []*tracker.NodeStatus{n1, n2, n3} {
		n.Update(50)
	}

	b := New([]*tracker.NodeStatus{n1, n2, n3})

	// Collect several picks and verify round-robin rotation.
	seen := make(map[string]int)
	for i := 0; i < 9; i++ {
		got := b.Pick(false)
		if got == nil {
			t.Fatal("unexpected nil")
		}
		seen[got.URL]++
	}

	// Each node should be picked exactly 3 times over 9 calls.
	for _, url := range []string{"http://a:8545", "http://b:8545", "http://c:8545"} {
		if seen[url] != 3 {
			t.Errorf("expected %s picked 3 times, got %d", url, seen[url])
		}
	}
}

func TestPick_HighestHeadOnly(t *testing.T) {
	low := tracker.NewNodeStatus("http://low:8545", "http://low:8545", false)
	mid := tracker.NewNodeStatus("http://mid:8545", "http://mid:8545", false)
	high := tracker.NewNodeStatus("http://high:8545", "http://high:8545", false)
	low.Update(10)
	mid.Update(50)
	high.Update(100)

	b := New([]*tracker.NodeStatus{low, mid, high})

	for i := 0; i < 10; i++ {
		got := b.Pick(false)
		if got == nil {
			t.Fatal("unexpected nil")
		}
		if got.URL != "http://high:8545" {
			t.Fatalf("expected high node, got %s", got.URL)
		}
	}
}

func TestPick_ArchiveOnly(t *testing.T) {
	full := tracker.NewNodeStatus("http://full:8545", "http://full:8545", false)
	arch := tracker.NewNodeStatus("http://archive:8545", "http://archive:8545", true)
	full.Update(100)
	arch.Update(100)

	b := New([]*tracker.NodeStatus{full, arch})

	for i := 0; i < 10; i++ {
		got := b.Pick(true)
		if got == nil {
			t.Fatal("unexpected nil")
		}
		if got.URL != "http://archive:8545" {
			t.Fatalf("expected archive node, got %s", got.URL)
		}
	}
}

func TestPick_ArchiveOnly_NoArchiveNodes(t *testing.T) {
	full := tracker.NewNodeStatus("http://full:8545", "http://full:8545", false)
	full.Update(100)

	b := New([]*tracker.NodeStatus{full})
	if got := b.Pick(true); got != nil {
		t.Fatalf("expected nil when no archive nodes, got %v", got.URL)
	}
}

func TestHeadGeneration_IncrementsOnAdvance(t *testing.T) {
	n := tracker.NewNodeStatus("http://a:8545", "http://a:8545", false)
	n.Update(10)
	b := New([]*tracker.NodeStatus{n})

	b.Pick(false)
	gen1 := b.HeadGeneration()

	n.Update(20)
	b.Pick(false)
	gen2 := b.HeadGeneration()

	if gen2 <= gen1 {
		t.Fatalf("expected generation to advance: gen1=%d gen2=%d", gen1, gen2)
	}
}

func TestHeadGeneration_StableWhenHeadUnchanged(t *testing.T) {
	n := tracker.NewNodeStatus("http://a:8545", "http://a:8545", false)
	n.Update(10)
	b := New([]*tracker.NodeStatus{n})

	b.Pick(false)
	gen1 := b.HeadGeneration()

	// Pick several more times without changing head.
	for i := 0; i < 20; i++ {
		b.Pick(false)
	}
	gen2 := b.HeadGeneration()

	if gen2 != gen1 {
		t.Fatalf("expected stable generation: gen1=%d gen2=%d", gen1, gen2)
	}
}

func TestPick_ConcurrentRaceDetector(t *testing.T) {
	nodes := make([]*tracker.NodeStatus, 4)
	for i := range nodes {
		name := "node" + string(rune('A'+i))
			nodes[i] = tracker.NewNodeStatus(name, "http://"+name+":8545", i%2 == 0)
		nodes[i].Update(int64(100 + i))
	}
	b := New(nodes)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = b.Pick(false)
				_ = b.Pick(true)
				_ = b.HeadGeneration()
			}
		}()
	}

	// Also mutate heads concurrently.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				nodes[idx].Update(int64(200 + j))
			}
		}(i)
	}

	wg.Wait()
}

func TestPick_MixHealthyStale(t *testing.T) {
	n1 := tracker.NewNodeStatus("http://healthy1:8545", "http://healthy1:8545", false)
	n2 := tracker.NewNodeStatus("http://stale:8545", "http://stale:8545", false)
	n3 := tracker.NewNodeStatus("http://healthy2:8545", "http://healthy2:8545", false)

	n1.Update(100)
	// n2 never updated — stale/unhealthy
	n3.Update(100)

	b := New([]*tracker.NodeStatus{n1, n2, n3})

	seen := make(map[string]int)
	for i := 0; i < 20; i++ {
		got := b.Pick(false)
		if got == nil {
			t.Fatal("unexpected nil")
		}
		seen[got.URL]++
	}

	if seen["http://stale:8545"] != 0 {
		t.Fatalf("stale node should never be picked, was picked %d times", seen["http://stale:8545"])
	}
	if seen["http://healthy1:8545"] == 0 || seen["http://healthy2:8545"] == 0 {
		t.Fatalf("both healthy nodes should be picked: %v", seen)
	}
}

func TestPick_HighestHeadStale(t *testing.T) {
	high := tracker.NewNodeStatus("http://high:8545", "http://high:8545", false)
	low := tracker.NewNodeStatus("http://low:8545", "http://low:8545", false)

	// high has a head but is stale (updatedAt far in the past)
	high.Update(200)
	high.ForceUpdatedAt(1) // epoch — stale
	low.Update(100)

	b := New([]*tracker.NodeStatus{high, low})

	got := b.Pick(false)
	if got == nil {
		t.Fatal("unexpected nil")
	}
	if got.URL != "http://low:8545" {
		t.Fatalf("expected low node (only healthy one), got %s", got.URL)
	}
}

func TestPick_EmptyNodes(t *testing.T) {
	b := New([]*tracker.NodeStatus{})
	if got := b.Pick(false); got != nil {
		t.Fatalf("expected nil for empty node list, got %v", got.URL)
	}
}

func TestHeadGenerationPtr(t *testing.T) {
	n := tracker.NewNodeStatus("http://a:8545", "http://a:8545", false)
	n.Update(10)
	b := New([]*tracker.NodeStatus{n})
	b.Pick(false)

	ptr := b.HeadGenerationPtr()
	if ptr.Load() != b.HeadGeneration() {
		t.Fatal("HeadGenerationPtr and HeadGeneration disagree")
	}
}
