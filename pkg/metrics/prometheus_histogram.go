package metrics

import (
	"sync"
)

// PrometheusHistogram tracks values in buckets, similar to Prometheus histograms.
// It is useful for monitoring agent-to-agent IPC latency.
type PrometheusHistogram struct {
	mu      sync.RWMutex
	buckets []float64 // Upper bounds for buckets
	counts  []uint64  // Cumulative counts
	sum     float64   // Sum of all observations
	count   uint64    // Total number of observations
}

// NewPrometheusHistogram creates a new Prometheus-style histogram with specified buckets.
// It assumes the buckets are sorted in ascending order.
func NewPrometheusHistogram(buckets []float64) *PrometheusHistogram {
	return &PrometheusHistogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
}

// Observe records a value and increments appropriate cumulative buckets.
func (h *PrometheusHistogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += value
	h.count++

	for i, bound := range h.buckets {
		if value <= bound {
			h.counts[i]++
		}
	}
}

// GetBuckets returns the current buckets and their cumulative counts.
func (h *PrometheusHistogram) GetBuckets() ([]float64, []uint64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	bounds := make([]float64, len(h.buckets))
	copy(bounds, h.buckets)

	counts := make([]uint64, len(h.counts))
	copy(counts, h.counts)

	return bounds, counts
}

// GetStats returns the total sum and count.
func (h *PrometheusHistogram) GetStats() (sum float64, count uint64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sum, h.count
}
