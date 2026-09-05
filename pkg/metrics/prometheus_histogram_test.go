package metrics

import (
	"testing"
)

func TestPrometheusHistogram(t *testing.T) {
	buckets := []float64{1.0, 5.0, 10.0, 50.0}
	h := NewPrometheusHistogram(buckets)

	// Observe values
	h.Observe(2.0)
	h.Observe(4.0)
	h.Observe(10.0)
	h.Observe(20.0)

	bounds, counts := h.GetBuckets()

	if len(bounds) != len(buckets) {
		t.Errorf("Expected %d bounds, got %d", len(buckets), len(bounds))
	}

	expectedCounts := []uint64{0, 2, 3, 4}

	for i, c := range expectedCounts {
		if counts[i] != c {
			t.Errorf("Bucket %f expected count %d, got %d", bounds[i], c, counts[i])
		}
	}

	sum, count := h.GetStats()
	if count != 4 {
		t.Errorf("Expected count 4, got %d", count)
	}

	expectedSum := 2.0 + 4.0 + 10.0 + 20.0
	if sum != expectedSum {
		t.Errorf("Expected sum %f, got %f", expectedSum, sum)
	}
}
