package cache

import (
	"bytes"
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/tidwall/gjson"
)

// EVMRequestInfo holds pre-parsed fields from a JSON-RPC request.
// Extracted once via gjson (zero-alloc scan), reused for filter check,
// cache key, and id patching.
type EVMRequestInfo struct {
	Method string
	Params string // raw JSON string (gjson result, references original body)
	ID     string // raw JSON string
}

// ParseEVMRequest extracts method, params, and id from a JSON-RPC request body
// using gjson. Zero allocations for the scan — returned strings reference the
// original body bytes.
func ParseEVMRequest(body []byte) (info EVMRequestInfo, ok bool) {
	results := gjson.GetManyBytes(body, "method", "params", "id")
	method := results[0].Str
	if method == "" {
		return EVMRequestInfo{}, false
	}
	return EVMRequestInfo{
		Method: method,
		Params: results[1].Raw,
		ID:     results[2].Raw,
	}, true
}

var compactBufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// EVMCacheKey builds a fixed-size cache key from pre-parsed request info.
// Params are JSON-compacted to canonicalize whitespace so that semantically
// identical requests from different clients produce the same key.
func EVMCacheKey(network string, info *EVMRequestInfo) string {
	params := unsafeBytes(info.Params)

	// Fast path: if params have no whitespace, skip compaction
	if !bytes.ContainsAny(params, " \t\n\r") {
		return HashKey(unsafeBytes(network), unsafeBytes(info.Method), params)
	}

	buf := compactBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := json.Compact(buf, params); err == nil {
		params = buf.Bytes()
	}
	key := HashKey(unsafeBytes(network), unsafeBytes(info.Method), params)
	compactBufPool.Put(buf)
	return key
}

func unsafeBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// ParseEVMResponse extracts result and error raw JSON from a response body
// using gjson. Returns owned copies so the original body can be GC'd.
func ParseEVMResponse(body []byte) (result, errField json.RawMessage) {
	results := gjson.GetManyBytes(body, "result", "error")
	if r := results[0].Raw; r != "" {
		result = make(json.RawMessage, len(r))
		copy(result, r)
	}
	if e := results[1].Raw; e != "" {
		errField = make(json.RawMessage, len(e))
		copy(errField, e)
	}
	return result, errField
}

var evmBuildBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// BuildEVMResponse constructs a JSON-RPC response from pre-parsed result/error
// and the given id. Zero re-parsing — pure byte concatenation.
func BuildEVMResponse(result, errField json.RawMessage, id string) []byte {
	bp := evmBuildBufPool.Get().(*[]byte)
	buf := (*bp)[:0]

	buf = append(buf, `{"jsonrpc":"2.0",`...)
	if len(result) > 0 {
		buf = append(buf, `"result":`...)
		buf = append(buf, result...)
	} else if len(errField) > 0 {
		buf = append(buf, `"error":`...)
		buf = append(buf, errField...)
	} else {
		buf = append(buf, `"result":null`...)
	}
	buf = append(buf, `,"id":`...)
	if len(id) > 0 {
		buf = append(buf, id...)
	} else {
		buf = append(buf, "null"...)
	}
	buf = append(buf, '}')

	// Copy out before returning to pool
	out := make([]byte, len(buf))
	copy(out, buf)

	*bp = buf
	evmBuildBufPool.Put(bp)
	return out
}
