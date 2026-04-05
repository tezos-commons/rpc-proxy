package proxy

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/tezos-commons/rpc-proxy/balancer"
	"github.com/tezos-commons/rpc-proxy/cache"
	"github.com/tezos-commons/rpc-proxy/log"
	"github.com/tezos-commons/rpc-proxy/metrics"
	"github.com/tezos-commons/rpc-proxy/tracker"
	"github.com/valyala/fasthttp"
)

var tezosClient = &fasthttp.Client{
	MaxConnsPerHost:     8192,
	MaxIdleConnDuration: 30 * time.Second,
	ReadTimeout:         60 * time.Second,
	WriteTimeout:        10 * time.Second,
	MaxResponseBodySize: 50 * 1024 * 1024, // 50MB — prevents OOM from huge upstream responses
}

var (
	headerConnection      = []byte("Connection")
	headerHost            = []byte("Host")
	headerContentEncoding = []byte("Content-Encoding")
	headerAcceptEncoding  = []byte("Accept-Encoding")
	gzipValue             = []byte("gzip")
)

var uriBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

func tezosCacheKey(network string, ctx *fasthttp.RequestCtx, upstreamPath []byte) string {
	return cache.HashKey(
		unsafeBytes(network),
		ctx.Method(),
		upstreamPath,
		ctx.QueryArgs().QueryString(),
		ctx.PostBody(),
	)
}

func serveCachedEntry(ctx *fasthttp.RequestCtx, e *cache.Entry) {
	ctx.SetStatusCode(e.Status)
	for _, h := range e.Headers {
		// Skip Content-Encoding from cached headers — we set it ourselves
		if bytes.Equal(h.Key, headerContentEncoding) {
			continue
		}
		ctx.Response.Header.SetBytesKV(h.Key, h.Value)
	}

	ae := ctx.Request.Header.PeekBytes(headerAcceptEncoding)
	if cache.AcceptsGzip(ae) && len(e.GzipBody) > 0 {
		ctx.Response.Header.SetBytesKV(headerContentEncoding, gzipValue)
		ctx.SetBody(e.GzipBody)
	} else {
		ctx.SetBody(e.Body)
	}
}

func handleTezos(ctx *fasthttp.RequestCtx, bal *balancer.Balancer, nc *cache.NetworkCache, network string, upstreamPath []byte, cacheable bool, archiveOnly bool, fallbacks []string, m *metrics.Metrics, logger *log.Logger) {
	m.RecordRequest()

	// Cache lookup
	var cacheKey string
	if cacheable {
		cacheKey = tezosCacheKey(network, ctx, upstreamPath)
		if e, ok := nc.Get(cacheKey); ok {
			m.RecordCacheHit()
			serveCachedEntry(ctx, e)
			return
		}
	}

	// Singleflight dedup for cacheable requests
	if cacheable {
		shouldCache := nc.ShouldCache(cacheKey)

		// Capture request data inside the singleflight func — only the leader
		// goroutine executes this, so reading from ctx is safe (it owns its ctx).
		// Waiters block and receive the result without allocating reqData.
		v, _, shared := nc.Flights.Do(cacheKey, func() (any, error) {
			reqData := captureTezosRequest(ctx, upstreamPath)
			return doTezosForward(bal, &reqData, archiveOnly, m, logger)
		})

		if e, _ := v.(*cache.Entry); e != nil {
			// Only the singleflight leader stores to cache (after waiters are unblocked)
			if !shared && shouldCache {
				nc.Set(cacheKey, e)
			}
			serveCachedEntry(ctx, e)
			return
		}

		// All nodes failed — try fallbacks before giving up
		if tezosDirectFallback(ctx, fallbacks, upstreamPath, logger) {
			return
		}

		m.RecordError()
		ctx.SetStatusCode(fasthttp.StatusBadGateway)
		ctx.SetBodyString("upstream error")
		return
	}

	// Non-cacheable (e.g. injection): forward directly from ctx, no copy needed
	directTezosForward(ctx, bal, upstreamPath, archiveOnly, fallbacks, m, logger)
}

// tezosRequestData holds the data extracted from a fasthttp.RequestCtx needed
// to forward a Tezos request. Captured before entering singleflight so the
// closure never touches a ctx it doesn't own.
type tezosRequestData struct {
	uri     []byte
	method  []byte
	headers []cache.HeaderPair
	body    []byte
}

