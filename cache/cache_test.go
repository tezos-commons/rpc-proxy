package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestCache(maxSize int64) (*NetworkCache, *atomic.Int64) {
	gen := &atomic.Int64{}
	gen.Store(1)
	c := New(gen, maxSize)
	return c, gen
}

func TestNetworkCache_GetSet(t *testing.T) {
	c, _ := newTestCache(100)

	// Small body: should not be gzip-compressed (below gzipMinSize threshold)
	small := &Entry{Status: 200, Body: []byte("hello")}
	c.Set("k1", small)

	got, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(got.Body) != "hello" {
		t.Fatalf("Body = %q; want %q", got.Body, "hello")
	}
	if got.GzipBody != nil {
		t.Fatal("GzipBody should be nil for small bodies")
	}

	// Large body: should be gzip-compressed
	largeBody := make([]byte, 512)
	for i := range largeBody {
		largeBody[i] = 'x'
	}
	big := &Entry{Status: 200, Body: largeBody}
	c.Set("k2", big)

	got2, ok := c.Get("k2")
	if !ok {
		t.Fatal("expected cache hit for large entry")
	}
	if got2.GzipBody == nil {
		t.Fatal("GzipBody should be populated for large bodies")
	}
}

func TestNetworkCache_GetMiss(t *testing.T) {
	c, _ := newTestCache(100)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestNetworkCache_SetOnly2xx(t *testing.T) {
	c, _ := newTestCache(100)

	for _, status := range []int{100, 199, 300, 404, 500} {
		c.Set(fmt.Sprintf("s%d", status), &Entry{Status: status, Body: []byte("x")})
	}

	for _, status := range []int{100, 199, 300, 404, 500} {
		_, ok := c.Get(fmt.Sprintf("s%d", status))
		if ok {
			t.Fatalf("status %d should not be cached", status)
		}
	}

	// 200 and 299 should be cached
	c.Set("s200", &Entry{Status: 200, Body: []byte("ok")})
	c.Set("s299", &Entry{Status: 299, Body: []byte("ok")})
	if _, ok := c.Get("s200"); !ok {
		t.Fatal("status 200 should be cached")
	}
	if _, ok := c.Get("s299"); !ok {
		t.Fatal("status 299 should be cached")
	}
}

func TestNetworkCache_GenerationInvalidation(t *testing.T) {
	c, gen := newTestCache(100)

	c.Set("k1", &Entry{Status: 200, Body: []byte("gen1")})
	if _, ok := c.Get("k1"); !ok {
		t.Fatal("expected hit in same generation")
	}

	// Bump generation
	gen.Store(2)

	// Old entry should now be a miss (stale)
	_, ok := c.Get("k1")
	if ok {
		t.Fatal("expected miss after generation bump")
	}
}

func TestNetworkCache_GetDoesNotDeleteStale(t *testing.T) {
	c, gen := newTestCache(100)

	c.Set("k1", &Entry{Status: 200, Body: []byte("gen1")})

	gen.Store(2)

	// Get returns miss but does NOT delete
	_, ok := c.Get("k1")
	if ok {
		t.Fatal("expected miss")
	}

	// Restore generation — entry should be available again
	gen.Store(1)
	_, ok = c.Get("k1")
	if !ok {
		t.Fatal("entry should still exist after stale Get (not deleted)")
	}
}

func TestNetworkCache_SizeLimit(t *testing.T) {
	c, _ := newTestCache(5)

	for i := 0; i < 10; i++ {
		c.Set(fmt.Sprintf("k%d", i), &Entry{Status: 200, Body: []byte("x")})
	}

	// Should only have 5 entries (maxSize)
	hits := 0
	for i := 0; i < 10; i++ {
		if _, ok := c.Get(fmt.Sprintf("k%d", i)); ok {
			hits++
		}
	}
	if hits != 5 {
		t.Fatalf("expected 5 cached entries, got %d", hits)
	}
}

func TestNetworkCache_SetDuplicateDoesNotCountTwice(t *testing.T) {
	c, _ := newTestCache(5)

	c.Set("k1", &Entry{Status: 200, Body: []byte("first")})
	// Set same key again — LoadOrStore finds existing, undoes reservation
	c.Set("k1", &Entry{Status: 200, Body: []byte("second")})

	// Should still be able to store 4 more (total capacity 5)
	for i := 2; i <= 5; i++ {
		c.Set(fmt.Sprintf("k%d", i), &Entry{Status: 200, Body: []byte("x")})
	}

	hits := 0
	for i := 1; i <= 5; i++ {
		if _, ok := c.Get(fmt.Sprintf("k%d", i)); ok {
			hits++
		}
	}
	if hits != 5 {
		t.Fatalf("expected 5 cached entries after duplicate set, got %d", hits)
	}
}

func TestNetworkCache_ShouldCache_FirstSeeFalse(t *testing.T) {
	c, _ := newTestCache(100)

	if c.ShouldCache("new-key") {
		t.Fatal("ShouldCache should return false on first see")
	}
}

func TestNetworkCache_ShouldCache_SecondSeeTrue(t *testing.T) {
	c, _ := newTestCache(100)

	c.ShouldCache("k1")         // first see
	if !c.ShouldCache("k1") { // second see
		t.Fatal("ShouldCache should return true on second see in same generation")
	}
}

func TestNetworkCache_ShouldCache_GenerationReset(t *testing.T) {
	c, gen := newTestCache(100)

	c.ShouldCache("k1") // first see gen=1
	c.ShouldCache("k1") // second see gen=1 -> true

	gen.Store(2)

	// After generation change, first see in new gen should return false
	if c.ShouldCache("k1") {
		t.Fatal("ShouldCache should return false after generation change (stale seen entry)")
	}

	// Second see in new generation should be true
	if !c.ShouldCache("k1") {
		t.Fatal("ShouldCache should return true on second see in new generation")
	}
}

func TestNetworkCache_ShouldCache_SeenSizeLimit(t *testing.T) {
	c, _ := newTestCache(1) // maxSize=1, so maxSeen = 1 * 32 = 32

	// Fill up seen map
	for i := 0; i < 32; i++ {
		c.ShouldCache(fmt.Sprintf("k%d", i))
	}

	// Next unique key should be rejected (seenSize over limit)
	// The key gets added then immediately removed, returning false
	if c.ShouldCache("overflow") {
		t.Fatal("expected false for overflow key")
	}
}

func TestNetworkCache_StartSweep(t *testing.T) {
	gen := &atomic.Int64{}
	gen.Store(1)

	c := &NetworkCache{
		entries:    NewShardMap[*Entry](),
		seen:       NewShardMap[int64](),
		maxSize:    100,
		generation: gen,
	}

	// Insert entry at gen 1
	c.entries.Store("k1", &Entry{generation: 1})
	c.size.Store(1)
	c.seen.Store("k1", int64(1))
	c.seenSize.Store(1)

	// Bump generation to make entries stale
	gen.Store(2)

	// Run sweep manually using RangeDelete (same logic as StartSweep)
	currentGen := gen.Load()
	var removed int64
	c.entries.RangeDelete(func(_ string, e *Entry) bool {
		if e.generation != currentGen {
			removed++
			return true
		}
		return false
	})
	c.size.Add(-removed)

	var seenRemoved int64
	c.seen.RangeDelete(func(_ string, v int64) bool {
		if v != currentGen {
			seenRemoved++
			return true
		}
		return false
	})
	c.seenSize.Add(-seenRemoved)

	if c.entries.Len() != 0 {
		t.Fatal("sweep should have removed stale entry")
	}
	if c.size.Load() != 0 {
		t.Fatalf("size should be 0 after sweep, got %d", c.size.Load())
	}
	if c.seen.Len() != 0 {
		t.Fatal("sweep should have removed stale seen entry")
	}
}

func TestNetworkCache_StartSweep_Context(t *testing.T) {
	c, gen := newTestCache(100)

	c.Set("k1", &Entry{Status: 200, Body: []byte("x")})
	gen.Store(2) // make it stale

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.StartSweep(ctx)
		close(done)
	}()

	// Wait for at least one sweep tick (sweeps every 10s, so we wait a bit)
	// Instead of waiting 10s, cancel quickly just to test cancellation works
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartSweep did not stop after context cancellation")
	}
}

