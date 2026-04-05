package filter

// Tier classifies a request for rate limiting purposes.
type Tier int

const (
	TierDefault   Tier = iota // 100 req/s per IP
	TierExpensive             // 20 req/s per IP
	TierInjection             // 10 req/s per IP
	TierScript                // 5 req/s per IP
	TierStreaming              // 5 req/s per IP (concurrency limiting deferred to v2)
	TierDebug                 // 1 req/s per IP
	NumTiers                  // sentinel
)

func (t Tier) String() string {
	switch t {
	case TierDefault:
		return "default"
	case TierExpensive:
		return "expensive"
	case TierInjection:
		return "injection"
	case TierScript:
		return "script"
	case TierStreaming:
		return "streaming"
	case TierDebug:
		return "debug"
	default:
		return "unknown"
	}
}
