package tracker

import (
	"fmt"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// NodeStatus tests
// ---------------------------------------------------------------------------

func TestNewNodeStatus(t *testing.T) {
	ns := NewNodeStatus("https://rpc.example.com", "https://rpc.example.com", true)
	if ns.URL != "https://rpc.example.com" {
		t.Fatalf("expected URL https://rpc.example.com, got %s", ns.URL)
	}
	if !ns.Archive {
		t.Fatal("expected Archive to be true")
	}

	ns2 := NewNodeStatus("https://rolling.example.com", "https://rolling.example.com", false)
	if ns2.Archive {
		t.Fatal("expected Archive to be false")
	}
}

func TestNodeStatus_Update(t *testing.T) {
	ns := NewNodeStatus("http://localhost:8732", "http://localhost:8732", false)

	// Before any update, head should be zero and healthy false.
	head, healthy := ns.GetHead()
	if head != 0 {
		t.Fatalf("expected initial head 0, got %d", head)
	}
	if healthy {
		t.Fatal("expected initial healthy to be false")
	}

	ns.Update(100)
	head, healthy = ns.GetHead()
	if head != 100 {
		t.Fatalf("expected head 100, got %d", head)
	}
	if !healthy {
		t.Fatal("expected healthy after Update")
	}

	// updatedAt should have been set (non-zero).
	if ns.updatedAt.Load() == 0 {
		t.Fatal("expected updatedAt to be non-zero after Update")
	}
}

func TestNodeStatus_Staleness(t *testing.T) {
	ns := NewNodeStatus("http://localhost:8732", "http://localhost:8732", false)
	ns.Update(50)

	_, healthy := ns.GetHead()
	if !healthy {
		t.Fatal("expected healthy after Update")
	}

	// SetUnhealthy is a no-op — health is determined by staleness
	ns.SetUnhealthy()
	_, healthy = ns.GetHead()
	if !healthy {
		t.Fatal("expected still healthy after SetUnhealthy (staleness-based)")
	}

	// Simulate stale node by setting updatedAt far in the past
	ns.updatedAt.Store(1)
	_, healthy = ns.GetHead()
	if healthy {
		t.Fatal("expected unhealthy when stale")
	}
}

func TestNodeStatus_GetHead(t *testing.T) {
	ns := NewNodeStatus("http://localhost:8732", "http://localhost:8732", false)

	// Successive updates should reflect the latest head.
	for _, h := range []int64{10, 20, 30} {
		ns.Update(h)
		got, healthy := ns.GetHead()
		if got != h {
			t.Fatalf("expected head %d, got %d", h, got)
		}
		if !healthy {
			t.Fatal("expected healthy after Update")
		}
	}

	// SetUnhealthy is a no-op — node remains healthy based on staleness.
	ns.SetUnhealthy()
	head, healthy := ns.GetHead()
	if head != 30 {
		t.Fatalf("expected head 30 after SetUnhealthy, got %d", head)
	}
	if !healthy {
		t.Fatal("expected still healthy (staleness-based)")
	}
}

func TestNodeStatus_Concurrent(t *testing.T) {
	ns := NewNodeStatus("http://localhost:8732", "http://localhost:8732", false)
	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 200

	// Writers: Update
	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				ns.Update(int64(id*iterations + j))
			}
		}(i)
	}

	// Readers: GetHead
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				ns.GetHead()
			}
		}()
	}

	// Unhealthy togglers
	for range goroutines / 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				ns.SetUnhealthy()
			}
		}()
	}

	wg.Wait()
	// If we get here without the race detector complaining, the test passes.
}

// ---------------------------------------------------------------------------
// RecentBlocks tests
// ---------------------------------------------------------------------------

func TestRecentBlocks_AddAndContainsHash(t *testing.T) {
	rb := NewRecentBlocks(100)

	rb.Add(1, "hash1")
	rb.Add(2, "hash2")
	rb.Add(3, "hash3")

	if !rb.ContainsHash("hash1") {
		t.Fatal("expected hash1 to be present")
	}
	if !rb.ContainsHash("hash2") {
		t.Fatal("expected hash2 to be present")
	}
	if !rb.ContainsHash("hash3") {
		t.Fatal("expected hash3 to be present")
	}
	if rb.ContainsHash("nonexistent") {
		t.Fatal("expected nonexistent hash to be absent")
	}
}

