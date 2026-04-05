package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/tezos-commons/rpc-proxy/balancer"
	"github.com/tezos-commons/rpc-proxy/log"
	"github.com/tezos-commons/rpc-proxy/metrics"
	"github.com/valyala/fasthttp"
)

var streamBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

var streamingHTTPClient = &http.Client{
	Timeout: 0, // no timeout — streaming is long-lived
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     30 * time.Second,
	},
}

// handleTezosStream pipes a streaming Tezos endpoint (e.g. /monitor/heads/main)
// from an upstream node to the client without buffering the entire response.
func handleTezosStream(fctx *fasthttp.RequestCtx, bal *balancer.Balancer, upstreamPath []byte, m *metrics.Metrics, logger *log.Logger) {
	m.RecordRequest()

	node := bal.Pick(false)
	if node == nil {
		m.RecordError()
		fctx.SetStatusCode(fasthttp.StatusBadGateway)
		fctx.SetBodyString("no healthy tezos node available")
		return
	}

	// Build upstream URL
	url := node.URL + string(upstreamPath)
	qs := fctx.QueryArgs().QueryString()
	if len(qs) > 0 {
		url += "?" + string(qs)
	}

	httpReq, err := http.NewRequest(string(fctx.Method()), url, bytes.NewReader(fctx.PostBody()))
	if err != nil {
		m.RecordError()
		fctx.SetStatusCode(fasthttp.StatusBadGateway)
		fctx.SetBodyString("failed to create upstream request")
		return
	}

	// Copy request headers
	fctx.Request.Header.VisitAll(func(key, value []byte) {
		if bytes.Equal(key, headerConnection) || bytes.Equal(key, headerHost) {
			return
		}
		httpReq.Header.Set(string(key), string(value))
	})

	resp, err := streamingHTTPClient.Do(httpReq)
	if err != nil {
		m.RecordError()
		logger.Warn(fmt.Sprintf("%s Tezos streaming error: %s",
			log.Tag(node.Name), log.Err(err)))
		fctx.SetStatusCode(fasthttp.StatusBadGateway)
		fctx.SetBodyString("upstream error")
		return
	}

	// Copy response headers
	fctx.SetStatusCode(resp.StatusCode)
	for k, vals := range resp.Header {
		if k == "Connection" || len(k) >= 15 && k[:15] == "Access-Control-" {
			continue
		}
		for _, v := range vals {
			fctx.Response.Header.Set(k, v)
		}
	}

	// Stream response body — pipes upstream to client without buffering.
	// When the client disconnects, Write/Flush errors cause us to return,
	// which triggers resp.Body.Close() and cleans up the upstream connection.
	fctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		defer resp.Body.Close()
		bp := streamBufPool.Get().(*[]byte)
		buf := *bp
		defer streamBufPool.Put(bp)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				if werr := w.Flush(); werr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	})
}
