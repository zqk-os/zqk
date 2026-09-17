package file

import (
	"testing"
	"time"
)

func TestContentionHistogram_RecordAndBuckets(t *testing.T) {
	h := NewContentionHistogram()

	// Record samples in various ranges
	h.Record(500 * time.Microsecond) // < 1ms
	h.Record(2 * time.Millisecond)   // 1-5ms
	h.Record(10 * time.Millisecond)  // 5-25ms
	h.Record(50 * time.Millisecond)  // 25-100ms
	h.Record(200 * time.Millisecond) // 100-500ms
	h.Record(1 * time.Second)        // > 500ms

	snap := h.Snapshot()

	if snap.TotalSamples != 6 {
		t.Fatalf("expected 6 total samples, got %d", snap.TotalSamples)
	}

	if snap.Under1ms != 1 || snap.Between1And5ms != 1 || snap.Between5And25ms != 1 ||
		snap.Between25And100ms != 1 || snap.Between100And500ms != 1 || snap.Over500ms != 1 {
		t.Fatalf("unexpected bucket counts: %+v", snap)
	}
}

func TestContentionHistogram_Percentiles(t *testing.T) {
	h := NewContentionHistogram()

	// Populate 100 samples: 80 in <1ms, 15 in 1-5ms, 5 in 5-25ms
	for i := 0; i < 80; i++ {
		h.Record(500 * time.Microsecond)
	}
	for i := 0; i < 15; i++ {
		h.Record(3 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		h.Record(15 * time.Millisecond)
	}

	snap := h.Snapshot()
	p50 := snap.EstimatedP50()
	p90 := snap.EstimatedP90()
	p99 := snap.EstimatedP99()

	if p50 > 1*time.Millisecond {
		t.Fatalf("expected p50 <= 1ms, got %v", p50)
	}

	if p90 <= 1*time.Millisecond || p90 > 5*time.Millisecond {
		t.Fatalf("expected p90 between 1ms and 5ms, got %v", p90)
	}

	if p99 <= 5*time.Millisecond {
		t.Fatalf("expected p99 > 5ms, got %v", p99)
	}
}