func captureTezosRequest(ctx *fasthttp.RequestCtx, upstreamPath []byte) tezosRequestData {
	method := ctx.Method()
	methodCopy := make([]byte, len(method))
	copy(methodCopy, method)

	body := ctx.PostBody()
	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)

	qs := ctx.QueryArgs().QueryString()
	uriSize := len(upstreamPath) + 1 + len(qs) // +1 for '?'
	uri := make([]byte, 0, uriSize)
	uri = append(uri, upstreamPath...)
	if len(qs) > 0 {
		uri = append(uri, '?')
		uri = append(uri, qs...)
	}

	// Two-pass header copy: count total bytes, single allocation, then copy.
	var totalHeaderBytes int
	var headerCount int
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || bytes.Equal(key, headerHost) {
			return
		}
		totalHeaderBytes += len(key) + len(value)
		headerCount++
	})
	headerBuf := make([]byte, totalHeaderBytes)
	headers := make([]cache.HeaderPair, 0, headerCount)
	off := 0
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || bytes.Equal(key, headerHost) {
			return
		}
		k := headerBuf[off : off+len(key)]
		copy(k, key)
		off += len(key)
		v := headerBuf[off : off+len(value)]
		copy(v, value)
		off += len(value)
		headers = append(headers, cache.HeaderPair{Key: k, Value: v})
	})

	return tezosRequestData{
		uri:     uri,
		method:  methodCopy,
		headers: headers,
		body:    bodyCopy,
	}
}

// doTezosForward performs the actual upstream request and returns a cache entry.
// Retries once on a different node on failure.
// Does not touch any fasthttp.RequestCtx — safe to call from singleflight.
func doTezosForward(bal *balancer.Balancer, reqData *tezosRequestData, archiveOnly bool, m *metrics.Metrics, logger *log.Logger) (*cache.Entry, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		node := bal.Pick(archiveOnly)
		if node == nil {
			m.RecordError()
			return nil, errNoHealthyNode
		}

		entry, err := doTezosRequest(node, reqData)
		if err != nil {
			m.RecordError()
			logger.Warn(fmt.Sprintf("%s Tezos upstream error: %s",
				log.Tag(node.Name), log.Err(err)))
			continue
		}
		return entry, nil
	}
	return nil, errNoHealthyNode
}

func doTezosRequest(node *tracker.NodeStatus, reqData *tezosRequestData) (*cache.Entry, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build upstream URI: nodeURL + pre-built path?query (pooled buffer).
	// Safe to return to pool immediately: SetRequestURIBytes copies internally.
	bp := uriBufPool.Get().(*[]byte)
	uri := (*bp)[:0]
	uri = append(uri, node.URL...)
	uri = append(uri, reqData.uri...)
	req.SetRequestURIBytes(uri)
	*bp = uri
	uriBufPool.Put(bp)

	req.Header.SetMethodBytes(reqData.method)

	for _, h := range reqData.headers {
		req.Header.SetBytesKV(h.Key, h.Value)
	}

	req.SetBody(reqData.body)

	if err := tezosClient.DoTimeout(req, resp, 60*time.Second); err != nil {
		return nil, err
	}

	// Build cache entry from response — two-pass header copy into single buffer
	// since fasthttp reuses buffers after ReleaseResponse.
	var respHeaderBytes int
	var respHeaderCount int
	resp.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || IsAccessControl(key) {
			return
		}
		respHeaderBytes += len(key) + len(value)
		respHeaderCount++
	})
	respHeaderBuf := make([]byte, respHeaderBytes)
	headers := make([]cache.HeaderPair, 0, respHeaderCount)
	off := 0
	resp.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || IsAccessControl(key) {
			return
		}
		k := respHeaderBuf[off : off+len(key)]
		copy(k, key)
		off += len(key)
		v := respHeaderBuf[off : off+len(value)]
		copy(v, value)
		off += len(value)
		headers = append(headers, cache.HeaderPair{Key: k, Value: v})
	})

	body := resp.Body()
	rawBody := make([]byte, len(body))
	copy(rawBody, body)

	return &cache.Entry{
		Status:  resp.StatusCode(),
		Headers: headers,
		Body:    rawBody,
	}, nil
}

