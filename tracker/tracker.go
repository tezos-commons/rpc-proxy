package tracker

import (
	"sync/atomic"
	"time"
)

// StaleAfter is how long after the last successful update a node is still
// considered healthy. Generous to survive brief monitor/poll interruptions.
// Tezos blocks are ~15s, EVM ~2-4s — 60s covers several missed updates.
const StaleAfter = 60 * time.Second

type NodeStatus struct {
	Name    string
	URL     string
	Archive bool // set at construction, immutable

	head      atomic.Int64
	updatedAt atomic.Int64 // unix nano
}

func NewNodeStatus(name, url string, archive bool) *NodeStatus {
	return &NodeStatus{Name: name, URL: url, Archive: archive}
}

func (n *NodeStatus) Update(head int64) {
	now := time.Now().UnixNano()
	n.head.Store(head)
	n.updatedAt.Store(now)
}

// SetUnhealthy is a no-op — health is determined by staleness.
// Kept for API compatibility.
func (n *NodeStatus) SetUnhealthy() {}

// ForceUpdatedAt sets the updatedAt timestamp directly. Test helper only.
func (n *NodeStatus) ForceUpdatedAt(unixNano int64) {
	n.updatedAt.Store(unixNano)
}

// GetHead returns the node's last known head and whether it's healthy.
// A node is healthy if it has ever reported a head and the last update
// was within StaleAfter.
func (n *NodeStatus) GetHead() (int64, bool) {
	updated := n.updatedAt.Load()
	if updated == 0 {
		return 0, false
	}
	healthy := time.Since(time.Unix(0, updated)) < StaleAfter
	return n.head.Load(), healthy
}