func TestNetworkCache_ConcurrentSetGet(t *testing.T) {
	c, _ := newTestCache(1000)
	const goroutines = 50
	const ops = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("k%d", i)
				c.Set(key, &Entry{Status: 200, Body: []byte("v")})
			}
		}(g)
	}

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("k%d", i)
				c.Get(key)
			}
		}(g)
	}

	wg.Wait()
}

func TestNetworkCache_ConcurrentShouldCache(t *testing.T) {
	c, _ := newTestCache(10000)
	const goroutines = 50
	const ops = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				c.ShouldCache(fmt.Sprintf("k%d", i))
			}
		}(g)
	}

	wg.Wait()
}

func TestNetworkCache_Generation(t *testing.T) {
	c, gen := newTestCache(100)
	if c.Generation() != 1 {
		t.Fatalf("Generation() = %d; want 1", c.Generation())
	}
	gen.Store(42)
	if c.Generation() != 42 {
		t.Fatalf("Generation() = %d; want 42", c.Generation())
	}
}

func TestNetworkCache_NewDefaultMaxSize(t *testing.T) {
	gen := &atomic.Int64{}
	c := New(gen, 0)
	// maxSize should default to 10000
	// We can verify by filling beyond default
	for i := 0; i < 10001; i++ {
		c.Set(fmt.Sprintf("k%d", i), &Entry{Status: 200, Body: []byte("x")})
	}
	hits := 0
	for i := 0; i < 10001; i++ {
		if _, ok := c.Get(fmt.Sprintf("k%d", i)); ok {
			hits++
		}
	}
	if hits != 10000 {
		t.Fatalf("expected 10000 entries with default maxSize, got %d", hits)
	}
}
