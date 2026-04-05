package filter

import (
	"fmt"
	"testing"

	"github.com/tezos-commons/rpc-proxy/tracker"
)

// ---------------------------------------------------------------------------
// CheckTezos
// ---------------------------------------------------------------------------

func TestCheckTezos_BlockedPaths(t *testing.T) {
	blocked := []struct {
		method string
		path   string
	}{
		// Network / P2P
		{"GET", "/network"},
		{"GET", "/network/peers"},
		{"GET", "/network/connections"},
		// Workers
		{"GET", "/workers"},
		{"GET", "/workers/block_validator"},
		// Stats / GC
		{"GET", "/stats"},
		{"GET", "/stats/memory"},
		{"GET", "/gc"},
		{"GET", "/gc/trigger"},
		// Config dump (GET only)
		{"GET", "/config"},
		// Logging mutation
		{"PUT", "/config/logging"},
		// Private
		{"GET", "/private/anything"},
		{"POST", "/private/inject"},
		// Block / protocol injection
		{"POST", "/injection/block"},
		{"POST", "/injection/protocol"},
		// Fetch protocol
		{"GET", "/fetch_protocol/abc"},
		// Monitor received blocks
		{"GET", "/monitor/received_blocks/xyz"},
		// Force bootstrap (PATCH on chain root)
		{"PATCH", "/chains/main"},
		{"PATCH", "/chains/test"},
		// Active peers heads
		{"GET", "/chains/main/active_peers_heads"},
		// Invalid blocks
		{"DELETE", "/chains/main/blocks/head/invalid_blocks/hash"},
		{"GET", "/chains/main/blocks/head/invalid_blocks"},
		// Mempool mutations
		{"POST", "/chains/main/mempool/ban_operation"},
		{"POST", "/chains/main/mempool/unban_operation"},
		{"POST", "/chains/main/mempool/unban_all_operations"},
		{"POST", "/chains/main/mempool/request_operations"},
		// Mempool filter POST
		{"POST", "/chains/main/mempool/filter"},
		// Seed reveal
		{"POST", "/chains/main/blocks/head/context/seed"},
		// Cache internals
		{"GET", "/chains/main/blocks/head/context/cache/contracts/all"},
		{"GET", "/chains/main/blocks/head/context/cache/contracts/size"},
		{"POST", "/chains/main/blocks/head/context/cache/contracts/rank"},
	}

	for _, tc := range blocked {
		denied, _ := CheckTezos(tc.method, tc.path)
		if !denied {
			t.Errorf("expected blocked: %s %s", tc.method, tc.path)
		}
	}
}

func TestCheckTezos_AllowedPaths(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{"GET", "/chains/main/blocks/head/header"},
		{"GET", "/chains/main/blocks/head/operations"},
		{"GET", "/chains/main/blocks/head/context/contracts/tz1abc"},
		{"POST", "/injection/operation"},
		{"GET", "/monitor/heads/main"},
		{"GET", "/chains/main/mempool/filter"},              // GET is fine
		{"GET", "/config/network/user_activated_upgrades"},   // sub-path of /config
		{"POST", "/config"},                                   // POST /config not blocked
		{"GET", "/chains/main/blocks/head/context/delegates/tz1abc"},
		{"GET", "/version"},
	}

	for _, tc := range allowed {
		denied, _ := CheckTezos(tc.method, tc.path)
		if denied {
			t.Errorf("expected allowed: %s %s", tc.method, tc.path)
		}
	}
}

