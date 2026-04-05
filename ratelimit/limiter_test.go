package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tezos-commons/rpc-proxy/filter"
)

// ---------------------------------------------------------------------------
// 1. Token bucket allows requests up to rate, then rejects
// ---------------------------------------------------------------------------

func TestTokenBucket_AllowUpToRate_ThenReject(t *testing.T) {
	tb := newTokenBucket(5) // 5 rps, burst = 7 (5+5/2)

	// Burst capacity is 7, so 7 requests should succeed immediately.
	allowed := 0
	for i := 0; i < 20; i++ {
		if tb.allow() {
			allowed++
		}
	}
	if allowed != 7 {
		t.Fatalf("expected 7 allowed (burst capacity), got %d", allowed)
	}
}

func TestTokenBucket_RejectsAfterBurstExhausted(t *testing.T) {
	tb := newTokenBucket(3) // burst = max(3+1, 2) = 4

	// Drain the bucket.
	for tb.allow() {
	}

	// Next request should be rejected.
	if tb.allow() {
		t.Fatal("expected rejection after tokens exhausted")
	}
}

// ---------------------------------------------------------------------------
// 2. Token bucket refills over time
// ---------------------------------------------------------------------------

func TestTokenBucket_RefillsOverTime(t *testing.T) {
	tb := newTokenBucket(10) // 10 rps, burst = 15

	// Drain all tokens.
	for tb.allow() {
	}

	// After 200ms at 10 rps, we should gain ~2 tokens.
	time.Sleep(250 * time.Millisecond)

	allowed := 0
	for tb.allow() {
		allowed++
	}

	// Should have refilled at least 1 token (be lenient for CI timing).
	if allowed < 1 {
		t.Fatalf("expected at least 1 token after 250ms at 10 rps, got %d", allowed)
	}
	if allowed > 3 {
		t.Fatalf("expected at most 3 tokens after 250ms at 10 rps, got %d", allowed)
	}
}

func TestTokenBucket_RefillCappedAtMax(t *testing.T) {
	tb := newTokenBucket(10) // burst = 15

	// Drain some tokens.
	for i := 0; i < 5; i++ {
		tb.allow()
	}

	// Wait long enough to overfill if uncapped.
	time.Sleep(3 * time.Second)

	allowed := 0
	for tb.allow() {
		allowed++
	}

	// Should be capped at burst = 15 (the max).
	if allowed > 15 {
		t.Fatalf("tokens exceeded burst cap: got %d, max burst 15", allowed)
	}
}

// ---------------------------------------------------------------------------
// 3. Token bucket burst capacity (1.5x rate, minimum 2)
// ---------------------------------------------------------------------------

