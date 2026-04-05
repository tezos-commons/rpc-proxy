package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tezos-commons/rpc-proxy/cache"
	"github.com/tezos-commons/rpc-proxy/filter"
)

// tokenBucket is a mutex-protected token bucket rate limiter.
// Uses a mutex instead of atomic CAS to avoid a heap allocation per allow() call.
// Per-IP per-tier granularity means contention is minimal.
type tokenBucket struct {
	mu        sync.Mutex
	tokens    int64 // tokens * 1000 (fixed-point for precision)
	lastTime  int64 // unix nano
	rate      int64 // tokens per second * 1000
	maxTokens int64 // burst * 1000
}

func newTokenBucket(rps int) *tokenBucket {
	// Burst = 1.5x the rate, minimum 2
	burst := rps + rps/2
	if burst < 2 {
		burst = 2
	}
	return &tokenBucket{
		tokens:    int64(burst) * 1000,
		lastTime:  time.Now().UnixNano(),
		rate:      int64(rps) * 1000,
		maxTokens: int64(burst) * 1000,
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	now := time.Now().UnixNano()

	elapsed := now - tb.lastTime
	if elapsed < 0 {
		elapsed = 0
	}

	// Add tokens based on elapsed time
	added := tb.rate * elapsed / int64(time.Second)
	tb.tokens += added
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastTime = now

	if tb.tokens < 1000 {
		tb.mu.Unlock()
		return false
	}

	tb.tokens -= 1000
	tb.mu.Unlock()
	return true
}

// ipEntry holds per-tier token buckets for a single IP.
// Buckets are lazily initialized on first use to save memory — most IPs
// only ever hit 1-2 tiers.
type ipEntry struct {
	mu       sync.Mutex
	buckets  [filter.NumTiers]*tokenBucket
	lastSeen atomic.Int64 // unix nano
	rates    *[filter.NumTiers]int
}

func (e *ipEntry) allow(tier filter.Tier) bool {
	// Fast path: bucket already initialized (common case after first request)
	e.mu.Lock()
	tb := e.buckets[tier]
	if tb == nil {
		tb = newTokenBucket(e.rates[tier])
		e.buckets[tier] = tb
	}
	e.mu.Unlock()
	return tb.allow()
}

// maxIPEntries caps the number of tracked IPs to prevent memory exhaustion
// from spoofed X-Forwarded-For flooding. At capacity, new IPs are rejected
// (fail-closed) until cleanup evicts stale entries.
const maxIPEntries = 100_000

// IPRateLimiter provides per-IP, per-tier rate limiting.
type IPRateLimiter struct {
	entries    *cache.ShardMap[*ipEntry]
	rates      [filter.NumTiers]int
	size       atomic.Int64
	maxEntries int64
	disabled   bool
}

// New creates a new IPRateLimiter with the given per-tier rates (requests per second per IP).
func New(rates [filter.NumTiers]int, disabled bool) *IPRateLimiter {
	return &IPRateLimiter{
		entries:    cache.NewShardMap[*ipEntry](),
		rates:      rates,
		maxEntries: maxIPEntries,
		disabled:   disabled,
	}
}

// Allow checks if a request from the given IP at the given tier is allowed.
func (l *IPRateLimiter) Allow(ip string, tier filter.Tier) bool {
	if l.disabled {
		return true
	}
	// Fast path: existing IP
	if v, ok := l.entries.Load(ip); ok {
		v.lastSeen.Store(time.Now().UnixNano())
		return v.allow(tier)
	}

	// New IP — check capacity to prevent memory exhaustion
	if l.size.Load() >= l.maxEntries {
		return false
	}

	entry := &ipEntry{rates: &l.rates}
	entry.lastSeen.Store(time.Now().UnixNano())

	actual, loaded := l.entries.LoadOrStore(ip, entry)
	if !loaded {
		l.size.Add(1)
	}
	actual.lastSeen.Store(time.Now().UnixNano())
	return actual.allow(tier)
}

// UpdateRates replaces the per-tier rate limits and clears all existing IP
// entries so that new token buckets are created with the updated rates.
// Brief disruption: all IPs get their burst refilled on next request.
func (l *IPRateLimiter) UpdateRates(rates [filter.NumTiers]int) {
	l.rates = rates
	// Clear all entries — they'll be lazily recreated with new rates.
	l.entries.RangeDelete(func(_ string, _ *ipEntry) bool { return true })
	l.size.Store(0)
}

// SetDisabled enables or disables rate limiting.
func (l *IPRateLimiter) SetDisabled(disabled bool) {
	l.disabled = disabled
}

// StartCleanup periodically removes stale IP entries (not seen in 5 minutes).
func (l *IPRateLimiter) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-5 * time.Minute).UnixNano()
			var removed int64
			l.entries.RangeDelete(func(_ string, entry *ipEntry) bool {
				if entry.lastSeen.Load() < cutoff {
					removed++
					return true
				}
				return false
			})
			l.size.Add(-removed)
		}
	}
}