func TestRecentBlocks_Head(t *testing.T) {
	rb := NewRecentBlocks(100)

	if rb.Head() != 0 {
		t.Fatalf("expected initial head 0, got %d", rb.Head())
	}

	rb.Add(10, "h10")
	if rb.Head() != 10 {
		t.Fatalf("expected head 10, got %d", rb.Head())
	}

	rb.Add(20, "h20")
	if rb.Head() != 20 {
		t.Fatalf("expected head 20, got %d", rb.Head())
	}

	// Adding a lower level should not decrease head.
	rb.Add(5, "h5")
	if rb.Head() != 20 {
		t.Fatalf("expected head still 20, got %d", rb.Head())
	}
}

func TestRecentBlocks_Pruning(t *testing.T) {
	window := int64(10)
	rb := NewRecentBlocks(window)

	// Add levels 1..20.
	for i := int64(1); i <= 20; i++ {
		rb.Add(i, fmt.Sprintf("h%d", i))
	}

	// Head is 20, cutoff = 20-10 = 10. Levels < 10 should be pruned.
	for i := int64(1); i < 10; i++ {
		hash := fmt.Sprintf("h%d", i)
		if rb.ContainsHash(hash) {
			t.Fatalf("expected hash h%d (level %d) to be pruned", i, i)
		}
	}

	// Levels 10..20 should still be present.
	for i := int64(10); i <= 20; i++ {
		hash := fmt.Sprintf("h%d", i)
		if !rb.ContainsHash(hash) {
			t.Fatalf("expected hash h%d (level %d) to be present", i, i)
		}
	}
}

func TestRecentBlocks_Reorg(t *testing.T) {
	rb := NewRecentBlocks(100)

	// Add levels 1..5 on the "original" chain.
	for i := int64(1); i <= 5; i++ {
		rb.Add(i, fmt.Sprintf("orig%d", i))
	}

	// Reorg at level 3: add level 3 with a different hash.
	rb.Add(3, "fork3")

	// Levels 3, 4, 5 originals should be gone.
	if rb.ContainsHash("orig3") {
		t.Fatal("expected orig3 to be removed after reorg")
	}
	if rb.ContainsHash("orig4") {
		t.Fatal("expected orig4 to be removed after reorg")
	}
	if rb.ContainsHash("orig5") {
		t.Fatal("expected orig5 to be removed after reorg")
	}

	// The new fork hash should be present.
	if !rb.ContainsHash("fork3") {
		t.Fatal("expected fork3 to be present")
	}

	// Levels 1 and 2 should still be intact.
	if !rb.ContainsHash("orig1") {
		t.Fatal("expected orig1 to still be present")
	}
	if !rb.ContainsHash("orig2") {
		t.Fatal("expected orig2 to still be present")
	}
}

func TestRecentBlocks_EmptyHashIgnored(t *testing.T) {
	rb := NewRecentBlocks(100)

	rb.Add(1, "")
	if rb.Head() != 0 {
		t.Fatal("expected head to remain 0 when adding empty hash")
	}
	if rb.ContainsHash("") {
		t.Fatal("expected empty hash not to be stored")
	}

	// Ensure maps are still empty.
	rb.mu.RLock()
	levelCount := len(rb.byLevel)
	hashCount := len(rb.byHash)
	rb.mu.RUnlock()
	if levelCount != 0 || hashCount != 0 {
		t.Fatalf("expected empty maps, got byLevel=%d byHash=%d", levelCount, hashCount)
	}
}

func TestRecentBlocks_Window(t *testing.T) {
	rb := NewRecentBlocks(200)
	if rb.Window() != 200 {
		t.Fatalf("expected window 200, got %d", rb.Window())
	}

	// Zero or negative should fall back to DefaultBlockWindow.
	rb2 := NewRecentBlocks(0)
	if rb2.Window() != DefaultBlockWindow {
		t.Fatalf("expected default window %d, got %d", DefaultBlockWindow, rb2.Window())
	}

	rb3 := NewRecentBlocks(-10)
	if rb3.Window() != DefaultBlockWindow {
		t.Fatalf("expected default window %d, got %d", DefaultBlockWindow, rb3.Window())
	}
}

func TestRecentBlocks_Concurrent(t *testing.T) {
	rb := NewRecentBlocks(50)
	var wg sync.WaitGroup
	const goroutines = 30
	const iterations = 200

	// Writers: Add
	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				level := int64(id*iterations + j)
				hash := fmt.Sprintf("h%d-%d", id, j)
				rb.Add(level, hash)
			}
		}(i)
	}

	// Readers: ContainsHash
	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				rb.ContainsHash(fmt.Sprintf("h%d-%d", id, j))
			}
		}(i)
	}

	// Readers: Head
	for range goroutines / 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				rb.Head()
			}
		}()
	}

	wg.Wait()
	// Race detector will flag any issues.
}
