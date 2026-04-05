package filter

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tezos-commons/rpc-proxy/tracker"
)

// NeedsTezosArchive returns true if the Tezos request path references a
// historical block that a rolling node likely cannot serve.
func NeedsTezosArchive(path string, rb *tracker.RecentBlocks) bool {
	const seg = "/blocks/"
	idx := strings.Index(path, seg)
	if idx < 0 {
		return false // no block reference (monitor, injection, config, etc.)
	}

	blockID := path[idx+len(seg):]
	if slashIdx := strings.IndexByte(blockID, '/'); slashIdx >= 0 {
		blockID = blockID[:slashIdx]
	}
	if blockID == "" {
		return false
	}

	// "head" or relative to head — any node
	if blockID == "head" || strings.HasPrefix(blockID, "head~") || strings.HasPrefix(blockID, "head-") {
		return false
	}

	// Numeric level — check against recent window
	if level, err := strconv.ParseInt(blockID, 10, 64); err == nil {
		return level < rb.Head()-rb.Window()
	}

	// Block hash — check recent set
	if rb.ContainsHash(blockID) {
		return false
	}

	// Unknown reference (hash not in recent set, "genesis", etc.) — archive
	return true
}

// evmBlockTagIndex maps JSON-RPC methods to the parameter index containing
// a block tag. Methods not in this map don't reference blocks.
var evmBlockTagIndex = map[string]int{
	"eth_getBalance":                            1,
	"eth_getCode":                               1,
	"eth_getTransactionCount":                   1,
	"eth_getStorageAt":                          2,
	"eth_call":                                  1,
	"eth_estimateGas":                           1,
	"eth_getBlockByNumber":                      0,
	"eth_getBlockTransactionCountByNumber":      0,
	"eth_getUncleCountByBlockNumber":            0,
	"eth_getUncleByBlockNumberAndIndex":         0,
	"eth_getTransactionByBlockNumberAndIndex":   0,
	"eth_feeHistory":                            1,
}

// NeedsEVMArchive returns true if the EVM JSON-RPC request references a
// historical block that a rolling node likely cannot serve.
// method and params come from cache.EVMRequestInfo (already parsed by gjson).
func NeedsEVMArchive(method, params string, rb *tracker.RecentBlocks) bool {
	if method == "eth_getLogs" {
		return evmLogsNeedsArchive(params, rb)
	}

	idx, ok := evmBlockTagIndex[method]
	if !ok {
		return false
	}

	tag := gjson.Get(params, strconv.Itoa(idx)).Str
	return isArchiveBlockTag(tag, rb)
}

// isArchiveBlockTag returns true if the block tag references a historical block.
func isArchiveBlockTag(tag string, rb *tracker.RecentBlocks) bool {
	switch tag {
	case "", "latest", "pending", "safe", "finalized":
		return false
	case "earliest":
		return true
	default:
		// Hex block number — parse and compare to head
		level, err := strconv.ParseInt(strings.TrimPrefix(tag, "0x"), 16, 64)
		if err != nil {
			return true // unparseable — be conservative
		}
		return level < rb.Head()-rb.Window()
	}
}

// evmLogsNeedsArchive checks eth_getLogs filter object for historical block refs.
func evmLogsNeedsArchive(params string, rb *tracker.RecentBlocks) bool {
	if params == "" {
		return false
	}
	fromBlock := gjson.Get(params, "0.fromBlock").Str
	toBlock := gjson.Get(params, "0.toBlock").Str
	return isArchiveBlockTag(fromBlock, rb) || isArchiveBlockTag(toBlock, rb)
}

