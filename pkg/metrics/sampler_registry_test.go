package metrics

import (
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"
)

func TestSamplerRegistry_RegisterSampler_SystemMetrics_DistinctCapped(t *testing.T) {
	t.Parallel()

	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() {
		_ = reg.StopAll() //nolint:errcheck // test cleanup
	})

	cfgTemplate := DefaultSamplerConfig()
	cfgTemplate.MetricType = MetricTypeSystem
	cfgTemplate.FieldName = ""
	cfgTemplate.FlushInterval = 0 // no flush ticker goroutine; cap is about distinct *Sampler instances

	for i := range maxSamplers {
		cfg := *cfgTemplate
		cfg.ObjectKind = fmt.Sprintf("kind_%d", i)
		if err := reg.RegisterSampler(&cfg); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}

	stats := reg.GetStats()
	if dc, ok := stats["distinct_sampler_count"].(int); !ok || dc != maxSamplers {
		t.Fatalf("expected distinct_sampler_count %d, got %#v", maxSamplers, stats["distinct_sampler_count"])
	}

	// One more system sampler must alias an existing instance (no new distinct sampler).
	cfgExtra := *cfgTemplate
	cfgExtra.ObjectKind = "kind_overflow"
	if err := reg.RegisterSampler(&cfgExtra); err != nil {
		t.Fatalf("register overflow: %v", err)
	}
	stats = reg.GetStats()
	if dc, ok := stats["distinct_sampler_count"].(int); !ok || dc != maxSamplers {
		t.Fatalf("after overflow expected distinct_sampler_count still %d, got %#v (stats=%v)", maxSamplers, stats["distinct_sampler_count"], stats)
	}

	seen := make(map[*Sampler]struct{})
	for i := range maxSamplers {
		s := reg.GetSampler(fmt.Sprintf("kind_%d", i), "", MetricTypeSystem)
		if s == nil {
			t.Fatalf("nil sampler for kind_%d", i)
		}
		seen[s] = struct{}{}
	}
	over := reg.GetSampler("kind_overflow", "", MetricTypeSystem)
	if over == nil {
		t.Fatal("nil sampler for overflow kind")
	}
	seen[over] = struct{}{}
	if len(seen) != maxSamplers {
		t.Fatalf("expected %d unique *Sampler pointers across kinds+overflow, got %d", maxSamplers, len(seen))
	}
}

func TestSamplerRegistry_RegisterSampler_NonSystem_ReturnsErrorAtCap(t *testing.T) {
	t.Parallel()

	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() {
		_ = reg.StopAll() //nolint:errcheck // test cleanup
	})

	cfg := DefaultSamplerConfig()
	cfg.MetricType = MetricTypeScalar
	cfg.FieldName = "f"
	cfg.FlushInterval = 0

	for i := range maxSamplers {
		c := *cfg
		c.ObjectKind = fmt.Sprintf("ob_%d", i)
		if err := reg.RegisterSampler(&c); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}

	cExtra := *cfg
	cExtra.ObjectKind = "ob_overflow"
	if err := reg.RegisterSampler(&cExtra); err == nil {
		t.Fatal("expected error when registry is at capacity for non-system registration")
	}
}

func TestSamplerRegistry_PipelineSampleSampler_DoesNotGroupByObjectID(t *testing.T) {
	t.Parallel()
	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() {
		_ = reg.StopAll() //nolint:errcheck // test cleanup
	})
	s, err := reg.GetOrCreatePipelineSampleSampler()
	if err != nil {
		t.Fatalf("GetOrCreatePipelineSampleSampler: %v", err)
	}
	if s.config.GroupByObjectID {
		t.Fatal("pipeline sample sampler GroupByObjectID must be false so system check events do not create one metric batch per object")
	}
}
