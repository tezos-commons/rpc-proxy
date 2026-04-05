package cache

import (
	"fmt"
	"sync"
	"testing"
)

func TestShardMap_StoreAndLoad(t *testing.T) {
	sm := NewShardMap[int]()

	sm.Store("a", 1)
	sm.Store("b", 2)

	v, ok := sm.Load("a")
	if !ok || v != 1 {
		t.Fatalf("Load(a) = %d, %v; want 1, true", v, ok)
	}

	v, ok = sm.Load("b")
	if !ok || v != 2 {
		t.Fatalf("Load(b) = %d, %v; want 2, true", v, ok)
	}

	_, ok = sm.Load("c")
	if ok {
		t.Fatal("Load(c) should return false for missing key")
	}
}

func TestShardMap_Delete(t *testing.T) {
	sm := NewShardMap[string]()
	sm.Store("x", "val")

	sm.Delete("x")
	_, ok := sm.Load("x")
	if ok {
		t.Fatal("expected key to be deleted")
	}

	// Delete non-existent key should not panic
	sm.Delete("nonexistent")
}

func TestShardMap_LoadOrStore(t *testing.T) {
	sm := NewShardMap[int]()

	// First call stores the value
	actual, loaded := sm.LoadOrStore("k", 10)
	if loaded {
		t.Fatal("expected loaded=false on first call")
	}
	if actual != 10 {
		t.Fatalf("actual = %d; want 10", actual)
	}

	// Second call returns existing value
	actual, loaded = sm.LoadOrStore("k", 99)
	if !loaded {
		t.Fatal("expected loaded=true on second call")
	}
	if actual != 10 {
		t.Fatalf("actual = %d; want 10 (original)", actual)
	}
}

func TestShardMap_Len(t *testing.T) {
	sm := NewShardMap[int]()
	if sm.Len() != 0 {
		t.Fatalf("empty map Len = %d; want 0", sm.Len())
	}

	for i := 0; i < 100; i++ {
		sm.Store(fmt.Sprintf("key-%d", i), i)
	}
	if sm.Len() != 100 {
		t.Fatalf("Len = %d; want 100", sm.Len())
	}

	sm.Delete("key-0")
	sm.Delete("key-50")
	if sm.Len() != 98 {
		t.Fatalf("Len = %d; want 98 after deleting 2", sm.Len())
	}
}

func TestShardMap_Range(t *testing.T) {
	sm := NewShardMap[int]()
	for i := 0; i < 10; i++ {
		sm.Store(fmt.Sprintf("k%d", i), i)
	}

	seen := make(map[string]int)
	sm.Range(func(key string, value int) bool {
		seen[key] = value
		return true
	})
	if len(seen) != 10 {
		t.Fatalf("Range visited %d entries; want 10", len(seen))
	}
}

func TestShardMap_RangeEarlyStop(t *testing.T) {
	sm := NewShardMap[int]()
	for i := 0; i < 1000; i++ {
		sm.Store(fmt.Sprintf("k%d", i), i)
	}

	count := 0
	sm.Range(func(key string, value int) bool {
		count++
		return count < 5
	})
	if count != 5 {
		t.Fatalf("Range stopped after %d; want 5", count)
	}
}

func TestShardMap_RangeDelete(t *testing.T) {
	sm := NewShardMap[int]()
	for i := 0; i < 20; i++ {
		sm.Store(fmt.Sprintf("k%d", i), i)
	}

	// Delete even-valued entries
	sm.RangeDelete(func(_ string, v int) bool {
		return v%2 == 0
	})

	if sm.Len() != 10 {
		t.Fatalf("Len after RangeDelete = %d; want 10", sm.Len())
	}

	// Verify only odd values remain
	sm.Range(func(_ string, v int) bool {
		if v%2 == 0 {
			t.Fatalf("even value %d should have been deleted", v)
		}
		return true
	})
}

func TestShardMap_ConcurrentReadWrite(t *testing.T) {
	sm := NewShardMap[int]()
	const goroutines = 50
	const ops = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("g%d-k%d", g, i)
				sm.Store(key, i)
			}
		}(g)
	}

	// Readers
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("g%d-k%d", g, i)
				sm.Load(key)
			}
		}(g)
	}

	wg.Wait()
	// Just verifying no race/panic
}

func TestShardMap_ConcurrentLoadOrStore(t *testing.T) {
	sm := NewShardMap[int]()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	wins := make([]bool, goroutines)

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			_, loaded := sm.LoadOrStore("race-key", g)
			if !loaded {
				wins[g] = true
			}
		}(g)
	}

	wg.Wait()

	// Exactly one goroutine should have "won" (loaded=false)
	winCount := 0
	for _, w := range wins {
		if w {
			winCount++
		}
	}
	if winCount != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winCount)
	}
}
