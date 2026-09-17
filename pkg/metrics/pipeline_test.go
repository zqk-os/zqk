package metrics

import (
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestMetricPipeline_Sample_UsesSingleSharedSamplerManyKinds(t *testing.T) {
	t.Parallel()

	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() {
		_ = reg.StopAll() //nolint:errcheck // test cleanup
	})

	// Seed the shared fallback sampler with no flush ticker and a global batch so StopAll does not
	// flush hundreds of per-kind batches (each would wait on async metric creation).
	cfg := DefaultSamplerConfig()
	cfg.ObjectKind = samplerRegistryWildcardObjectKind
	cfg.FieldName = ""
	cfg.MetricType = MetricTypeSystem
	cfg.FlushInterval = 0
	cfg.GroupByObjectID = false
	cfg.BatchSize = 10000
	cfg.MaxBatchSize = 10000
	if err := reg.RegisterSampler(cfg); err != nil {
		t.Fatalf("RegisterSampler: %v", err)
	}

	mp := NewMetricPipeline(storage.NewNoopObjectStorage())
	mp.SetSamplerRegistry(reg)

	for i := 0; i < 500; i++ {
		ev := map[string]any{
			objects.FieldKeyKind: fmt.Sprintf("pipeline_kind_%d", i),
		}
		_, err := mp.Sample(ev)
		if err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
	}

	stats := reg.GetStats()
	if dc, ok := stats["distinct_sampler_count"].(int); !ok || dc != 1 {
		t.Fatalf("expected distinct_sampler_count 1 (shared pipeline sampler), got %#v stats=%v",
			stats["distinct_sampler_count"], stats)
	}
}