// directTezosForward forwards a request directly from ctx to upstream without
// copying request data. Used for non-cacheable requests that don't enter
// singleflight (e.g. injection), avoiding the allocation overhead of
// captureTezosRequest. Retries once on a different node on failure.
func directTezosForward(ctx *fasthttp.RequestCtx, bal *balancer.Balancer, upstreamPath []byte, archiveOnly bool, fallbacks []string, m *metrics.Metrics, logger *log.Logger) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		node := bal.Pick(archiveOnly)
		if node == nil {
			break
		}

		err := doDirectTezosRequest(ctx, node, upstreamPath)
		if err != nil {
			m.RecordError()
			logger.Warn(fmt.Sprintf("%s Tezos upstream error: %s",
				log.Tag(node.Name), log.Err(err)))
			continue
		}
		return
	}

	// All nodes failed — try fallbacks
	if tezosDirectFallback(ctx, fallbacks, upstreamPath, logger) {
		return
	}

	ctx.SetStatusCode(fasthttp.StatusBadGateway)
	ctx.SetBodyString("upstream error")
}

func doDirectTezosRequest(ctx *fasthttp.RequestCtx, node *tracker.NodeStatus, upstreamPath []byte) error {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// Build upstream URI using pooled buffer
	bp := uriBufPool.Get().(*[]byte)
	buf := (*bp)[:0]
	buf = append(buf, node.URL...)
	buf = append(buf, upstreamPath...)
	qs := ctx.QueryArgs().QueryString()
	if len(qs) > 0 {
		buf = append(buf, '?')
		buf = append(buf, qs...)
	}
	req.SetRequestURIBytes(buf)
	*bp = buf
	uriBufPool.Put(bp)

	req.Header.SetMethodBytes(ctx.Method())

	// Copy request headers directly from ctx — safe because ctx is owned
	// by this goroutine (no singleflight sharing).
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || bytes.Equal(key, headerHost) {
			return
		}
		req.Header.SetBytesKV(key, value)
	})

	// Ensure we request gzip from upstream
	if len(req.Header.PeekBytes(headerAcceptEncoding)) == 0 {
		req.Header.SetBytesKV(headerAcceptEncoding, gzipValue)
	}

	req.SetBody(ctx.PostBody())

	if err := tezosClient.DoTimeout(req, resp, 60*time.Second); err != nil {
		return err
	}

	// Write response directly to ctx — fasthttp copies internally via SetBody,
	// so it's safe even though resp is released after this function returns.
	ctx.SetStatusCode(resp.StatusCode())
	resp.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || IsAccessControl(key) {
			return
		}
		ctx.Response.Header.SetBytesKV(key, value)
	})
	ctx.SetBody(resp.Body())
	return nil
}

// tezosDirectFallback tries each fallback URL in order. Returns true if one succeeded.
func tezosDirectFallback(ctx *fasthttp.RequestCtx, fallbacks []string, upstreamPath []byte, logger *log.Logger) bool {
	if len(fallbacks) == 0 {
		return false
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	for _, fb := range fallbacks {
		resp.Reset()

		bp := uriBufPool.Get().(*[]byte)
		buf := (*bp)[:0]
		buf = append(buf, fb...)
		buf = append(buf, upstreamPath...)
		qs := ctx.QueryArgs().QueryString()
		if len(qs) > 0 {
			buf = append(buf, '?')
			buf = append(buf, qs...)
		}
		req.SetRequestURIBytes(buf)
		*bp = buf
		uriBufPool.Put(bp)

		req.Header.SetMethodBytes(ctx.Method())
		ctx.Request.Header.VisitAll(func(key, value []byte) {
			if bytes.Equal(key, headerConnection) || bytes.Equal(key, headerHost) {
				return
			}
			req.Header.SetBytesKV(key, value)
		})
		req.SetBody(ctx.PostBody())

		if err := tezosClient.DoTimeout(req, resp, 30*time.Second); err != nil {
			logger.Warn(fmt.Sprintf("%s Tezos fallback error: %s",
				log.Tag("fallback"), log.Err(err)))
			continue
		}

		ctx.SetStatusCode(resp.StatusCode())
		resp.Header.VisitAll(func(key, value []byte) {
			if bytes.Equal(key, headerConnection) || IsAccessControl(key) {
				return
			}
			ctx.Response.Header.SetBytesKV(key, value)
		})
		ctx.SetBody(resp.Body())
		logger.Info(fmt.Sprintf("%s Served from fallback %s", log.Tag("fallback"), fb))
		return true
	}
	return false
}

type errType struct{}

func (errType) Error() string { return "no healthy node" }

var errNoHealthyNode = errType{}
