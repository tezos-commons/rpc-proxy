package cache

import (
	"encoding/json"
	"testing"
)

func TestParseEVMRequest_Valid(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`)
	info, ok := ParseEVMRequest(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if info.Method != "eth_blockNumber" {
		t.Fatalf("Method = %q; want %q", info.Method, "eth_blockNumber")
	}
	if info.Params != "[]" {
		t.Fatalf("Params = %q; want %q", info.Params, "[]")
	}
	if info.ID != "1" {
		t.Fatalf("ID = %q; want %q", info.ID, "1")
	}
}

func TestParseEVMRequest_StringID(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"eth_call","params":[{},"latest"],"id":"abc-123"}`)
	info, ok := ParseEVMRequest(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if info.ID != `"abc-123"` {
		t.Fatalf("ID = %q; want %q", info.ID, `"abc-123"`)
	}
}

func TestParseEVMRequest_NullID(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":null}`)
	info, ok := ParseEVMRequest(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if info.ID != "null" {
		t.Fatalf("ID = %q; want %q", info.ID, "null")
	}
}

func TestParseEVMRequest_MissingMethod(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","params":[],"id":1}`)
	_, ok := ParseEVMRequest(body)
	if ok {
		t.Fatal("expected ok=false when method is missing")
	}
}

func TestParseEVMRequest_NoParams(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	info, ok := ParseEVMRequest(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if info.Params != "" {
		t.Fatalf("Params = %q; want empty", info.Params)
	}
}

func TestParseEVMRequest_NoID(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","params":[]}`)
	info, ok := ParseEVMRequest(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if info.ID != "" {
		t.Fatalf("ID = %q; want empty", info.ID)
	}
}

func TestParseEVMResponse_ResultOnly(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","result":"0x10","id":1}`)
	result, errField := ParseEVMResponse(body)
	if string(result) != `"0x10"` {
		t.Fatalf("result = %q; want %q", result, `"0x10"`)
	}
	if errField != nil {
		t.Fatalf("errField should be nil, got %q", errField)
	}
}

func TestParseEVMResponse_ErrorOnly(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","error":{"code":-32601,"message":"method not found"},"id":1}`)
	result, errField := ParseEVMResponse(body)
	if result != nil {
		t.Fatalf("result should be nil, got %q", result)
	}

	var errObj map[string]interface{}
	if err := json.Unmarshal(errField, &errObj); err != nil {
		t.Fatalf("failed to unmarshal error: %v", err)
	}
	if errObj["code"].(float64) != -32601 {
		t.Fatalf("error code = %v; want -32601", errObj["code"])
	}
}

func TestParseEVMResponse_Neither(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1}`)
	result, errField := ParseEVMResponse(body)
	if result != nil {
		t.Fatalf("expected nil result, got %q", result)
	}
	if errField != nil {
		t.Fatalf("expected nil errField, got %q", errField)
	}
}

func TestEVMCacheKey_Consistent(t *testing.T) {
	info := &EVMRequestInfo{Method: "eth_blockNumber", Params: "[]"}
	k1 := EVMCacheKey("mainnet", info)
	k2 := EVMCacheKey("mainnet", info)
	if k1 != k2 {
		t.Fatal("EVMCacheKey not consistent for same input")
	}
}

func TestEVMCacheKey_DifferentNetworks(t *testing.T) {
	info := &EVMRequestInfo{Method: "eth_blockNumber", Params: "[]"}
	k1 := EVMCacheKey("mainnet", info)
	k2 := EVMCacheKey("goerli", info)
	if k1 == k2 {
		t.Fatal("different networks should produce different cache keys")
	}
}

func TestEVMCacheKey_DifferentMethods(t *testing.T) {
	i1 := &EVMRequestInfo{Method: "eth_blockNumber", Params: "[]"}
	i2 := &EVMRequestInfo{Method: "eth_chainId", Params: "[]"}
	if EVMCacheKey("mainnet", i1) == EVMCacheKey("mainnet", i2) {
		t.Fatal("different methods should produce different cache keys")
	}
}

func TestEVMCacheKey_IgnoresID(t *testing.T) {
	i1 := &EVMRequestInfo{Method: "eth_blockNumber", Params: "[]", ID: "1"}
	i2 := &EVMRequestInfo{Method: "eth_blockNumber", Params: "[]", ID: "999"}
	if EVMCacheKey("mainnet", i1) != EVMCacheKey("mainnet", i2) {
		t.Fatal("cache key should not depend on request ID")
	}
}

func TestBuildEVMResponse_WithResult(t *testing.T) {
	result := json.RawMessage(`"0x10"`)
	out := BuildEVMResponse(result, nil, "1")

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if resp["jsonrpc"] != "2.0" {
		t.Fatalf("jsonrpc = %v; want 2.0", resp["jsonrpc"])
	}
	if resp["result"] != "0x10" {
		t.Fatalf("result = %v; want 0x10", resp["result"])
	}
	if resp["id"].(float64) != 1 {
		t.Fatalf("id = %v; want 1", resp["id"])
	}
}

func TestBuildEVMResponse_WithError(t *testing.T) {
	errField := json.RawMessage(`{"code":-32601,"message":"not found"}`)
	out := BuildEVMResponse(nil, errField, "42")

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"].(float64) != -32601 {
		t.Fatalf("error code = %v; want -32601", errObj["code"])
	}
	if resp["id"].(float64) != 42 {
		t.Fatalf("id = %v; want 42", resp["id"])
	}
}

func TestBuildEVMResponse_StringID(t *testing.T) {
	result := json.RawMessage(`true`)
	out := BuildEVMResponse(result, nil, `"req-abc"`)

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if resp["id"] != "req-abc" {
		t.Fatalf("id = %v; want req-abc", resp["id"])
	}
}

func TestBuildEVMResponse_NullID(t *testing.T) {
	result := json.RawMessage(`"0x1"`)
	out := BuildEVMResponse(result, nil, "null")

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if resp["id"] != nil {
		t.Fatalf("id = %v; want null", resp["id"])
	}
}

func TestBuildEVMResponse_EmptyID(t *testing.T) {
	result := json.RawMessage(`"0x1"`)
	out := BuildEVMResponse(result, nil, "")

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	// Empty id should produce null
	if resp["id"] != nil {
		t.Fatalf("id = %v; want null for empty id", resp["id"])
	}
}

func TestBuildEVMResponse_ObjectResult(t *testing.T) {
	result := json.RawMessage(`{"number":"0x10","hash":"0xabc"}`)
	out := BuildEVMResponse(result, nil, "5")

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	resObj := resp["result"].(map[string]interface{})
	if resObj["number"] != "0x10" {
		t.Fatalf("result.number = %v; want 0x10", resObj["number"])
	}
}

func TestBuildEVMResponse_NeitherResultNorError(t *testing.T) {
	out := BuildEVMResponse(nil, nil, "1")

	var resp map[string]interface{}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if resp["jsonrpc"] != "2.0" {
		t.Fatalf("jsonrpc = %v; want 2.0", resp["jsonrpc"])
	}
	if _, ok := resp["result"]; !ok {
		t.Fatal("expected 'result' key in response")
	}
	if resp["result"] != nil {
		t.Fatalf("result = %v; want null", resp["result"])
	}
	if resp["id"].(float64) != 1 {
		t.Fatalf("id = %v; want 1", resp["id"])
	}
}

func TestBuildEVMResponse_PoolReuse(t *testing.T) {
	// Call multiple times to exercise sync.Pool reuse
	for i := 0; i < 100; i++ {
		result := json.RawMessage(`"0x1"`)
		out := BuildEVMResponse(result, nil, "1")
		var resp map[string]interface{}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("iteration %d: invalid JSON: %v", i, err)
		}
	}
}
