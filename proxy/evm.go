package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tezos-commons/rpc-proxy/balancer"
	"github.com/tezos-commons/rpc-proxy/cache"
	"github.com/tezos-commons/rpc-proxy/filter"
	"github.com/tezos-commons/rpc-proxy/log"
	"github.com/tezos-commons/rpc-proxy/metrics"
	"github.com/tezos-commons/rpc-proxy/ratelimit"
	"github.com/tezos-commons/rpc-proxy/tracker"
	"github.com/valyala/fasthttp"
)

var evmClient = &fasthttp.Client{
	MaxConnsPerHost:     8192,
	MaxIdleConnDuration: 30 * time.Second,
	ReadTimeout:         30 * time.Second,
	WriteTimeout:        10 * time.Second,
	MaxResponseBodySize: 50 * 1024 * 1024, // 50MB — prevents OOM from huge upstream responses
}

var (
	errNoNodeJSON    = []byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"no healthy node available"},"id":null}`)
	errUpstreamJSON  = []byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"upstream error"},"id":null}`)
	errBadBatchJSON  = []byte(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"invalid batch JSON"},"id":null}`)
	errBatchTooLarge = []byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"batch too large (max 100)"},"id":null}`)
	errForbiddenJSON = []byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"method not allowed"},"id":null}`)
	errRateLimitJSON = []byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"rate limit exceeded"},"id":null}`)
	contentJSON      = []byte("application/json")
	emptyBatch       = []byte("[]")
	nullID           = "null"
)

var respBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 4096)
		return &b
	},
}

var batchSemPool = sync.Pool{
	New: func() any {
		return make(chan struct{}, maxBatchConcurrency)
	},
}

// maxBatchConcurrency limits goroutines per batch to avoid scheduler overhead
// and to prevent a single batch from monopolizing the upstream semaphore.
const maxBatchConcurrency = 16

// maxBatchSize is the maximum number of items allowed in a JSON-RPC batch request.
const maxBatchSize = 100

// evmGzipMinSize is the minimum response size for gzip when the id field needs
// patching. Higher than cache.gzipMinSize because the rebuilt response can't
// use the pre-compressed cache entry.
const evmGzipMinSize = 1024

func handleEVM(ctx *fasthttp.RequestCtx, bal *balancer.Balancer, nc *cache.NetworkCache, rb *tracker.RecentBlocks, network string, fallbacks []string, m *metrics.Metrics, limiter *ratelimit.IPRateLimiter, logger *log.Logger) {
	body := ctx.PostBody()

	if len(body) > 0 && body[0] == '[' {
		handleEVMBatch(ctx, bal, nc, rb, network, body, fallbacks, m, limiter, logger)
		return
	}

	handleEVMSingle(ctx, bal, nc, rb, network, body, fallbacks, m, limiter, logger)
}

func handleEVMSingle(ctx *fasthttp.RequestCtx, bal *balancer.Balancer, nc *cache.NetworkCache, rb *tracker.RecentBlocks, network string, body []byte, fallbacks []string, m *metrics.Metrics, limiter *ratelimit.IPRateLimiter, logger *log.Logger) {
	m.RecordRequest()

	// Single parse: extract method, params, id
	info, parsed := cache.ParseEVMRequest(body)

	// Unparsed messages get the most restrictive tier to prevent
	// garbage bodies from bypassing method-based rate limiting.
	tier := filter.TierDebug
	if parsed {
		denied, methodTier := filter.CheckEVMMethod(info.Method)
		if denied {
			ctx.SetStatusCode(fasthttp.StatusForbidden)
			ctx.Response.Header.SetContentTypeBytes(contentJSON)
			ctx.SetBody(errForbiddenJSON)
			return
		}
		tier = methodTier
	}

	ip := clientIP(ctx)
	if !limiter.Allow(ip, tier) {
		ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(errRateLimitJSON)
		return
	}

	archiveOnly := parsed && filter.NeedsEVMArchive(info.Method, info.Params, rb)
	cacheable := parsed && tier != filter.TierInjection && tier != filter.TierStreaming

	if cacheable {
		cacheKey := cache.EVMCacheKey(network, &info)

		// Cache hit — serve from pre-parsed result/error, patch id
		if e, ok := nc.Get(cacheKey); ok {
			m.RecordCacheHit()
			serveEVMCached(ctx, e, info.ID)
			return
		}

		shouldCache := nc.ShouldCache(cacheKey)

		// Singleflight dedup
		v, _, shared := nc.Flights.Do(cacheKey, func() (any, error) {
			return doEVMForward(bal, body, archiveOnly, m, logger)
		})

		if e, _ := v.(*cache.Entry); e != nil {
			if !shared && shouldCache {
				nc.Set(cacheKey, e)
			}
			serveEVMCached(ctx, e, info.ID)
			return
		}

		// All nodes failed — try fallbacks
		if evmDirectFallback(ctx, fallbacks, body, logger) {
			return
		}
		m.RecordError()
		ctx.SetStatusCode(fasthttp.StatusBadGateway)
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(errUpstreamJSON)
		return
	}

	directEVMForward(ctx, bal, body, archiveOnly, fallbacks, m, logger)
}