func TestCheckTezos_TierClassification(t *testing.T) {
	cases := []struct {
		method string
		path   string
		tier   Tier
	}{
		// Streaming
		{"GET", "/monitor/heads/main", TierStreaming},
		{"GET", "/monitor/bootstrapped", TierStreaming},
		{"GET", "/chains/main/mempool/monitor_operations", TierStreaming},
		// Injection
		{"POST", "/injection/operation", TierInjection},
		// Script
		{"POST", "/chains/main/blocks/head/helpers/scripts/run_code", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/trace_code", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/run_view", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/run_script_view", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/run_instruction", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/typecheck_code", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/typecheck_data", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/run_operation", TierScript},
		{"POST", "/chains/main/blocks/head/helpers/scripts/simulate_operation", TierScript},
		// Expensive
		{"GET", "/chains/main/blocks/head/context/raw/bytes", TierExpensive},
		{"GET", "/chains/main/blocks/head/context/merkle_tree", TierExpensive},
		{"GET", "/chains/main/blocks/head/context/merkle_tree_v2", TierExpensive},
		{"POST", "/chains/main/blocks/head/helpers/preapply/operations", TierExpensive},
		{"GET", "/chains/main/blocks/head/context/sapling/something", TierExpensive},
		{"GET", "/chains/main/blocks/head/context/big_maps/123/key", TierExpensive},
		{"GET", "/chains/main/blocks/head/context/contracts", TierExpensive},
		{"GET", "/chains/main/blocks/head/context/delegates", TierExpensive},
		// Default
		{"GET", "/chains/main/blocks/head/header", TierDefault},
		{"GET", "/chains/main/blocks/head/operations", TierDefault},
		{"GET", "/version", TierDefault},
		// injection/operation GET is default, not injection tier
		{"GET", "/injection/operation", TierDefault},
	}

	for _, tc := range cases {
		denied, tier := CheckTezos(tc.method, tc.path)
		if denied {
			t.Errorf("unexpectedly blocked: %s %s", tc.method, tc.path)
			continue
		}
		if tier != tc.tier {
			t.Errorf("%s %s: got tier %v, want %v", tc.method, tc.path, tier, tc.tier)
		}
	}
}

// ---------------------------------------------------------------------------
// CheckEVMMethod
// ---------------------------------------------------------------------------

func TestCheckEVMMethod_Denied(t *testing.T) {
	denied := []string{
		"produceBlock",
		"proposeNextBlockTimestamp",
		"produceProposal",
		"executeSingleTransaction",
		"injectTransaction",
		"waitTransactionConfirmation",
		"injectTezlinkOperation",
		"lockBlockProduction",
		"unlockBlockProduction",
	}
	for _, m := range denied {
		got, _ := CheckEVMMethod(m)
		if !got {
			t.Errorf("expected denied: %s", m)
		}
	}
}

func TestCheckEVMMethod_DebugTier(t *testing.T) {
	methods := []string{
		"debug_traceTransaction",
		"debug_traceCall",
		"debug_traceBlockByNumber",
		"http_traceCall",
		"tez_replayBlock",
	}
	for _, m := range methods {
		denied, tier := CheckEVMMethod(m)
		if denied {
			t.Errorf("unexpectedly denied: %s", m)
			continue
		}
		if tier != TierDebug {
			t.Errorf("%s: got tier %v, want debug", m, tier)
		}
	}
}

func TestCheckEVMMethod_ExpensiveTier(t *testing.T) {
	methods := []string{"eth_call", "eth_estimateGas", "eth_getLogs", "txpool_content"}
	for _, m := range methods {
		denied, tier := CheckEVMMethod(m)
		if denied {
			t.Errorf("unexpectedly denied: %s", m)
			continue
		}
		if tier != TierExpensive {
			t.Errorf("%s: got tier %v, want expensive", m, tier)
		}
	}
}

func TestCheckEVMMethod_InjectionTier(t *testing.T) {
	methods := []string{
		"eth_sendRawTransaction",
		"eth_sendRawTransactionSync",
		"tez_sendRawTezlinkOperation",
	}
	for _, m := range methods {
		denied, tier := CheckEVMMethod(m)
		if denied {
			t.Errorf("unexpectedly denied: %s", m)
			continue
		}
		if tier != TierInjection {
			t.Errorf("%s: got tier %v, want injection", m, tier)
		}
	}
}

func TestCheckEVMMethod_DefaultTier(t *testing.T) {
	methods := []string{
		"eth_blockNumber",
		"eth_getBalance",
		"eth_chainId",
		"net_version",
		"web3_clientVersion",
	}
	for _, m := range methods {
		denied, tier := CheckEVMMethod(m)
		if denied {
			t.Errorf("unexpectedly denied: %s", m)
			continue
		}
		if tier != TierDefault {
			t.Errorf("%s: got tier %v, want default", m, tier)
		}
	}
}

// ---------------------------------------------------------------------------
// CheckEVMPath
// ---------------------------------------------------------------------------

func TestCheckEVMPath(t *testing.T) {
	cases := []struct {
		path    string
		allowed bool
	}{
		// Blocked
		{"/private", false},
		{"/private/inject", false},
		{"/evm", false},
		{"/evm/kernel_version", false},
		{"/configuration", false},
		{"/mode", false},
		{"/metrics", false},
		// Allowed
		{"/", true},
		{"/health", true},
		{"/some/other/path", true},
	}
	for _, tc := range cases {
		got := CheckEVMPath(tc.path)
		if got != tc.allowed {
			t.Errorf("CheckEVMPath(%q) = %v, want %v", tc.path, got, tc.allowed)
		}
	}
}

// ---------------------------------------------------------------------------
// NeedsTezosArchive
// ---------------------------------------------------------------------------

func newTezosTracker(head int64, window int64) *tracker.RecentBlocks {
	rb := tracker.NewRecentBlocks(window)
	// Add blocks from (head - window + 1) .. head
	for l := head - window + 1; l <= head; l++ {
		rb.Add(l, fmt.Sprintf("BL%d", l))
	}
	return rb
}

func TestNeedsTezosArchive_HeadReferences(t *testing.T) {
	rb := newTezosTracker(1000, 500)

	cases := []struct {
		path    string
		archive bool
	}{
		{"/chains/main/blocks/head/header", false},
		{"/chains/main/blocks/head~2/header", false},
		{"/chains/main/blocks/head-5/header", false},
	}
	for _, tc := range cases {
		got := NeedsTezosArchive(tc.path, rb)
		if got != tc.archive {
			t.Errorf("NeedsTezosArchive(%q) = %v, want %v", tc.path, got, tc.archive)
		}
	}
}

func TestNeedsTezosArchive_RecentLevels(t *testing.T) {
	rb := newTezosTracker(1000, 500)

	// Level within window: head(1000) - window(500) = 500, so 501 is recent
	if NeedsTezosArchive("/chains/main/blocks/900/header", rb) {
		t.Error("level 900 should not need archive")
	}
	if NeedsTezosArchive("/chains/main/blocks/501/header", rb) {
		t.Error("level 501 should not need archive (at window boundary)")
	}
}

func TestNeedsTezosArchive_OldLevels(t *testing.T) {
	rb := newTezosTracker(1000, 500)

	// Level 499 is below head - window = 500
	if !NeedsTezosArchive("/chains/main/blocks/499/header", rb) {
		t.Error("level 499 should need archive")
	}
	if !NeedsTezosArchive("/chains/main/blocks/1/header", rb) {
		t.Error("level 1 should need archive")
	}
}

func TestNeedsTezosArchive_KnownHash(t *testing.T) {
	rb := newTezosTracker(1000, 500)

	// "BL900" is a known recent hash
	if NeedsTezosArchive("/chains/main/blocks/BL900/header", rb) {
		t.Error("known hash BL900 should not need archive")
	}
}

func TestNeedsTezosArchive_UnknownHash(t *testing.T) {
	rb := newTezosTracker(1000, 500)

	if !NeedsTezosArchive("/chains/main/blocks/BLunknown/header", rb) {
		t.Error("unknown hash should need archive")
	}
}

func TestNeedsTezosArchive_NoBlockSegment(t *testing.T) {
	rb := newTezosTracker(1000, 500)

	// Paths without /blocks/ never need archive
	if NeedsTezosArchive("/monitor/heads/main", rb) {
		t.Error("/monitor path should not need archive")
	}
	if NeedsTezosArchive("/injection/operation", rb) {
		t.Error("/injection path should not need archive")
	}
}

// ---------------------------------------------------------------------------
// NeedsEVMArchive
// ---------------------------------------------------------------------------

func newEVMTracker(head int64, window int64) *tracker.RecentBlocks {
	rb := tracker.NewRecentBlocks(window)
	for l := head - window + 1; l <= head; l++ {
		rb.Add(l, fmt.Sprintf("0xhash%d", l))
	}
	return rb
}

func TestNeedsEVMArchive_NamedTags(t *testing.T) {
	rb := newEVMTracker(1000, 500)

	// Named tags that don't need archive
	for _, tag := range []string{"latest", "pending", "safe", "finalized"} {
		params := fmt.Sprintf(`["%s"]`, tag)
		if NeedsEVMArchive("eth_getBlockByNumber", params, rb) {
			t.Errorf("tag %q should not need archive", tag)
		}
	}

	// "earliest" always needs archive
	params := `["earliest"]`
	if !NeedsEVMArchive("eth_getBlockByNumber", params, rb) {
		t.Error(`"earliest" should need archive`)
	}
}

func TestNeedsEVMArchive_EmptyTag(t *testing.T) {
	rb := newEVMTracker(1000, 500)
	// Missing/empty tag defaults to not needing archive
	if NeedsEVMArchive("eth_getBlockByNumber", `[""]`, rb) {
		t.Error("empty tag should not need archive")
	}
	if NeedsEVMArchive("eth_getBlockByNumber", `[]`, rb) {
		t.Error("missing param should not need archive")
	}
}

func TestNeedsEVMArchive_RecentHexBlock(t *testing.T) {
	rb := newEVMTracker(1000, 500)

	// 0x3e8 = 1000 (head), recent
	if NeedsEVMArchive("eth_getBlockByNumber", `["0x3e8"]`, rb) {
		t.Error("0x3e8 (1000) should not need archive")
	}
	// 0x1f5 = 501, at boundary of window
	if NeedsEVMArchive("eth_getBlockByNumber", `["0x1f5"]`, rb) {
		t.Error("0x1f5 (501) should not need archive")
	}
}

func TestNeedsEVMArchive_OldHexBlock(t *testing.T) {
	rb := newEVMTracker(1000, 500)

	// 0x1f3 = 499, below head - window = 500
	if !NeedsEVMArchive("eth_getBlockByNumber", `["0x1f3"]`, rb) {
		t.Error("0x1f3 (499) should need archive")
	}
	// 0x1 = 1
	if !NeedsEVMArchive("eth_getBlockByNumber", `["0x1"]`, rb) {
		t.Error("0x1 should need archive")
	}
}

func TestNeedsEVMArchive_ParameterIndex(t *testing.T) {
	rb := newEVMTracker(1000, 500)

	// eth_getBalance uses param index 1
	params := `["0xaddr", "earliest"]`
	if !NeedsEVMArchive("eth_getBalance", params, rb) {
		t.Error("eth_getBalance with earliest should need archive")
	}

	params = `["0xaddr", "latest"]`
	if NeedsEVMArchive("eth_getBalance", params, rb) {
		t.Error("eth_getBalance with latest should not need archive")
	}

	// eth_getStorageAt uses param index 2
	params = `["0xaddr", "0x0", "0x1"]`
	if !NeedsEVMArchive("eth_getStorageAt", params, rb) {
		t.Error("eth_getStorageAt with 0x1 should need archive")
	}
}

func TestNeedsEVMArchive_UnknownMethod(t *testing.T) {
	rb := newEVMTracker(1000, 500)

	// Methods not in evmBlockTagIndex never need archive
	if NeedsEVMArchive("eth_chainId", `[]`, rb) {
		t.Error("eth_chainId should not need archive")
	}
	if NeedsEVMArchive("net_version", `[]`, rb) {
		t.Error("net_version should not need archive")
	}
}

func TestNeedsEVMArchive_GetLogs(t *testing.T) {
	rb := newEVMTracker(1000, 500)

	// fromBlock is old
	if !NeedsEVMArchive("eth_getLogs", `[{"fromBlock":"0x1","toBlock":"latest"}]`, rb) {
		t.Error("eth_getLogs with old fromBlock should need archive")
	}
	// toBlock is old
	if !NeedsEVMArchive("eth_getLogs", `[{"fromBlock":"latest","toBlock":"0x1"}]`, rb) {
		t.Error("eth_getLogs with old toBlock should need archive")
	}
	// Both recent
	if NeedsEVMArchive("eth_getLogs", `[{"fromBlock":"latest","toBlock":"latest"}]`, rb) {
		t.Error("eth_getLogs with latest/latest should not need archive")
	}
	// fromBlock earliest
	if !NeedsEVMArchive("eth_getLogs", `[{"fromBlock":"earliest"}]`, rb) {
		t.Error("eth_getLogs with earliest fromBlock should need archive")
	}
	// No filter object
	if NeedsEVMArchive("eth_getLogs", `[]`, rb) {
		t.Error("eth_getLogs with empty params should not need archive")
	}
	// Empty params string
	if NeedsEVMArchive("eth_getLogs", "", rb) {
		t.Error("eth_getLogs with empty string should not need archive")
	}
}

// ---------------------------------------------------------------------------
// Tier.String
// ---------------------------------------------------------------------------

func TestTierString(t *testing.T) {
	cases := []struct {
		tier Tier
		str  string
	}{
		{TierDefault, "default"},
		{TierExpensive, "expensive"},
		{TierInjection, "injection"},
		{TierScript, "script"},
		{TierStreaming, "streaming"},
		{TierDebug, "debug"},
		{NumTiers, "unknown"},
		{Tier(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.tier.String(); got != tc.str {
			t.Errorf("Tier(%d).String() = %q, want %q", tc.tier, got, tc.str)
		}
	}
}
