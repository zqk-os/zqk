package storage

import (
	"testing"
)

func TestEncodeCacheMetricsForPersistence(t *testing.T) {
	GetValidationMetrics("test_kind_encode").RecordValidation(1000, 5, 1)
	data, meta, err := EncodeCacheMetricsForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 20 {
		t.Fatalf("short payload: %s", string(data))
	}
	if meta.KindValidationMetrics < 1 {
		t.Fatalf("expected at least one kind row, got %+v", meta)
	}
}