// doEVMForward performs the upstream call and returns a cache.Entry with
// pre-parsed result/error (no id). Retries once on a different node.
// Caller is responsible for storing in cache after singleflight completes.
func doEVMForward(bal *balancer.Balancer, body []byte, archiveOnly bool, m *metrics.Metrics, logger *log.Logger) (*cache.Entry, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		node := bal.Pick(archiveOnly)
		if node == nil {
			m.RecordError()
			return nil, errNoHealthyNode
		}

		respBody, statusCode, err := forwardEVMRequest(node.URL, body)
		if err != nil {
			m.RecordError()
			logger.Warn(fmt.Sprintf("%s EVM upstream error: %s",
				log.Tag(node.Name), log.Err(err)))
			continue
		}

		// Parse response once — store result/error separately for id patching
		result, errField := cache.ParseEVMResponse(respBody)

		// Build canonical body with null id for cache storage + gzip
		canonicalBody := cache.BuildEVMResponse(result, errField, nullID)

		return &cache.Entry{
			Status:    statusCode,
			Headers:   []cache.HeaderPair{{Key: []byte("Content-Type"), Value: contentJSON}},
			Body:      canonicalBody,
			EVMResult: result,
			EVMError:  errField,
		}, nil
	}
	return nil, errNoHealthyNode
}

// serveEVMCached serves a cached EVM response, patching the caller's id.
// If callerID matches null (or is empty), serves pre-compressed gzip directly.
func serveEVMCached(ctx *fasthttp.RequestCtx, e *cache.Entry, callerID string) {
	ctx.SetStatusCode(e.Status)
	for _, h := range e.Headers {
		ctx.Response.Header.SetBytesKV(h.Key, h.Value)
	}

	ae := ctx.Request.Header.PeekBytes(headerAcceptEncoding)
	wantsGzip := cache.AcceptsGzip(ae)

	// Fast path: if caller id is null or empty, serve pre-compressed body directly
	isNullID := callerID == "" || callerID == "null"
	if isNullID {
		if wantsGzip && len(e.GzipBody) > 0 {
			ctx.Response.Header.SetBytesKV(headerContentEncoding, gzipValue)
			ctx.SetBody(e.GzipBody)
		} else {
			ctx.SetBody(e.Body)
		}
		return
	}

	// Common path: rebuild with caller's id (no JSON parsing — just concatenation)
	rebuilt := cache.BuildEVMResponse(e.EVMResult, e.EVMError, callerID)
	if wantsGzip && len(rebuilt) >= evmGzipMinSize {
		ctx.Response.Header.SetBytesKV(headerContentEncoding, gzipValue)
		ctx.SetBody(cache.Compress(rebuilt))
	} else {
		ctx.SetBody(rebuilt)
	}
}

func directEVMForward(ctx *fasthttp.RequestCtx, bal *balancer.Balancer, body []byte, archiveOnly bool, fallbacks []string, m *metrics.Metrics, logger *log.Logger) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		node := bal.Pick(archiveOnly)
		if node == nil {
			m.RecordError()
			ctx.SetStatusCode(fasthttp.StatusBadGateway)
			ctx.Response.Header.SetContentTypeBytes(contentJSON)
			ctx.SetBody(errNoNodeJSON)
			return
		}

		respBody, statusCode, err := forwardEVMRequest(node.URL, body)
		if err != nil {
			m.RecordError()
			logger.Warn(fmt.Sprintf("%s EVM upstream error: %s",
				log.Tag(node.Name), log.Err(err)))
			continue
		}

		ctx.SetStatusCode(statusCode)
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(respBody)
		return
	}

	// All nodes failed — try fallbacks
	if evmDirectFallback(ctx, fallbacks, body, logger) {
		return
	}
	ctx.SetStatusCode(fasthttp.StatusBadGateway)
	ctx.Response.Header.SetContentTypeBytes(contentJSON)
	ctx.SetBody(errUpstreamJSON)
}

