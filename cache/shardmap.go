package cache

import (
	"sync"

	"github.com/cespare/xxhash/v2"
)

const numShards = 256

type shard[V any] struct {
	mu    sync.RWMutex
	items map[string]V
}

// ShardMap is a concurrent map sharded by key hash.
// Much better than sync.Map for high-churn workloads.
type ShardMap[V any] struct {
	shards [numShards]shard[V]
}

func NewShardMap[V any]() *ShardMap[V] {
	sm := &ShardMap[V]{}
	for i := range sm.shards {
		sm.shards[i].items = make(map[string]V)
	}
	return sm
}

func (m *ShardMap[V]) getShard(key string) *shard[V] {
	return &m.shards[xxhash.Sum64String(key)%numShards]
}

func (m *ShardMap[V]) Load(key string) (V, bool) {
	s := m.getShard(key)
	s.mu.RLock()
	v, ok := s.items[key]
	s.mu.RUnlock()
	return v, ok
}

func (m *ShardMap[V]) Store(key string, value V) {
	s := m.getShard(key)
	s.mu.Lock()
	s.items[key] = value
	s.mu.Unlock()
}

// LoadOrStore returns the existing value if present, otherwise stores and returns the given value.
// The loaded return value is true if the value was already present.
func (m *ShardMap[V]) LoadOrStore(key string, value V) (actual V, loaded bool) {
	s := m.getShard(key)
	s.mu.Lock()
	if v, ok := s.items[key]; ok {
		s.mu.Unlock()
		return v, true
	}
	s.items[key] = value
	s.mu.Unlock()
	return value, false
}

func (m *ShardMap[V]) Delete(key string) {
	s := m.getShard(key)
	s.mu.Lock()
	delete(s.items, key)
	s.mu.Unlock()
}

// Range calls fn for each key-value pair. If fn returns false, iteration stops.
// Acquires read locks per shard — do not call Store/Delete from fn.
func (m *ShardMap[V]) Range(fn func(key string, value V) bool) {
	for i := range m.shards {
		s := &m.shards[i]
		s.mu.RLock()
		for k, v := range s.items {
			if !fn(k, v) {
				s.mu.RUnlock()
				return
			}
		}
		s.mu.RUnlock()
	}
}

// RangeDelete calls fn for each key-value pair and deletes entries where fn returns true.
// Acquires write locks per shard.
func (m *ShardMap[V]) RangeDelete(fn func(key string, value V) bool) {
	for i := range m.shards {
		s := &m.shards[i]
		s.mu.Lock()
		for k, v := range s.items {
			if fn(k, v) {
				delete(s.items, k)
			}
		}
		s.mu.Unlock()
	}
}

// RangeDeleteAndCompact calls fn for each key-value pair, deletes entries where
// fn returns true, and compacts only the shards that had deletions to release
// memory. Combines two passes into one, reducing total lock acquisitions.
func (m *ShardMap[V]) RangeDeleteAndCompact(fn func(key string, value V) bool) {
	for i := range m.shards {
		s := &m.shards[i]
		s.mu.Lock()
		dirty := false
		for k, v := range s.items {
			if fn(k, v) {
				delete(s.items, k)
				dirty = true
			}
		}
		if dirty {
			fresh := make(map[string]V, len(s.items))
			for k, v := range s.items {
				fresh[k] = v
			}
			s.items = fresh
		}
		s.mu.Unlock()
	}
}

func (m *ShardMap[V]) Len() int {
	n := 0
	for i := range m.shards {
		s := &m.shards[i]
		s.mu.RLock()
		n += len(s.items)
		s.mu.RUnlock()
	}
	return n
}
