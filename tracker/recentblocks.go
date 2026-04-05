package tracker

import "sync"

const DefaultBlockWindow = 500

// RecentBlocks stores the last N (level, hash) pairs for a network.
// Used to determine whether a block reference is recent enough for
// a rolling node, or requires an archive node.
// Thread-safe via RWMutex.
type RecentBlocks struct {
	mu         sync.RWMutex
	byLevel    map[int64]string  // level → hash
	byHash     map[string]int64  // hash → level
	head       int64             // highest level seen
	minLevel   int64             // lowest level in the map (for fast prune check)
	window     int64
	compactCtr int64             // compact maps every N prunes to release bucket memory
}

func NewRecentBlocks(window int64) *RecentBlocks {
	if window <= 0 {
		window = DefaultBlockWindow
	}
	return &RecentBlocks{
		byLevel: make(map[int64]string, window),
		byHash:  make(map[string]int64, window),
		window:  window,
	}
}

// Add records a (level, hash) pair. Handles reorgs: if the level already
// exists with a different hash, all entries at level >= this one are removed
// (old fork). Then prunes entries older than head - window.
func (rb *RecentBlocks) Add(level int64, hash string) {
	if hash == "" {
		return
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Reorg detection: if this level has a different hash, flush the old fork
	if existing, ok := rb.byLevel[level]; ok && existing != hash {
		rb.removeFrom(level)
	}

	rb.byLevel[level] = hash
	rb.byHash[hash] = level

	if level > rb.head {
		rb.head = level
	}
	if rb.minLevel == 0 || level < rb.minLevel {
		rb.minLevel = level
	}

	// Prune old entries — only when map grows 10% beyond window to amortize
	// the iteration cost and reduce write lock hold time.
	cutoff := rb.head - rb.window
	if rb.minLevel < cutoff && int64(len(rb.byLevel)) > rb.window+rb.window/10 {
		newMin := rb.head
		for l, h := range rb.byLevel {
			if l < cutoff {
				delete(rb.byLevel, l)
				delete(rb.byHash, h)
			} else if l < newMin {
				newMin = l
			}
		}
		rb.minLevel = newMin

		// Compact maps periodically to release retained bucket memory.
		// Go maps never shrink their bucket array after deletes.
		rb.compactCtr++
		if rb.compactCtr%500 == 0 {
			fresh := make(map[int64]string, len(rb.byLevel))
			for k, v := range rb.byLevel {
				fresh[k] = v
			}
			rb.byLevel = fresh

			freshH := make(map[string]int64, len(rb.byHash))
			for k, v := range rb.byHash {
				freshH[k] = v
			}
			rb.byHash = freshH
		}
	}
}

// removeFrom removes all entries with level >= the given level.
// Must be called with mu held.
func (rb *RecentBlocks) removeFrom(level int64) {
	for l, h := range rb.byLevel {
		if l >= level {
			delete(rb.byLevel, l)
			delete(rb.byHash, h)
		}
	}
}

// ContainsHash returns true if the hash is in the recent blocks set.
func (rb *RecentBlocks) ContainsHash(hash string) bool {
	rb.mu.RLock()
	_, ok := rb.byHash[hash]
	rb.mu.RUnlock()
	return ok
}

// Head returns the highest level in the recent blocks set.
func (rb *RecentBlocks) Head() int64 {
	rb.mu.RLock()
	h := rb.head
	rb.mu.RUnlock()
	return h
}

// Window returns the configured block window size.
func (rb *RecentBlocks) Window() int64 {
	return rb.window
}
