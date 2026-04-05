package filter

import "strings"

// CheckTezos returns whether a request to the given upstream path is allowed,
// and if so, which rate-limit tier applies.
// httpMethod is the HTTP method (GET, POST, etc.).
// path is the upstream path after stripping /tezos/<network>.
func CheckTezos(httpMethod, path string) (denied bool, tier Tier) {
	if isTezosBlocked(httpMethod, path) {
		return true, 0
	}
	return false, classifyTezos(httpMethod, path)
}

func isTezosBlocked(method, path string) bool {
	// --- Shell RPCs: absolute path blocks ---

	// All P2P/peer endpoints
	if hasPrefix(path, "/network/") || path == "/network" {
		return true
	}
	// Internal worker diagnostics
	if hasPrefix(path, "/workers/") || path == "/workers" {
		return true
	}
	// Memory/GC stats
	if hasPrefix(path, "/stats/") || path == "/stats" {
		return true
	}
	// GC trigger
	if hasPrefix(path, "/gc/") || path == "/gc" {
		return true
	}
	// Full config dump (but allow /config/network/*, /config/history_mode)
	if path == "/config" && method == "GET" {
		return true
	}
	// Logging mutation
	if path == "/config/logging" && method == "PUT" {
		return true
	}
	// Private injection endpoints
	if hasPrefix(path, "/private/") {
		return true
	}
	// Block and protocol injection
	if path == "/injection/block" {
		return true
	}
	if path == "/injection/protocol" {
		return true
	}
	// Network fetch trigger
	if hasPrefix(path, "/fetch_protocol/") {
		return true
	}
	// Monitor received blocks (DoS vector)
	if hasPrefix(path, "/monitor/received_blocks/") {
		return true
	}
	// Force bootstrap flag
	if method == "PATCH" && matchChainRoot(path) {
		return true
	}
	// Active peers heads (leaks peer info)
	if containsSegment(path, "/active_peers_heads") {
		return true
	}
	// Invalid blocks DELETE
	if method == "DELETE" && containsSegment(path, "/invalid_blocks/") {
		return true
	}
	// Invalid blocks listing (internal diagnostics)
	if hasSuffix(path, "/invalid_blocks") {
		return true
	}
	// Mempool mutations
	if containsSegment(path, "/mempool/ban_operation") ||
		containsSegment(path, "/mempool/unban_operation") ||
		containsSegment(path, "/mempool/unban_all_operations") ||
		containsSegment(path, "/mempool/request_operations") {
		return true
	}
	// Mempool filter POST
	if method == "POST" && containsSegment(path, "/mempool/filter") {
		return true
	}

	// --- Protocol RPCs: under /chains/*/blocks/*/ ---

	// Seed reveal
	if method == "POST" && containsSegment(path, "/context/seed") {
		return true
	}
	// Internal cache state
	if containsSegment(path, "/context/cache/contracts/all") ||
		containsSegment(path, "/context/cache/contracts/size") {
		return true
	}
	if method == "POST" && containsSegment(path, "/context/cache/contracts/rank") {
		return true
	}

	return false
}

func classifyTezos(method, path string) Tier {
	// Streaming endpoints
	if hasPrefix(path, "/monitor/") {
		return TierStreaming
	}
	if containsSegment(path, "/mempool/monitor_operations") {
		return TierStreaming
	}

	// Operation injection
	if path == "/injection/operation" && method == "POST" {
		return TierInjection
	}

	// Script execution (CPU-intensive)
	if containsSegment(path, "/helpers/scripts/run_code") ||
		containsSegment(path, "/helpers/scripts/trace_code") ||
		containsSegment(path, "/helpers/scripts/run_view") ||
		containsSegment(path, "/helpers/scripts/run_script_view") ||
		containsSegment(path, "/helpers/scripts/run_instruction") ||
		containsSegment(path, "/helpers/scripts/typecheck_code") ||
		containsSegment(path, "/helpers/scripts/typecheck_data") ||
		containsSegment(path, "/helpers/scripts/run_operation") ||
		containsSegment(path, "/helpers/scripts/simulate_operation") {
		return TierScript
	}

	// Expensive queries
	if containsSegment(path, "/context/raw/bytes") ||
		containsSegment(path, "/context/merkle_tree") ||
		containsSegment(path, "/context/merkle_tree_v2") ||
		containsSegment(path, "/helpers/preapply/") ||
		containsSegment(path, "/context/sapling/") {
		return TierExpensive
	}
	// Big maps
	if containsSegment(path, "/context/big_maps/") {
		return TierExpensive
	}
	// List all contracts / delegates (large result)
	if hasSuffix(path, "/context/contracts") || hasSuffix(path, "/context/delegates") {
		return TierExpensive
	}

	return TierDefault
}

// matchChainRoot returns true for paths like /chains/main (but not /chains/main/...)
func matchChainRoot(path string) bool {
	if !hasPrefix(path, "/chains/") {
		return false
	}
	rest := path[len("/chains/"):]
	return !strings.Contains(rest, "/")
}

func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func hasSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

func containsSegment(path, segment string) bool {
	return strings.Contains(path, segment)
}
