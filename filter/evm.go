package filter

// CheckEVMPath checks if an EVM REST path (after /etherlink/<network>) is allowed.
// Most EVM traffic is JSON-RPC to "/", but we block private REST paths.
func CheckEVMPath(path string) bool {
	// Block private JSON-RPC endpoint
	if hasPrefix(path, "/private") {
		return false
	}
	// Block inter-node peer services
	if hasPrefix(path, "/evm/") || path == "/evm" {
		return false
	}
	// Block config/mode/metrics leaks
	if path == "/configuration" || path == "/mode" || path == "/metrics" {
		return false
	}
	return true
}

// Private JSON-RPC methods that must never be exposed.
var evmDeniedMethods = map[string]struct{}{
	"produceBlock":                {},
	"proposeNextBlockTimestamp":   {},
	"produceProposal":            {},
	"executeSingleTransaction":   {},
	"injectTransaction":          {},
	"waitTransactionConfirmation": {},
	"injectTezlinkOperation":     {},
	"lockBlockProduction":        {},
	"unlockBlockProduction":      {},
}

// EVM methods that are CPU-intensive (debug tracing).
var evmDebugMethods = map[string]struct{}{
	"debug_traceTransaction":   {},
	"debug_traceCall":          {},
	"debug_traceBlockByNumber": {},
	"http_traceCall":           {},
	"tez_replayBlock":          {},
}

// EVM methods that are expensive (state queries).
var evmExpensiveMethods = map[string]struct{}{
	"eth_call":        {},
	"eth_estimateGas": {},
	"eth_getLogs":     {},
	"txpool_content":  {},
}

// EVM methods that involve transaction injection.
var evmInjectionMethods = map[string]struct{}{
	"eth_sendRawTransaction":     {},
	"eth_sendRawTransactionSync": {},
	"tez_sendRawTezlinkOperation": {},
}

// CheckEVMMethod returns whether a JSON-RPC method is allowed,
// and if so, which rate-limit tier applies.
func CheckEVMMethod(method string) (denied bool, tier Tier) {
	if _, ok := evmDeniedMethods[method]; ok {
		return true, 0
	}
	if _, ok := evmDebugMethods[method]; ok {
		return false, TierDebug
	}
	if _, ok := evmExpensiveMethods[method]; ok {
		return false, TierExpensive
	}
	if _, ok := evmInjectionMethods[method]; ok {
		return false, TierInjection
	}
	return false, TierDefault
}
