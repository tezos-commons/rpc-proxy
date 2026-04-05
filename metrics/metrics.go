package metrics

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tezos-commons/rpc-proxy/log"
)

// Metrics tracks request counts and error counts per 1-second interval.
type Metrics struct {
	requests  atomic.Int64
	errors    atomic.Int64
	cacheHits atomic.Int64
	logger    *log.Logger
}

func New(logger *log.Logger) *Metrics {
	return &Metrics{logger: logger}
}

func (m *Metrics) RecordRequest() {
	m.requests.Add(1)
}

func (m *Metrics) RecordError() {
	m.errors.Add(1)
}

func (m *Metrics) RecordCacheHit() {
	m.cacheHits.Add(1)
}

// StartLogger logs request/error stats every 1 second. Skips idle intervals.
func (m *Metrics) StartLogger(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqs := m.requests.Swap(0)
			errs := m.errors.Swap(0)
			hits := m.cacheHits.Swap(0)
			if reqs == 0 && errs == 0 && hits == 0 {
				continue
			}

			var errorRate float64
			if reqs > 0 {
				errorRate = float64(errs) / float64(reqs) * 100
			}

			m.logger.Info(fmt.Sprintf("%s %d req/s, %d errors (%.1f%%), %d cache hits",
				log.Tag("stats"), reqs, errs, errorRate, hits))
		}
	}
}