func handleEVMBatch(ctx *fasthttp.RequestCtx, bal *balancer.Balancer, nc *cache.NetworkCache, rb *tracker.RecentBlocks, network string, body []byte, fallbacks []string, m *metrics.Metrics, limiter *ratelimit.IPRateLimiter, logger *log.Logger) {
	parsed := gjson.ParseBytes(body)
	if !parsed.IsArray() {
		m.RecordRequest()
		m.RecordError()
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(errBadBatchJSON)
		return
	}

	batch := parsed.Array()

	if len(batch) == 0 {
		m.RecordRequest()
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(emptyBatch)
		return
	}

	if len(batch) > maxBatchSize {
		m.RecordRequest()
		m.RecordError()
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(errBatchTooLarge)
		return
	}

	ip := clientIP(ctx)
	responses := make([][]byte, len(batch))
	var wg sync.WaitGroup

	// Semaphore to bound batch-internal concurrency (pooled to avoid allocation)
	batchSem := batchSemPool.Get().(chan struct{})

	for i, item := range batch {
		m.RecordRequest()
		wg.Add(1)

		batchSem <- struct{}{} // acquire
		go func(idx int, rawItem string) {
			defer func() {
				<-batchSem // release
				wg.Done()
			}()

			responses[idx] = processBatchItem(unsafeBytes(rawItem), ip, network, bal, nc, rb, m, limiter, logger)
		}(i, item.Raw)
	}

	wg.Wait()
	batchSemPool.Put(batchSem)

	// Assemble batch response
	bp := respBufPool.Get().(*[]byte)
	buf := (*bp)[:0]
	buf = append(buf, '[')
	for i, r := range responses {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, r...)
	}
	buf = append(buf, ']')

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.Header.SetContentTypeBytes(contentJSON)
	// SetBody copies buf internally — safe to return to pool after this call.
	// Do NOT replace with SetBodyRaw (zero-copy) without removing the pool return.
	ctx.SetBody(buf)

	*bp = buf
	respBufPool.Put(bp)
}

func processBatchItem(reqBody []byte, ip, network string, bal *balancer.Balancer, nc *cache.NetworkCache, rb *tracker.RecentBlocks, m *metrics.Metrics, limiter *ratelimit.IPRateLimiter, logger *log.Logger) []byte {
	// Single parse for method, params, id
	info, parsed := cache.ParseEVMRequest(reqBody)

	tier := filter.TierDebug
	if parsed {
		denied, methodTier := filter.CheckEVMMethod(info.Method)
		if denied {
			return errForbiddenJSON
		}
		tier = methodTier
	}
	if !limiter.Allow(ip, tier) {
		return errRateLimitJSON
	}

	archiveOnly := parsed && filter.NeedsEVMArchive(info.Method, info.Params, rb)
	cacheable := parsed && tier != filter.TierInjection && tier != filter.TierStreaming
	if cacheable {
		cacheKey := cache.EVMCacheKey(network, &info)

		if e, ok := nc.Get(cacheKey); ok {
			m.RecordCacheHit()
			return cache.BuildEVMResponse(e.EVMResult, e.EVMError, info.ID)
		}

		shouldCache := nc.ShouldCache(cacheKey)

		v, _, shared := nc.Flights.Do(cacheKey, func() (any, error) {
			return doEVMForward(bal, reqBody, archiveOnly, m, logger)
		})

		if e, _ := v.(*cache.Entry); e != nil {
			if !shared && shouldCache {
				nc.Set(cacheKey, e)
			}
			return cache.BuildEVMResponse(e.EVMResult, e.EVMError, info.ID)
		}

		m.RecordError()
		return errUpstreamJSON
	}

	// Direct forward with retry
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		node := bal.Pick(archiveOnly)
		if node == nil {
			m.RecordError()
			return errNoNodeJSON
		}

		respBody, _, err := forwardEVMRequest(node.URL, reqBody)
		if err != nil {
			m.RecordError()
			logger.Warn(fmt.Sprintf("%s EVM batch upstream error: %s",
				log.Tag(node.Name), log.Err(err)))
			continue
		}
		return respBody
	}
	return errUpstreamJSON
}

func forwardEVMRequest(nodeURL string, body []byte) ([]byte, int, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(nodeURL)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentTypeBytes(contentJSON)
	req.SetBody(body)

	if err := evmClient.DoTimeout(req, resp, 30*time.Second); err != nil {
		return nil, 0, err
	}

	src := resp.Body()
	respBody := make([]byte, len(src))
	copy(respBody, src)

	return respBody, resp.StatusCode(), nil
}

// evmDirectFallback tries each fallback URL in order for an EVM JSON-RPC request.
// Returns true if one succeeded.
func evmDirectFallback(ctx *fasthttp.RequestCtx, fallbacks []string, body []byte, logger *log.Logger) bool {
	if len(fallbacks) == 0 {
		return false
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	for _, fb := range fallbacks {
		resp.Reset()

		req.SetRequestURI(fb)
		req.Header.SetMethod(fasthttp.MethodPost)
		req.Header.SetContentTypeBytes(contentJSON)
		req.SetBody(body)

		if err := evmClient.DoTimeout(req, resp, 30*time.Second); err != nil {
			logger.Warn(fmt.Sprintf("%s EVM fallback error: %s",
				log.Tag("fallback"), log.Err(err)))
			continue
		}

		ctx.SetStatusCode(resp.StatusCode())
		ctx.Response.Header.SetContentTypeBytes(contentJSON)
		ctx.SetBody(resp.Body())
		logger.Info(fmt.Sprintf("%s Served from fallback %s", log.Tag("fallback"), fb))
		return true
	}
	return false
}
