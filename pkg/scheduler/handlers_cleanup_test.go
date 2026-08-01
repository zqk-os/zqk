package scheduler

import (
	"errors"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestBuildBatchMetricErrorRecord(t *testing.T) {
	t.Parallel()

	metricID := "BAS-123"
	batchID := "BATCH-abc"
	nowStr := "2026-03-16T20:00:00Z"
	createErr := errors.New("validation errors block save: invalid status")

	obj := buildBatchMetricErrorRecord(metricID, batchID, nowStr, createErr)
	if obj == nil {
		t.Fatal("buildBatchMetricErrorRecord returned nil")
	}

	if got := obj[objects.FieldKeyID]; got != metricID {
		t.Errorf("id = %v, want %s", got, metricID)
	}
	if got := obj[objects.FieldKeyKind]; got != "base_metric" {
		t.Errorf("kind = %v, want base_metric", got)
	}
	if got := obj[objects.FieldKeyStatus]; got != "error" {
		t.Errorf("status = %v, want error", got)
	}
	if got := obj[objects.FieldKeyBatchID]; got != batchID {
		t.Errorf("batch_id = %v, want %s", got, batchID)
	}
	title, _ := obj[objects.FieldKeyTitle].(string)
	if title == emptyValue || len(title) > maxErrorTitleLength {
		t.Errorf("title length invalid: len=%d max=%d", len(title), maxErrorTitleLength)
	}
	if !strings.Contains(title, batchID) || !strings.Contains(title, "create failed") {
		t.Errorf("title should mention batch and failure: %q", title)
	}
}
