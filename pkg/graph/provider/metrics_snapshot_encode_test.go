package provider

import (
	"errors"
	"testing"
	"time"
)

func TestEncodeMetricsSnapshotForPersistence(t *testing.T) {
	t.Parallel()
	s := MetricsSnapshot{
		Timestamp: time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC),
		Operations: map[string]OperationMetrics{
			"op1": {
				Operation:     "op1",
				Count:         2,
				SuccessCount:  1,
				FailureCount:  1,
				TotalDuration: time.Second,
				ErrorRate:     50,
				LastError:     errors.New("boom"),
			},
		},
		TotalOperations: 2,
		Queries: map[string]QueryMetrics{
			"q1": {Query: "MATCH (n) RETURN n", Count: 1, TotalRows: 3},
		},
	}
	b, err := EncodeMetricsSnapshotForPersistence(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 50 {
		t.Fatalf("expected JSON payload, got %q", string(b))
	}
}
