package balancer

import (
	"sync/atomic"

	"github.com/tezos-commons/rpc-proxy/tracker"
)

// Balancer selects the best node based on highest head level.
// When multiple nodes share the highest head, it round-robins among them.
// It also tracks a generation counter that increments on each head change,
// used by the cache layer for invalidation.
type Balancer struct {
	nodes   []*tracker.NodeStatus
	rrIndex atomic.Uint64
	_       [56]byte // cache line pad — rrIndex is written every request;
	//                   headGen is read every cache lookup. Without padding
	//                   they share a 64-byte cache line causing false sharing.

	lastHead atomic.Int64 // last observed max head
	headGen  atomic.Int64 // bumped when head advances
}

func New(nodes []*tracker.NodeStatus) *Balancer {
	return &Balancer{
		nodes: nodes,
	}
}

// HeadGeneration returns the current head generation counter.
func (b *Balancer) HeadGeneration() int64 {
	return b.headGen.Load()
}

// NotifyHead advances the head generation if head is higher than the last
// observed maximum. Called by trackers when they detect a new block, so
// that cache invalidation is not coupled to Pick().
func (b *Balancer) NotifyHead(head int64) {
	if old := b.lastHead.Load(); head > old {
		if b.lastHead.CompareAndSwap(old, head) {
			b.headGen.Add(1)
		}
	}
}

// HeadGenerationPtr returns a pointer to the head generation atomic,
// for use by the cache layer.
func (b *Balancer) HeadGenerationPtr() *atomic.Int64 {
	return &b.headGen
}

// maxNodesPerNetwork is the max candidates tracked in a single pass.
// Stack-allocated, avoids heap allocation. Increase if networks grow beyond this.
const maxNodesPerNetwork = 16

// Pick returns the best available node, or nil if no healthy node exists.
// When archiveOnly is true, only archive nodes are considered.
// Single pass with stack-allocated candidate collection — no race between passes.
// Also bumps head generation if the max head has advanced.
func (b *Balancer) Pick(archiveOnly bool) *tracker.NodeStatus {
	var maxHead int64 = -1
	var candidates [maxNodesPerNetwork]*tracker.NodeStatus
	var count int

	for _, n := range b.nodes {
		if archiveOnly && !n.Archive {
			continue
		}
		head, healthy := n.GetHead()
		if !healthy {
			continue
		}
		if head > maxHead {
			maxHead = head
			candidates[0] = n
			count = 1
		} else if head == maxHead && count < maxNodesPerNetwork {
			candidates[count] = n
			count++
		}
	}

	if count == 0 {
		return nil
	}

	// Bump generation if head advanced
	if old := b.lastHead.Load(); maxHead > old {
		if b.lastHead.CompareAndSwap(old, maxHead) {
			b.headGen.Add(1)
		}
	}

	idx := b.rrIndex.Add(1) - 1
	return candidates[idx%uint64(count)]
}

// Nodes returns the underlying node list.
func (b *Balancer) Nodes() []*tracker.NodeStatus {
	return b.nodes
}
