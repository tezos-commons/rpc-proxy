package cache

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// HeaderPair stores a single HTTP header key-value pair.
type HeaderPair struct {
	Key   []byte
	Value []byte
}

// Entry stores a cached response. For generic (Tezos) responses, Body/GzipBody
// are used. For EVM JSON-RPC, Result/Error are stored separately so the id
// field can be patched without re-parsing.
type Entry struct {
	generation int64
	Status     int
	Headers    []HeaderPair
	Body       []byte // raw response body (Tezos) or full response with null id (EVM)
	GzipBody   []byte // pre-compressed gzip of Body

	// EVM-specific: pre-parsed result/error for id-patching without re-parsing
	EVMResult json.RawMessage
	EVMError  json.RawMessage
}

// NetworkCache caches responses per network with generation-based invalidation.
type NetworkCache struct {
	entries    *ShardMap[*Entry]
	seen       *ShardMap[int64]
	size       atomic.Int64
	seenSize   atomic.Int64
	maxSize    int64
	generation *atomic.Int64 // pointer to balancer's headGen
	Flights    singleflight.Group
}

func New(headGen *atomic.Int64, maxSize int64) *NetworkCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &NetworkCache{
		entries:    NewShardMap[*Entry](),
		seen:       NewShardMap[int64](),
		maxSize:    maxSize,
		generation: headGen,
	}
}

// Get returns a cached entry if it exists and is still valid (same generation).
func (c *NetworkCache) Get(key string) (*Entry, bool) {
	e, ok := c.entries.Load(key)
	if !ok {
		return nil, false
	}
	if e.generation != c.generation.Load() {
		return nil, false
	}
	return e, true
}

// maxSeenEntries caps the seen map to prevent memory exhaustion from
// unique-key flooding. Generous limit — 32x the cache size.
const maxSeenMultiplier = 32

// ShouldCache marks a key as seen and returns true if this is the second+
// time the key is requested in the current generation.
func (c *NetworkCache) ShouldCache(key string) bool {
	gen := c.generation.Load()

	v, loaded := c.seen.LoadOrStore(key, gen)
	if !loaded {
		// New key — check if seen map is over capacity
		if c.seenSize.Add(1) > c.maxSize*maxSeenMultiplier {
			c.seenSize.Add(-1)
			c.seen.Delete(key)
		}
		return false
	}

	if v != gen {
		c.seen.Store(key, gen)
		return false
	}

	return true
}

// Set stores a response in the cache. Only caches 2xx responses.
// Takes ownership of headers and body — caller must not reuse them.
func (c *NetworkCache) Set(key string, e *Entry) {
	if e.Status < 200 || e.Status >= 300 {
		return
	}

	// Optimistic size reservation — prevents concurrent goroutines from
	// overshooting maxSize, which the old Load+Add pattern allowed.
	newSize := c.size.Add(1)
	if newSize > c.maxSize {
		c.size.Add(-1)
		return
	}

	e.generation = c.generation.Load()
	if len(e.Body) >= gzipMinSize {
		e.GzipBody = Compress(e.Body)
	}

	if _, loaded := c.entries.LoadOrStore(key, e); loaded {
		// Key already existed — undo the reservation
		c.size.Add(-1)
	}
}

// SetMaxSize updates the maximum number of cache entries.
// If the new size is smaller, excess entries are evicted on the next sweep.
func (c *NetworkCache) SetMaxSize(maxSize int64) {
	if maxSize <= 0 {
		maxSize = 10000
	}
	c.maxSize = maxSize
}

// Generation returns the current cache generation.
func (c *NetworkCache) Generation() int64 {
	return c.generation.Load()
}

// StartSweep periodically removes stale entries and seen markers.
func (c *NetworkCache) StartSweep(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gen := c.generation.Load()
			var removed int64
			c.entries.RangeDeleteAndCompact(func(_ string, e *Entry) bool {
				if e.generation != gen {
					removed++
					return true
				}
				return false
			})
			c.size.Add(-removed)

			// Reconcile size counter to prevent drift from concurrent races
			actualSize := int64(c.entries.Len())
			c.size.Store(actualSize)

			var seenRemoved int64
			c.seen.RangeDeleteAndCompact(func(_ string, v int64) bool {
				if v != gen {
					seenRemoved++
					return true
				}
				return false
			})
			c.seenSize.Add(-seenRemoved)
			actualSeenSize := int64(c.seen.Len())
			c.seenSize.Store(actualSeenSize)
		}
	}
}
