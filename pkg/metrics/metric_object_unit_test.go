package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestTopNMap(t *testing.T) {
	counts := map[string]int{
		"apple":  10,
		"banana": 50,
		"cherry": 30,
		"date":   5,
	}

	top2 := TopNMap(counts, 2)
	assert.Len(t, top2, 2)
	assert.Equal(t, 50, top2["banana"])
	assert.Equal(t, 30, top2["cherry"])

	topAll := TopNMap(counts, 10)
	assert.Len(t, topAll, 4)

	topZero := TopNMap(counts, 0)
	assert.Empty(t, topZero)
}

func TestMetricObject_Helpers(t *testing.T) {
	// ComputeEffectiveObjectCount
	assert.Equal(t, 0, ComputeEffectiveObjectCount(-5, 10))
	assert.Equal(t, 10, ComputeEffectiveObjectCount(0, 10))
	assert.Equal(t, 42, ComputeEffectiveObjectCount(42, 10))

	// FormatMetricWindow
	now := time.Now().UTC()
	startStr, endStr, err := FormatMetricWindow("METRIC-001", now, now.Add(time.Hour))
	require.NoError(t, err)
	assert.NotEmpty(t, startStr)
	assert.NotEmpty(t, endStr)

	_, _, err = FormatMetricWindow("", now, now.Add(time.Hour))
	assert.Error(t, err)

	// ShouldFlushMetrics with nil storage
	logger, ok := ShouldFlushMetrics(nil, nil)
	assert.Nil(t, logger)
	assert.False(t, ok)

	// FlushMetricsIfReady with nil storage
	called := false
	FlushMetricsIfReady(nil, nil, func(l logging.Logger) {
		called = true
	})
	assert.False(t, called)

	// FlushCacheValidationMetricsToStorage with nil storage
	FlushCacheValidationMetricsToStorage(nil, nil)

	// FlushGraphProviderMetricsToStorage with nil storage
	FlushGraphProviderMetricsToStorage(nil, nil)
}

func TestComputeEffortVariance(t *testing.T) {
	// Zero estimated effort
	assert.Equal(t, 0.0, ComputeEffortVariance("0h", "10h"))
	assert.Equal(t, 0.0, ComputeEffortVariance("", "10h"))

	// Positive variance (actual > estimated)
	v := ComputeEffortVariance("10", "15")
	assert.InDelta(t, 50.0, v, 0.001)

	// Negative variance (actual < estimated)
	v2 := ComputeEffortVariance("10", "5")
	assert.InDelta(t, -50.0, v2, 0.001)

	// Exact match
	v3 := ComputeEffortVariance("10", "10")
	assert.InDelta(t, 0.0, v3, 0.001)
}

func TestBuildBaseMetricObject_And_AggregationResult(t *testing.T) {
	now := time.Now().UTC()
	cfg := &AggregationConfig{
		ObjectKind:  "task",
		FieldName:   "status",
		MetricType:  "system_task_metrics",
		WindowStart: now.Add(-time.Hour),
		WindowEnd:   now,
	}

	// 1. Default config
	obj1 := CreateBaseMetricObject(cfg, "Task Status Metric", 10, nil)
	assert.NotNil(t, obj1)
	assert.Equal(t, "Task Status Metric", obj1[objects.FieldKeyTitle])
	assert.Equal(t, 10, obj1[objects.FieldKeyCollectionCount])
	assert.Equal(t, DefaultMetricNamespace, obj1[objects.FieldKeyNamespaceID])
	assert.Equal(t, DefaultMetricOriginProject, obj1[objects.FieldKeyOriginProject])
	assert.Equal(t, DefaultMetricOriginSystem, obj1[objects.FieldKeyOriginSystem])

	// 2. Custom config with Sampled = true
	customCfg := &MetricObjectConfig{
		Sampled:        true,
		Source:         "custom_source",
		NamespaceID:    "ns-custom",
		OriginSystem:   "sys-custom",
		OriginProject:  "proj-custom",
		AdditionalTags: []string{"custom-tag-1"},
	}
	obj2 := CreateBaseMetricObject(cfg, "Sampled Metric", 25, customCfg)
	assert.NotNil(t, obj2)
	assert.True(t, obj2[objects.FieldKeySampled].(bool))
	assert.Equal(t, "custom_source", obj2[objects.FieldKeySource])
	assert.Equal(t, "ns-custom", obj2[objects.FieldKeyNamespaceID])
	assert.Equal(t, "sys-custom", obj2[objects.FieldKeyOriginSystem])
	assert.Equal(t, "proj-custom", obj2[objects.FieldKeyOriginProject])

	// 3. BuildAggregationResult
	res := BuildAggregationResult(cfg, "METRIC-RES-001", map[string]any{"count": 25}, 25)
	assert.NotNil(t, res)
	assert.Equal(t, "METRIC-RES-001", res.MetricID)
	assert.Equal(t, 25, res.ObjectCount)
	assert.Equal(t, 1, res.CollectionCount)
	assert.Equal(t, "task", res.ObjectKind)
}
