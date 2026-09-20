package metrics

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
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
	if len(title) < 5 {
		t.Fatalf("title is too short: %d %q", len(title), title)
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

func TestAsyncValidationMetricTitle(t *testing.T) {
	t.Parallel()
	s := asyncValidationMetricTitle(1, 2, 3)
	if s != "Async check: 1 validated, 2 failed of 3" {
		t.Fatalf("title: %q", s)
	}
}
