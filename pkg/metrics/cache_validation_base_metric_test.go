package metrics

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestBuildCacheValidationBaseMetricInstance(t *testing.T) {
	t.Parallel()
	meta := storage.CacheMetricsEncodeMeta{
		WindowStart:           time.Date(2026, 3, 28, 1, 2, 3, 0, time.UTC),
		WindowEnd:             time.Date(2026, 3, 28, 1, 2, 4, 0, time.UTC),
		AsyncStrategyKinds:    2,
		KindValidationMetrics: 3,
		TotalValidationsSum:   99,
	}
	inst, err := BuildCacheValidationBaseMetricInstance("BAS-cachetest", []byte(`{"x":1}`), meta)
	if err != nil {
		t.Fatal(err)
	}
	if inst[objects.FieldKeySource] != MetricSourceCASValidationCache {
		t.Fatalf("source: %v", inst[objects.FieldKeySource])
	}
	ctxMap, _ := inst[objects.FieldKeyContext].(map[string]any)
	if ctxMap == nil {
		t.Fatalf("context map missing or not a map")
	}
	if ctxMap[objects.FieldKeyCasValidationCacheMetricsJSON] != `{"x":1}` {
		t.Fatalf("json: %v", ctxMap[objects.FieldKeyCasValidationCacheMetricsJSON])
	}
	title, _ := inst[objects.FieldKeyTitle].(string)
	if len(title) < 5 || len(title) > 120 {
		t.Fatalf("title: %q", title)
	}
}
