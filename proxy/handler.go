package proxy

import (
	"bytes"
	"net"
	"sync/atomic"
	"unsafe"

	"github.com/tezos-commons/rpc-proxy/balancer"
	"github.com/tezos-commons/rpc-proxy/cache"
	"github.com/tezos-commons/rpc-proxy/filter"
	"github.com/tezos-commons/rpc-proxy/log"
	"github.com/tezos-commons/rpc-proxy/metrics"
	"github.com/tezos-commons/rpc-proxy/ratelimit"
	"github.com/tezos-commons/rpc-proxy/tracker"
	"github.com/valyala/fasthttp"
)

var (
	prefixTezos         = []byte("/tezos/")
	prefixEtherlink     = []byte("/etherlink/")
	slashByte           = byte('/')
	headerAccessControl = []byte("Access-Control-")
)

// IsAccessControl returns true if the header key starts with "Access-Control-".
func IsAccessControl(key []byte) bool {
	return len(key) >= len(headerAccessControl) &&
		bytes.EqualFold(key[:len(headerAccessControl)], headerAccessControl)
}

// NetworkSet holds a balancer, cache, and recent blocks for a single network.
type NetworkSet struct {
	Balancer     *balancer.Balancer
	Cache        *cache.NetworkCache
	RecentBlocks *tracker.RecentBlocks
	Fallbacks    []string // external fallback URLs, tried before returning 502
}

// maxStreamsPerIP caps concurrent streaming connections from a single IP
// to prevent a single client from exhausting the global streaming pool.
const maxStreamsPerIP = 10

// Handler is the main fasthttp request handler that routes to chain-specific proxies.
type Handler struct {
	tezosNetworks     map[string]*NetworkSet
	etherlinkNetworks map[string]*NetworkSet
	metrics           *metrics.Metrics
	limiter           *ratelimit.IPRateLimiter
	logger            *log.Logger
	maxStreams        int64           // max concurrent streaming connections
	activeStreams     atomic.Int64    // current streaming connection count
	ipStreams         *cache.ShardMap[*atomic.Int64] // per-IP stream counts
}

func NewHandler(
	tezosNetworks map[string]*NetworkSet,
	etherlinkNetworks map[string]*NetworkSet,
	m *metrics.Metrics,
	limiter *ratelimit.IPRateLimiter,
	maxStreams int64,
	logger *log.Logger,
) *Handler {
	return &Handler{
		tezosNetworks:     tezosNetworks,
		etherlinkNetworks: etherlinkNetworks,
		metrics:           m,
		limiter:           limiter,
		logger:            logger,
		maxStreams:        maxStreams,
		ipStreams:         cache.NewShardMap[*atomic.Int64](),
	}
}

// SetMaxStreams updates the maximum concurrent streaming connections.
func (h *Handler) SetMaxStreams(n int64) {
	h.maxStreams = n
}

