package metrics

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestBuildAsyncValidationBaseMetricInstance(t *testing.T) {
	t.Parallel()
	vm := validation.NewValidationMetrics()
	vm.SetTotalObjects(10)
	vm.IncrementValidated()
	vm.IncrementFailed()
	vm.Finalize()

	payload := []byte(`{"k":"v"}`)
	inst, err := BuildAsyncValidationBaseMetricInstance(vm, "op-123", "BAS-testnanoid", payload)
	if err != nil {
		t.Fatal(err)
	}
	if inst[objects.FieldKeyKind] != "base_metric" {
		t.Fatalf("kind: got %v", inst[objects.FieldKeyKind])
	}
	if inst[objects.FieldKeySource] != MetricSourceAsyncValidation {
		t.Fatalf("source: got %v", inst[objects.FieldKeySource])
	}
	ctxMap, _ := inst[objects.FieldKeyContext].(map[string]any)
	if ctxMap == nil {
		t.Fatalf("context map missing or not a map")
	}
	if ctxMap[objects.FieldKeyOperationID] != "op-123" {
		t.Fatalf("operation_id: got %v", ctxMap[objects.FieldKeyOperationID])
	}
	if ctxMap[objects.FieldKeyValidationMetricsJSON] != `{"k":"v"}` {
		t.Fatalf("validation_metrics_json: got %q", ctxMap[objects.FieldKeyValidationMetricsJSON])
	}
	if inst[objects.FieldKeyMetricTypeSpecific] != "validation_async_check" {
		t.Fatalf("metric_type_specific: got %v", inst[objects.FieldKeyMetricTypeSpecific])
	}
	title, _ := inst[objects.FieldKeyTitle].(string)
	if len(title) < 5 || len(title) > 120 {
		t.Fatalf("title length out of range: %d %q", len(title), title)
	}
}

func TestBuildAsyncValidationBaseMetricInstance_TruncatesLargeJSON(t *testing.T) {
	t.Parallel()
	vm := validation.NewValidationMetrics()
	vm.Finalize()
	large := make([]byte, maxValidationMetricsJSONLen+100)
	for i := range large {
		large[i] = 'x'
	}
	inst, err := BuildAsyncValidationBaseMetricInstance(vm, "", "BAS-trunc", large)
	if err != nil {
		t.Fatal(err)
	}
	ctxMap, _ := inst[objects.FieldKeyContext].(map[string]any)
	s, _ := ctxMap[objects.FieldKeyValidationMetricsJSON].(string)
	if len(s) != maxValidationMetricsJSONLen {
		t.Fatalf("expected truncated len %d, got %d", maxValidationMetricsJSONLen, len(s))
	}
	if !strings.HasSuffix(s, "\n...(truncated)") {
		t.Fatalf("expected truncation marker at end, got last 32 bytes %q", s[len(s)-32:])
	}
}

func TestAsyncValidationMetricTitleBounds(t *testing.T) {
	t.Parallel()
	s := asyncValidationMetricTitle(1, 2, 3)
	if len(s) < 5 || len(s) > 120 {
		t.Fatalf("title length: %d %q", len(s), s)
	}
	// Extreme numbers — still within 120
	s2 := asyncValidationMetricTitle(999999999, 999999999, 999999999)
	if len(s2) > 120 {
		t.Fatalf("title too long: %d", len(s2))
	}
}
