package metrics

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestBuildGraphProviderMetricsBaseMetricInstance(t *testing.T) {
	t.Parallel()
	meta := GraphProviderPersistMeta{
		WindowStart:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		WindowEnd:       time.Date(2026, 1, 2, 3, 5, 5, 0, time.UTC),
		OperationKinds:  2,
		QueryKinds:      3,
		TotalOperations: 10,
	}
	inst, err := BuildGraphProviderMetricsBaseMetricInstance("BAS-gptest", []byte(`{"k":1}`), meta)
	if err != nil {
		t.Fatal(err)
	}
	if inst[objects.FieldKeyKind] != "base_metric" {
		t.Fatalf("kind: %v", inst[objects.FieldKeyKind])
	}
	if inst[objects.FieldKeySource] != MetricSourceGraphProvider {
		t.Fatalf("source: %v", inst[objects.FieldKeySource])
	}
	if inst[objects.FieldKeyGraphProviderMetricsJSON] != `{"k":1}` {
		t.Fatalf("json field: %v", inst[objects.FieldKeyGraphProviderMetricsJSON])
	}
	title, _ := inst[objects.FieldKeyTitle].(string)
	if len(title) < 5 || len(title) > 120 {
		t.Fatalf("title length: %d %q", len(title), title)
	}
}
