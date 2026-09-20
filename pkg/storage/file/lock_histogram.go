package file

import (
	"sync"
	"sync/atomic"
	"time"
)

// ContentionHistogram tracks latency distributions for lock acquisition wait times.
type ContentionHistogram struct {
	under1ms           atomic.Int64
	between1And5ms     atomic.Int64
	between5And25ms    atomic.Int64
	between25And100ms  atomic.Int64
	between100And500ms atomic.Int64
	over500ms          atomic.Int64
	totalSamples       atomic.Int64
}

// HistogramSnapshot represents an immutable point-in-time view of lock latency distributions.
type HistogramSnapshot struct {
	Under1ms           int64
	Between1And5ms     int64
	Between5And25ms    int64
	Between25And100ms  int64
	Between100And500ms int64
	Over500ms          int64
	TotalSamples       int64
}

var (
	globalContentionHistogram     *ContentionHistogram
	globalContentionHistogramOnce sync.Once
)

// GetGlobalContentionHistogram returns the global singleton ContentionHistogram.
func GetGlobalContentionHistogram() *ContentionHistogram {
	globalContentionHistogramOnce.Do(func() {
		globalContentionHistogram = NewContentionHistogram()
	})
	return globalContentionHistogram
}

// NewContentionHistogram creates an initialized ContentionHistogram.
func NewContentionHistogram() *ContentionHistogram {
	return &ContentionHistogram{}
}

// Record classifies the duration into its corresponding latency bucket.
func (h *ContentionHistogram) Record(d time.Duration) {
	h.totalSamples.Add(1)

	switch {
	case d < 1*time.Millisecond:
		h.under1ms.Add(1)
	case d < 5*time.Millisecond:
		h.between1And5ms.Add(1)
	case d < 25*time.Millisecond:
		h.between5And25ms.Add(1)
	case d < 100*time.Millisecond:
		h.between25And100ms.Add(1)
	case d < 500*time.Millisecond:
		h.between100And500ms.Add(1)
	default:
		h.over500ms.Add(1)
	}
}

// Snapshot returns a point-in-time copy of all histogram bucket counts.
func (h *ContentionHistogram) Snapshot() HistogramSnapshot {
	return HistogramSnapshot{
		Under1ms:           h.under1ms.Load(),
		Between1And5ms:     h.between1And5ms.Load(),
		Between5And25ms:    h.between5And25ms.Load(),
		Between25And100ms:  h.between25And100ms.Load(),
		Between100And500ms: h.between100And500ms.Load(),
		Over500ms:          h.over500ms.Load(),
		TotalSamples:       h.totalSamples.Load(),
	}
}

// EstimatedP50 returns the estimated median latency from bucket distribution.
func (s HistogramSnapshot) EstimatedP50() time.Duration {
	return s.estimatedPercentile(0.50)
}

// EstimatedP90 returns the estimated 90th percentile latency.
func (s HistogramSnapshot) EstimatedP90() time.Duration {
	return s.estimatedPercentile(0.90)
}

// EstimatedP99 returns the estimated 99th percentile latency.
func (s HistogramSnapshot) EstimatedP99() time.Duration {
	return s.estimatedPercentile(0.99)
}

func (s HistogramSnapshot) estimatedPercentile(pct float64) time.Duration {
	if s.TotalSamples == 0 {
		return 0
	}

	target := int64(float64(s.TotalSamples) * pct)
	var accum int64

	accum += s.Under1ms
	if accum >= target {
		return 500 * time.Microsecond
	}
	accum += s.Between1And5ms
	if accum >= target {
		return 3 * time.Millisecond
	}
	accum += s.Between5And25ms
	if accum >= target {
		return 15 * time.Millisecond
	}
	accum += s.Between25And100ms
	if accum >= target {
		return 60 * time.Millisecond
	}
	accum += s.Between100And500ms
	if accum >= target {
		return 300 * time.Millisecond
	}
	return 750 * time.Millisecond
}