func (h *Handler) HandleRequest(ctx *fasthttp.RequestCtx) {
	// /ready — health check, no CORS or other headers needed
	if string(ctx.Path()) == "/ready" {
		h.handleReady(ctx)
		return
	}

	ctx.Response.Header.Set("X-Proxy", "tc-rpc-proxy")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	ctx.Response.Header.Set("Access-Control-Allow-Methods", "*")
	ctx.Response.Header.Set("Access-Control-Allow-Headers", "*")
	ctx.Response.Header.Set("Access-Control-Max-Age", "3600")

	if string(ctx.Method()) == "OPTIONS" {
		ctx.SetStatusCode(fasthttp.StatusNoContent)
		return
	}

	path := ctx.Path()

	// /tezos/<network>/...
	if bytes.HasPrefix(path, prefixTezos) {
		rest := path[len(prefixTezos):]
		slashIdx := bytes.IndexByte(rest, slashByte)

		var network string
		var upstreamPath []byte
		if slashIdx < 0 {
			network = string(rest)
			upstreamPath = []byte("/")
		} else {
			network = string(rest[:slashIdx])
			upstreamPath = rest[slashIdx:]
		}

		ns, ok := h.tezosNetworks[network]
		if !ok {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetBodyString("unknown tezos network")
			return
		}

		// GET on the base path → redirect to domain root
		if string(ctx.Method()) == "GET" && string(upstreamPath) == "/" {
			redirectToDomain(ctx)
			return
		}

		// Route filter
		httpMethod := unsafeString(ctx.Method())
		upstreamPathStr := unsafeString(upstreamPath)
		denied, tier := filter.CheckTezos(httpMethod, upstreamPathStr)
		if denied {
			ctx.SetStatusCode(fasthttp.StatusForbidden)
			ctx.SetBodyString("forbidden")
			return
		}

		// Per-IP rate limit
		ip := clientIP(ctx)
		if !h.limiter.Allow(ip, tier) {
			ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
			ctx.SetBodyString("rate limit exceeded")
			return
		}

		// Streaming endpoints are piped directly without buffering
		if tier == filter.TierStreaming {
			if h.activeStreams.Load() >= h.maxStreams {
				ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
				ctx.SetBodyString("too many streaming connections")
				return
			}

			// Per-IP stream limit
			ipCtr := h.getIPStreamCounter(ip)
			if ipCtr.Load() >= int64(maxStreamsPerIP) {
				ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
				ctx.SetBodyString("too many streaming connections from this IP")
				return
			}
			ipCtr.Add(1)
			h.activeStreams.Add(1)
			defer func() {
				ipCtr.Add(-1)
				h.activeStreams.Add(-1)
			}()
			handleTezosStream(ctx, ns.Balancer, upstreamPath, h.metrics, h.logger)
			return
		}

		archiveOnly := filter.NeedsTezosArchive(upstreamPathStr, ns.RecentBlocks)
		cacheable := tier != filter.TierInjection
		handleTezos(ctx, ns.Balancer, ns.Cache, network, upstreamPath, cacheable, archiveOnly, ns.Fallbacks, h.metrics, h.logger)
		return
	}

	// /etherlink/<network>/...
	if bytes.HasPrefix(path, prefixEtherlink) {
		rest := path[len(prefixEtherlink):]
		slashIdx := bytes.IndexByte(rest, slashByte)

		var network string
		var evmSubPath string
		if slashIdx < 0 {
			network = string(rest)
			evmSubPath = "/"
		} else {
			network = string(rest[:slashIdx])
			evmSubPath = unsafeString(rest[slashIdx:])
		}

		ns, ok := h.etherlinkNetworks[network]
		if !ok {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetBodyString("unknown etherlink network")
			return
		}

		// GET on the base path → redirect to domain root
		if string(ctx.Method()) == "GET" && evmSubPath == "/" {
			redirectToDomain(ctx)
			return
		}

		// Block private REST paths
		if !filter.CheckEVMPath(evmSubPath) {
			ctx.SetStatusCode(fasthttp.StatusForbidden)
			ctx.SetBodyString("forbidden")
			return
		}

		handleEVM(ctx, ns.Balancer, ns.Cache, ns.RecentBlocks, network, ns.Fallbacks, h.metrics, h.limiter, h.logger)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusNotFound)
	ctx.SetBodyString("not found")
}

// clientIP extracts the client IP from the request, stripping the port.
// Trusts X-Real-IP (typically set by a reverse proxy to the verified client IP)
// over X-Forwarded-For. If neither is present, falls back to RemoteAddr.
// WARNING: When not behind a trusted reverse proxy, clients can spoof these
// headers to bypass per-IP rate limiting.
func clientIP(ctx *fasthttp.RequestCtx) string {
	// X-Real-IP is preferred — set by trusted reverse proxies to the verified client IP
	if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
		return string(xri)
	}
	if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
		for i, b := range xff {
			if b == ',' {
				return string(xff[:i])
			}
		}
		return string(xff)
	}
	addr := ctx.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// getIPStreamCounter returns the atomic counter for the given IP's active streams.
func (h *Handler) getIPStreamCounter(ip string) *atomic.Int64 {
	if ctr, ok := h.ipStreams.Load(ip); ok {
		return ctr
	}
	ctr := &atomic.Int64{}
	actual, _ := h.ipStreams.LoadOrStore(ip, ctr)
	return actual
}

// redirectToDomain sends a 301 redirect to the domain root, derived from the
// Host header. Strips any port and sub-path.
func redirectToDomain(ctx *fasthttp.RequestCtx) {
	host := string(ctx.Host())
	// Strip port if present
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			host = host[:i]
			break
		}
		if host[i] < '0' || host[i] > '9' {
			break
		}
	}

	scheme := "https"
	if ctx.IsTLS() || string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
		scheme = "https"
	} else if len(ctx.Request.Header.Peek("X-Forwarded-Proto")) == 0 {
		scheme = "https" // default to https behind reverse proxy
	}

	ctx.SetStatusCode(fasthttp.StatusMovedPermanently)
	ctx.Response.Header.Set("Location", scheme+"://"+host+"/")
}

// unsafeString converts a byte slice to a string without copying.
// The string shares the byte slice's memory — the caller must ensure
// the bytes are not modified while the string is in use.
func unsafeString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// handleReady returns 200 if at least one network has a healthy node, 503 otherwise.
func (h *Handler) handleReady(ctx *fasthttp.RequestCtx) {
	for _, ns := range h.tezosNetworks {
		if ns.Balancer.Pick(false) != nil {
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.SetBodyString("ready")
			return
		}
	}
	for _, ns := range h.etherlinkNetworks {
		if ns.Balancer.Pick(false) != nil {
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.SetBodyString("ready")
			return
		}
	}
	ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
	ctx.SetBodyString("no healthy nodes")
}

// unsafeBytes converts a string to a byte slice without copying.
// The slice shares the string's memory — the caller must not modify the bytes.
func unsafeBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