func TestTokenBucket_BurstCapacity(t *testing.T) {
	tests := []struct {
		rps           int
		expectedBurst int
	}{
		{1, 2},   // 1 + 0 = 1, clamped to min 2
		{2, 3},   // 2 + 1 = 3
		{3, 4},   // 3 + 1 = 4 (integer division: 3/2=1)
		{4, 6},   // 4 + 2 = 6
		{5, 7},   // 5 + 2 = 7
		{10, 15}, // 10 + 5 = 15
		{20, 30}, // 20 + 10 = 30
		{100, 150},
	}

	for _, tt := range tests {
		tb := newTokenBucket(tt.rps)

		allowed := 0
		for tb.allow() {
			allowed++
		}

		if allowed != tt.expectedBurst {
			t.Errorf("rps=%d: expected burst %d, got %d", tt.rps, tt.expectedBurst, allowed)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Token bucket is correct under concurrent access
// ---------------------------------------------------------------------------

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	// Verify the mutex-based bucket is safe and correct under concurrent access.
	tb := newTokenBucket(100) // 100 rps, burst = 150

	var wg sync.WaitGroup
	var allowed atomic.Int64
	const goroutines = 50

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if tb.allow() {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	// All 50 should succeed (burst = 150).
	if got := allowed.Load(); got != goroutines {
		t.Fatalf("expected all %d to be allowed, got %d", goroutines, got)
	}
}

// ---------------------------------------------------------------------------
// 5. IPRateLimiter creates separate buckets per IP
// ---------------------------------------------------------------------------

func TestIPRateLimiter_SeparateBucketsPerIP(t *testing.T) {
	var rates [filter.NumTiers]int
	rates[filter.TierDefault] = 5 // burst = 7
	for i := 1; i < int(filter.NumTiers); i++ {
		rates[i] = 100 // high rate so they don't interfere
	}

	limiter := New(rates, false)

	// Drain IP "1.1.1.1".
	for limiter.Allow("1.1.1.1", filter.TierDefault) {
	}

	// IP "2.2.2.2" should still have tokens.
	if !limiter.Allow("2.2.2.2", filter.TierDefault) {
		t.Fatal("different IP should have its own bucket")
	}
}

// ---------------------------------------------------------------------------
// 6. IPRateLimiter creates separate buckets per tier
// ---------------------------------------------------------------------------

func TestIPRateLimiter_SeparateBucketsPerTier(t *testing.T) {
	var rates [filter.NumTiers]int
	rates[filter.TierDefault] = 3   // burst = 4
	rates[filter.TierExpensive] = 5 // burst = 7
	for i := 2; i < int(filter.NumTiers); i++ {
		rates[i] = 100
	}

	limiter := New(rates, false)
	ip := "10.0.0.1"

	// Drain TierDefault.
	for limiter.Allow(ip, filter.TierDefault) {
	}

	// TierExpensive should still have tokens.
	if !limiter.Allow(ip, filter.TierExpensive) {
		t.Fatal("different tier should have its own bucket")
	}

	// Verify TierDefault is actually exhausted.
	if limiter.Allow(ip, filter.TierDefault) {
		t.Fatal("TierDefault should be exhausted")
	}
}

// ---------------------------------------------------------------------------
// 7. IPRateLimiter getOrCreate is safe under concurrent access
// ---------------------------------------------------------------------------

func TestIPRateLimiter_ConcurrentGetOrCreate(t *testing.T) {
	var rates [filter.NumTiers]int
	for i := range rates {
		rates[i] = 100
	}
	limiter := New(rates, false)

	const goroutines = 50
	const ip = "race-test-ip"

	var wg sync.WaitGroup
	wg.Add(goroutines)

	var allowed atomic.Int64
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if limiter.Allow(ip, filter.TierDefault) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	// All goroutines should get the same entry; burst is 150, so all 50
	// should succeed.
	if got := allowed.Load(); got != goroutines {
		t.Fatalf("expected all %d to be allowed, got %d", goroutines, got)
	}

	// Verify only one entry exists for this IP.
	count := 0
	limiter.entries.Range(func(key string, _ *ipEntry) bool {
		if key == ip {
			count++
		}
		return true
	})
	if count != 1 {
		t.Fatalf("expected exactly 1 entry for IP, got %d", count)
	}
}

func TestIPRateLimiter_ConcurrentMultipleIPs(t *testing.T) {
	var rates [filter.NumTiers]int
	for i := range rates {
		rates[i] = 50
	}
	limiter := New(rates, false)

	const goroutines = 100
	ips := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		ip := ips[i%len(ips)]
		go func() {
			defer wg.Done()
			limiter.Allow(ip, filter.Tier(i%int(filter.NumTiers)))
		}()
	}
	wg.Wait()

	// Verify all IPs were created.
	seen := make(map[string]bool)
	limiter.entries.Range(func(key string, _ *ipEntry) bool {
		seen[key] = true
		return true
	})
	for _, ip := range ips {
		if !seen[ip] {
			t.Errorf("missing entry for IP %s", ip)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. StartCleanup removes stale entries (not seen in 5 min)
// ---------------------------------------------------------------------------

func TestStartCleanup_RemovesStaleEntries(t *testing.T) {
	var rates [filter.NumTiers]int
	for i := range rates {
		rates[i] = 10
	}
	limiter := New(rates, false)

	// Create entries.
	limiter.Allow("fresh-ip", filter.TierDefault)
	limiter.Allow("stale-ip", filter.TierDefault)

	// Manually backdate the stale entry's lastSeen to 6 minutes ago.
	staleEntry, _ := limiter.entries.Load("stale-ip")
	staleEntry.lastSeen.Store(time.Now().Add(-6 * time.Minute).UnixNano())

	// Run cleanup inline by simulating what the ticker does.
	cutoff := time.Now().Add(-5 * time.Minute).UnixNano()
	limiter.entries.RangeDelete(func(_ string, entry *ipEntry) bool {
		return entry.lastSeen.Load() < cutoff
	})

	// Verify stale-ip was removed and fresh-ip remains.
	if _, ok := limiter.entries.Load("stale-ip"); ok {
		t.Fatal("stale-ip should have been cleaned up")
	}
	if _, ok := limiter.entries.Load("fresh-ip"); !ok {
		t.Fatal("fresh-ip should still be present")
	}
}

func TestStartCleanup_ContextCancellation(t *testing.T) {
	var rates [filter.NumTiers]int
	for i := range rates {
		rates[i] = 10
	}
	limiter := New(rates, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		limiter.StartCleanup(ctx)
		close(done)
	}()

	// Cancel should cause StartCleanup to return.
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("StartCleanup did not stop after context cancellation")
	}
}

func TestStartCleanup_KeepsFreshEntries(t *testing.T) {
	var rates [filter.NumTiers]int
	for i := range rates {
		rates[i] = 10
	}
	limiter := New(rates, false)

	// Create entry with recent lastSeen (just 1 minute ago).
	limiter.Allow("recent-ip", filter.TierDefault)
	entry, _ := limiter.entries.Load("recent-ip")
	entry.lastSeen.Store(time.Now().Add(-1 * time.Minute).UnixNano())

	// Run cleanup logic.
	cutoff := time.Now().Add(-5 * time.Minute).UnixNano()
	limiter.entries.RangeDelete(func(_ string, e *ipEntry) bool {
		return e.lastSeen.Load() < cutoff
	})

	if _, ok := limiter.entries.Load("recent-ip"); !ok {
		t.Fatal("recent entry should not be cleaned up")
	}
}

// ---------------------------------------------------------------------------
// Fixed-point math verification
// ---------------------------------------------------------------------------

func TestTokenBucket_FixedPointInternals(t *testing.T) {
	tb := newTokenBucket(10) // rate=10000, maxTokens=15000

	if tb.rate != 10_000 {
		t.Errorf("expected rate 10000, got %d", tb.rate)
	}
	if tb.maxTokens != 15_000 {
		t.Errorf("expected maxTokens 15000, got %d", tb.maxTokens)
	}

	tb.mu.Lock()
	tokens := tb.tokens
	tb.mu.Unlock()
	if tokens != 15_000 {
		t.Errorf("expected initial tokens 15000, got %d", tokens)
	}
}

func TestTokenBucket_MinBurstIsTwo(t *testing.T) {
	// Even at 1 rps the burst should be 2.
	tb := newTokenBucket(1)

	if tb.maxTokens != 2000 {
		t.Errorf("expected maxTokens 2000 for 1 rps, got %d", tb.maxTokens)
	}

	allowed := 0
	for tb.allow() {
		allowed++
	}
	if allowed != 2 {
		t.Errorf("expected burst of 2 for 1 rps, got %d", allowed)
	}
}
