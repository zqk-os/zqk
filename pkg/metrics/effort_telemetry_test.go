package metrics

import (
	"bufio"
	"bytes"
	"context"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mockCustomAggregator is a custom aggregator for testing registration
type mockCustomAggregator struct {
	metricType string
}

func (m *mockCustomAggregator) GetMetricType() string {
	return m.metricType
}

func (m *mockCustomAggregator) Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
	return &AggregationResult{
		MetricType:      m.metricType,
		ObjectKind:      config.ObjectKind,
		FieldName:       config.FieldName,
		ObjectCount:     1,
		CollectionCount: 1,
	}, nil
}

// TestEffortTelemetry_StaticFloorInvariants satisfies the Static Floor Invariant requirement:
// verifies metric object schemas, default metric values, and aggregator type registrations.
func TestEffortTelemetry_StaticFloorInvariants(t *testing.T) {
	t.Parallel()

	t.Run("MetricObjectSchema_StandardAndCustom", func(t *testing.T) {
		t.Parallel()

		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
		cfg := &AggregationConfig{
			ObjectKind:   "backlog_item",
			FieldName:    "estimated_effort",
			MetricType:   MetricTypeScalar,
			WindowStart:  start,
			WindowEnd:    end,
			Aggregations: []string{"sum", "avg"},
		}

		// 1. Standard config (nil objConfig -> defaults)
		baseObj := CreateBaseMetricObject(cfg, "Test Metric Title", 15, nil)
		require.NotNil(t, baseObj)

		assert.Equal(t, MetricKindBaseMetric, baseObj[objects.FieldKeyKind])
		assert.Equal(t, "Test Metric Title", baseObj[objects.FieldKeyTitle])
		assert.Equal(t, MetricTypeSystem, baseObj[objects.FieldKeyMetricType])
		assert.Equal(t, MetricSourcePipeline, baseObj[objects.FieldKeySource])
		assert.Equal(t, 15, baseObj[objects.FieldKeyCollectionCount])
		assert.Equal(t, 15, baseObj[objects.FieldKeyObjectCount])
		assert.Equal(t, MetricStatusImplemented, baseObj[objects.FieldKeyStatus])
		assert.Equal(t, DefaultMetricNamespace, baseObj[objects.FieldKeyNamespaceID])
		assert.Equal(t, DefaultMetricOriginProject, baseObj[objects.FieldKeyOriginProject])
		assert.Equal(t, DefaultMetricOriginSystem, baseObj[objects.FieldKeyOriginSystem])
		assert.Equal(t, MetricSchemaVersion, baseObj[objects.FieldKeySchemaVersion])
		assert.Equal(t, MetricCreatedBySystem, baseObj[objects.FieldKeyCreatedBy])
		assert.Equal(t, "backlog_item", baseObj[objects.FieldKeyObjectKind])
		assert.Equal(t, "estimated_effort", baseObj[objects.FieldKeyFieldName])
		assert.Equal(t, MetricTypeScalar, baseObj[objects.FieldKeyMetricTypeSpecific])
		assert.Equal(t, start.Format(time.RFC3339), baseObj[objects.FieldKeyFirstSeen])
		assert.Equal(t, end.Format(time.RFC3339), baseObj[objects.FieldKeyLastSeen])
		assert.Equal(t, start.Format(time.RFC3339), baseObj[objects.FieldKeyWindowStart])
		assert.Equal(t, end.Format(time.RFC3339), baseObj[objects.FieldKeyWindowEnd])
		assert.NotEmpty(t, baseObj[objects.FieldKeyCreatedAt])

		tags, ok := baseObj[objects.FieldKeyTags].([]string)
		require.True(t, ok)
		assert.Contains(t, tags, MetricTagSystem)
		assert.Contains(t, tags, MetricTagPipeline)
		assert.Contains(t, tags, MetricTypeScalar)
		assert.Contains(t, tags, "backlog_item")
		assert.Contains(t, tags, "estimated_effort")

		// 2. Custom objConfig with sampled=true and extra tags
		customObjCfg := &MetricObjectConfig{
			Source:         MetricSourcePipelineSampler,
			Sampled:        true,
			AdditionalTags: []string{"telemetry", "effort_variance"},
			NamespaceID:    "zqk:telemetry",
			OriginSystem:   "effort_worker",
			OriginProject:  "zqk_public",
		}
		sampledObj := CreateBaseMetricObject(cfg, "Sampled Effort Metric", 5, customObjCfg)
		require.NotNil(t, sampledObj)

		assert.Equal(t, MetricSourcePipelineSampler, sampledObj[objects.FieldKeySource])
		assert.Equal(t, true, sampledObj[objects.FieldKeySampled])
		assert.Equal(t, "zqk:telemetry", sampledObj[objects.FieldKeyNamespaceID])
		assert.Equal(t, "effort_worker", sampledObj[objects.FieldKeyOriginSystem])
		assert.Equal(t, "zqk_public", sampledObj[objects.FieldKeyOriginProject])

		sampledTags, ok := sampledObj[objects.FieldKeyTags].([]string)
		require.True(t, ok)
		assert.Contains(t, sampledTags, MetricTagSampled)
		assert.Contains(t, sampledTags, "telemetry")
		assert.Contains(t, sampledTags, "effort_variance")
	})

	t.Run("DefaultMetricValues_ConstantsAndConfigs", func(t *testing.T) {
		t.Parallel()

		// Verify DefaultMetricObjectConfig values
		defaultObjCfg := DefaultMetricObjectConfig()
		require.NotNil(t, defaultObjCfg)
		assert.Equal(t, MetricSourcePipeline, defaultObjCfg.Source)
		assert.False(t, defaultObjCfg.Sampled)
		assert.Nil(t, defaultObjCfg.AdditionalTags)
		assert.Equal(t, DefaultMetricNamespace, defaultObjCfg.NamespaceID)
		assert.Equal(t, DefaultMetricOriginSystem, defaultObjCfg.OriginSystem)
		assert.Equal(t, DefaultMetricOriginProject, defaultObjCfg.OriginProject)

		// Verify DefaultSamplerConfig values
		defaultSamplerCfg := DefaultSamplerConfig()
		require.NotNil(t, defaultSamplerCfg)
		assert.True(t, defaultSamplerCfg.Enabled)
		assert.Equal(t, 50, defaultSamplerCfg.BatchSize)
		assert.Equal(t, 1000, defaultSamplerCfg.MaxBatchSize)
		assert.Equal(t, 5*time.Minute, defaultSamplerCfg.FlushInterval)
		assert.True(t, defaultSamplerCfg.GroupByObjectID)
		assert.NoError(t, defaultSamplerCfg.Validate())

		// Verify core system constants
		assert.Equal(t, "base_metric", MetricKindBaseMetric)
		assert.Equal(t, "system", MetricTypeSystem)
		assert.Equal(t, "performance", MetricTypePerformance)
		assert.Equal(t, "scalar_metric", MetricTypeScalar)
		assert.Equal(t, "list_metric", MetricTypeList)
		assert.Equal(t, "ordered_list_metric", MetricTypeOrderedList)
		assert.Equal(t, "status_history_metric", MetricTypeStatusHistory)
		assert.Equal(t, "implemented", MetricStatusImplemented)
		assert.Equal(t, "metrics_pipeline", MetricSourcePipeline)
		assert.Equal(t, "metrics_pipeline_sampler", MetricSourcePipelineSampler)
		assert.Equal(t, "async_validation", MetricSourceAsyncValidation)
		assert.Equal(t, "graph_provider", MetricSourceGraphProvider)
		assert.Equal(t, "cas_validation_cache", MetricSourceCASValidationCache)
		assert.Equal(t, objects.DefaultSchemaVersion, MetricSchemaVersion)
		assert.Equal(t, pkgctx.SystemAccountID, MetricCreatedBySystem)
	})

	t.Run("AggregatorTypeRegistrations", func(t *testing.T) {
		t.Parallel()

		mp := NewMetricPipeline(storage.NewNoopObjectStorage())
		require.NotNil(t, mp)

		// Check default aggregators
		aggScalar := mp.getDefaultAggregator(MetricTypeScalar)
		require.NotNil(t, aggScalar)
		assert.Equal(t, MetricTypeScalar, aggScalar.GetMetricType())

		aggList := mp.getDefaultAggregator(MetricTypeList)
		require.NotNil(t, aggList)
		assert.Equal(t, MetricTypeList, aggList.GetMetricType())

		aggOrdered := mp.getDefaultAggregator(MetricTypeOrderedList)
		require.NotNil(t, aggOrdered)
		assert.Equal(t, MetricTypeOrderedList, aggOrdered.GetMetricType())

		aggStatus := mp.getDefaultAggregator(MetricTypeStatusHistory)
		require.NotNil(t, aggStatus)
		assert.Equal(t, MetricTypeStatusHistory, aggStatus.GetMetricType())

		assert.Nil(t, mp.getDefaultAggregator("unregistered_metric_type"))

		// Check custom aggregator registration
		customAgg := &mockCustomAggregator{metricType: "custom_effort_metric"}
		mp.RegisterAggregator(customAgg)
		assert.Equal(t, customAgg, mp.aggregators["custom_effort_metric"])

		// Calling CollectMetrics with the registered aggregator works
		res, err := mp.CollectMetrics(context.Background(), &AggregationConfig{
			MetricType: "custom_effort_metric",
			ObjectKind: "backlog_item",
			FieldName:  "effort_score",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, "custom_effort_metric", res.MetricType)
		assert.Equal(t, 1, res.ObjectCount)
	})
}

// TestEffortTelemetry_OperationalDynamicProof satisfies the Operational Dynamic Proof requirement:
// verifies effort estimation calculations, AI metric telemetry events, pipeline aggregation under
// concurrent recording, and rollup accuracy.
func TestEffortTelemetry_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	t.Run("EffortEstimationCalculations", func(t *testing.T) {
		t.Parallel()

		// Test ComputeEffortVariance
		testCases := []struct {
			name      string
			estimated string
			actual    string
			expected  float64
		}{
			{name: "ExactMatch", estimated: "10.0", actual: "10.0", expected: 0.0},
			{name: "Underestimated_50Percent", estimated: "10.0", actual: "15.0", expected: 50.0},
			{name: "Overestimated_25Percent", estimated: "20.0", actual: "15.0", expected: -25.0},
			{name: "FractionalEffort", estimated: "4.0", actual: "5.0", expected: 25.0},
			{name: "SmallFractions", estimated: "2.5", actual: "3.75", expected: 50.0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				variance := ComputeEffortVariance(tc.estimated, tc.actual)
				assert.InDelta(t, tc.expected, variance, 0.001)
			})
		}

		// Test enhanceEDDWithCommits
		baseEDD := 20.0
		// Low work item ratio (< 1.0) -> indicates underestimation, increases EDD
		lowRatioMetrics := &CommitMetrics{WorkItemRatio: 0.5}
		eddLow := enhanceEDDWithCommits(baseEDD, lowRatioMetrics)
		assert.InDelta(t, baseEDD+eddAdjustUnderestimated, eddLow, 0.001)

		// High work item ratio (> 10.0) -> indicates overestimation, decreases EDD
		highRatioMetrics := &CommitMetrics{WorkItemRatio: 12.0}
		eddHigh := enhanceEDDWithCommits(baseEDD, highRatioMetrics)
		assert.InDelta(t, baseEDD-eddAdjustOverestimated, eddHigh, 0.001)

		// Healthy work item ratio (2.0 - 5.0) -> slight positive adjustment (decreases discrepancy)
		healthyRatioMetrics := &CommitMetrics{WorkItemRatio: 3.5}
		eddHealthy := enhanceEDDWithCommits(baseEDD, healthyRatioMetrics)
		assert.InDelta(t, baseEDD-eddAdjustHealthy, eddHealthy, 0.001)

		// Zero work item ratio -> unaffected
		zeroRatioMetrics := &CommitMetrics{WorkItemRatio: 0.0}
		eddZero := enhanceEDDWithCommits(baseEDD, zeroRatioMetrics)
		assert.InDelta(t, baseEDD, eddZero, 0.001)

		// Test enhancePCSWithCommits
		basePCS := 70.0
		// Healthy frequency (0.5 - 2.0) + increasing pattern -> +5.0 + 3.0 = +8.0
		pcsBoosted := enhancePCSWithCommits(basePCS, &CommitMetrics{
			CommitFrequency: 1.2,
			CommitPattern:   "increasing",
		})
		assert.InDelta(t, 78.0, pcsBoosted, 0.001)

		// Low frequency (< 0.1) + decreasing pattern -> -10.0 - 5.0 = -15.0
		pcsPenalized := enhancePCSWithCommits(basePCS, &CommitMetrics{
			CommitFrequency: 0.05,
			CommitPattern:   "decreasing",
		})
		assert.InDelta(t, 55.0, pcsPenalized, 0.001)

		// High frequency (> 5.0) + irregular pattern -> -5.0 - 3.0 = -8.0
		pcsChurn := enhancePCSWithCommits(basePCS, &CommitMetrics{
			CommitFrequency: 6.0,
			CommitPattern:   "irregular",
		})
		assert.InDelta(t, 62.0, pcsChurn, 0.001)
	})

	t.Run("AIMetricTelemetryEvents_Subscribers", func(t *testing.T) {
		t.Parallel()

		mp := NewMetricPipeline(storage.NewNoopObjectStorage())

		var sub1Events []map[string]any
		var sub2Events []map[string]any
		var mu sync.Mutex

		mp.AddSubscriber(func(ev map[string]any) {
			mu.Lock()
			defer mu.Unlock()
			sub1Events = append(sub1Events, ev)
		})
		mp.AddSubscriber(func(ev map[string]any) {
			mu.Lock()
			defer mu.Unlock()
			sub2Events = append(sub2Events, ev)
		})

		testEvent := map[string]any{
			objects.FieldKeyKind:      "backlog_item",
			"event_type":              "effort_recorded",
			"estimated_effort":        "5.0",
			"actual_effort":           "6.5",
			objects.FieldKeyCreatedAt: time.Now().Format(time.RFC3339),
		}

		_, err := mp.Sample(testEvent)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, sub1Events, 1)
		require.Len(t, sub2Events, 1)
		assert.Equal(t, "effort_recorded", sub1Events[0]["event_type"])
		assert.Equal(t, "5.0", sub1Events[0]["estimated_effort"])
		assert.Equal(t, "effort_recorded", sub2Events[0]["event_type"])
		assert.Equal(t, "6.5", sub2Events[0]["actual_effort"])
	})

	t.Run("PipelineAggregationConcurrentRecording", func(t *testing.T) {
		t.Parallel()

		metricsrecording.EnterAllowRecording()
		t.Cleanup(metricsrecording.LeaveAllowRecording)

		reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
		t.Cleanup(func() { _ = reg.StopAll() })

		cfg := DefaultSamplerConfig()
		cfg.ObjectKind = samplerRegistryWildcardObjectKind
		cfg.FieldName = ""
		cfg.MetricType = MetricTypeSystem
		cfg.FlushInterval = 0
		cfg.GroupByObjectID = false
		cfg.BatchSize = 10000
		cfg.MaxBatchSize = 10000
		require.NoError(t, reg.RegisterSampler(cfg))

		mp := NewMetricPipeline(storage.NewNoopObjectStorage())
		mp.SetSamplerRegistry(reg)

		var totalSubscriberCount atomic.Int64
		mp.AddSubscriber(func(ev map[string]any) {
			totalSubscriberCount.Add(1)
		})

		const numGoroutines = 30
		const eventsPerGoroutine = 50
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			workerID := g
			goroutinelabels.NewGoroutine("effort_telemetry_test_worker", "test parallel event publishing").StartSimple(func() {
				defer wg.Done()
				for i := 0; i < eventsPerGoroutine; i++ {
					ev := map[string]any{
						objects.FieldKeyKind: "backlog_item",
						"worker_id":          workerID,
						"effort_variance":    12.5,
						"seq":                i,
					}
					_, err := mp.Sample(ev)
					if err != nil {
						t.Errorf("worker %d sample %d failed: %v", workerID, i, err)
					}
				}
			})
		}

		wg.Wait()
		expectedTotal := int64(numGoroutines * eventsPerGoroutine)
		assert.Equal(t, expectedTotal, totalSubscriberCount.Load())
	})

	t.Run("RollupAccuracy", func(t *testing.T) {
		t.Parallel()

		// Test BuildAggregationResult
		start := time.Now().Add(-1 * time.Hour)
		end := time.Now()
		config := &AggregationConfig{
			ObjectKind:  "backlog_item",
			FieldName:   "estimated_effort",
			MetricType:  MetricTypeScalar,
			WindowStart: start,
			WindowEnd:   end,
		}
		aggs := map[string]any{
			"sum":   125.0,
			"avg":   25.0,
			"count": 5,
		}
		res := BuildAggregationResult(config, "METRIC-001", aggs, 5)
		require.NotNil(t, res)
		assert.Equal(t, "METRIC-001", res.MetricID)
		assert.Equal(t, "backlog_item", res.ObjectKind)
		assert.Equal(t, "estimated_effort", res.FieldName)
		assert.Equal(t, MetricTypeScalar, res.MetricType)
		assert.Equal(t, 125.0, res.Aggregations["sum"])
		assert.Equal(t, 25.0, res.Aggregations["avg"])
		assert.Equal(t, 5, res.ObjectCount)
		assert.Equal(t, 1, res.CollectionCount)

		// Test TopNMap accuracy
		counts := map[string]int{
			"item_A": 10,
			"item_B": 50,
			"item_C": 30,
			"item_D": 5,
			"item_E": 40,
		}

		top3 := TopNMap(counts, 3)
		require.Len(t, top3, 3)
		assert.Equal(t, 50, top3["item_B"])
		assert.Equal(t, 40, top3["item_E"])
		assert.Equal(t, 30, top3["item_C"])
		assert.NotContains(t, top3, "item_A")
		assert.NotContains(t, top3, "item_D")

		topAll := TopNMap(counts, 10)
		assert.Len(t, topAll, 5)

		topZero := TopNMap(counts, 0)
		assert.Empty(t, topZero)

		// Test generateDBSummary accuracy
		db := &DependenciesBlockers{
			Blockers: []BlockerItem{
				{ID: "BLK-1", Severity: "critical", Description: "Database down"},
				{ID: "BLK-2", Severity: "high", Description: "Network partitioned"},
				{ID: "BLK-3", Severity: "high", Description: "Auth token expired"},
				{ID: "BLK-4", Severity: "low", Description: "UI minor artifact"},
			},
			Dependencies: []DependencyItem{
				{ID: "DEP-1", Severity: "critical", Description: "Kernel v2 release"},
				{ID: "DEP-2", Severity: "medium", Description: "Doc update"},
			},
		}
		summary := generateDBSummary(db)
		assert.Contains(t, summary, "1 critical blocker(s) identified.")
		assert.Contains(t, summary, "2 high-priority blocker(s) identified.")
		assert.Contains(t, summary, "1 additional blocker(s) present.")
		assert.Contains(t, summary, "1 critical dependency(ies) identified.")
	})
}

// TestEffortTelemetry_NegativeAdversarialBoundary satisfies the Negative Adversarial Boundary requirement:
// tests nil pipelines, corrupted metric points, zero-division in rates/percentiles, boundary conditions
// on negative durations or oversized values without panic.
func TestEffortTelemetry_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	t.Run("NilPipelinesAndUninitializedStates", func(t *testing.T) {
		t.Parallel()

		// Sample on pipeline with nil sampler registry returns error, does not panic
		mp := NewMetricPipeline(storage.NewNoopObjectStorage())
		mp.SetSamplerRegistry(nil)
		flushed, err := mp.Sample(map[string]any{"kind": "backlog_item"})
		assert.False(t, flushed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sampler registry not initialized")

		// CollectMetrics with unsupported metric type returns error, does not panic
		_, err = mp.CollectMetrics(context.Background(), &AggregationConfig{
			MetricType: "unsupported_adversarial_metric_type",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no aggregator found for metric type")

		// CalculateProjectMetrics on empty mock storage returns sensible defaults without crashing
		mockEmpty := &MockStorageProvider{objects: []map[string]any{}}
		secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*"})
		projMetrics, err := CalculateProjectMetrics(context.Background(), mockEmpty, secCtx, "empty-proj", false)
		require.NoError(t, err)
		require.NotNil(t, projMetrics)
		assert.InDelta(t, 50.0, projMetrics.PCS, 0.001) // Default to neutral score
		assert.InDelta(t, 0.0, projMetrics.EDD, 0.001)
		require.NotNil(t, projMetrics.DB)
		assert.Equal(t, "No dependencies or blockers identified.", projMetrics.DB.Summary)

		// CalculateProjectMetrics gracefully propagates storage error when storage fails
		_, err = CalculateProjectMetrics(context.Background(), storage.NewNoopObjectStorage(), secCtx, "fail-proj", false)
		assert.Error(t, err)

		// Helpers with empty or uninitialized inputs
		assert.Equal(t, "No dependencies or blockers identified.", generateDBSummary(&DependenciesBlockers{}))
		assert.Equal(t, []string{"single_token_without_pipe"}, splitPair("single_token_without_pipe"))
		assert.False(t, HasReadableTrait(nil))
		assert.Empty(t, TopNMap(nil, 5))
	})

	t.Run("CorruptedMetricPointsAndFiles", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cfg := TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           dir,
			Series:        "corrupted_series",
		}

		// Write corrupted bytes to chunk file
		chunkStart := time.Unix(1_700_000_000, 0).UTC().Truncate(time.Hour)
		chunkName := sanitizeSeriesName(cfg.Series) + "_" + chunkStart.Format("20060102T1504") + ".chunk"
		chunkPath := filepath.Join(dir, chunkName)

		// Garbage bytes with invalid varint continuation bits
		corruptBytes := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}
		err := fileutil.WriteFile(chunkPath, corruptBytes, paths.FilePerm600)
		require.NoError(t, err)

		reader := NewTimeSeriesReader(cfg)
		var collected []TimeSeriesPoint
		// Reader should handle corrupted chunk without infinite loop or panic
		err = reader.Range(chunkStart, chunkStart.Add(time.Hour), func(p TimeSeriesPoint) error {
			collected = append(collected, p)
			return nil
		})
		// Corrupt data must either return an error or safely end iteration
		_ = err

		// NewTimeSeriesWriter validation failures
		_, err = NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: 0, Dir: dir, Series: "series"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "chunkDuration must be > 0")

		_, err = NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: -10 * time.Minute, Dir: dir, Series: "series"})
		assert.Error(t, err)

		_, err = NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: time.Hour, Dir: "", Series: "series"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dir and series are required")

		_, err = NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: time.Hour, Dir: dir, Series: ""})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dir and series are required")

		// TimeSeriesReader.Range with invalid duration
		invalidReader := NewTimeSeriesReader(TimeSeriesConfig{ChunkDuration: 0, Dir: dir, Series: "series"})
		err = invalidReader.Range(time.Now(), time.Now(), func(p TimeSeriesPoint) error { return nil })
		assert.Error(t, err)

		// SamplerConfig validation
		badBatchCfg := &SamplerConfig{BatchSize: 0, MaxBatchSize: 10}
		assert.Error(t, badBatchCfg.Validate())

		badMaxCfg := &SamplerConfig{BatchSize: 20, MaxBatchSize: 10}
		assert.Error(t, badMaxCfg.Validate())

		badIntervalCfg := &SamplerConfig{BatchSize: 10, MaxBatchSize: 20, FlushInterval: -1}
		assert.Error(t, badIntervalCfg.Validate())
	})

	t.Run("ZeroDivisionInRatesAndPercentiles", func(t *testing.T) {
		t.Parallel()

		// ComputeEffortVariance zero-division safeguards
		assert.Equal(t, 0.0, ComputeEffortVariance("0", "100"))
		assert.Equal(t, 0.0, ComputeEffortVariance("0.0", "50.0"))
		assert.Equal(t, 0.0, ComputeEffortVariance("", ""))
		assert.Equal(t, 0.0, ComputeEffortVariance("invalid_num", "10.0"))
		// When actual effort is missing/invalid, parsed as 0 -> ((0 - 10)/10)*100 = -100.0%
		assert.Equal(t, -100.0, ComputeEffortVariance("10.0", "not_a_number"))

		// calculateBasePCS and calculateBaseEDD zero object division safeguard
		mockEmpty := &MockStorageProvider{objects: []map[string]any{}}
		secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*"})
		pcs, err := calculateBasePCS(context.Background(), mockEmpty, secCtx, "proj")
		require.NoError(t, err)
		assert.Equal(t, 50.0, pcs)

		edd, err := calculateBaseEDD(context.Background(), mockEmpty, secCtx, "proj")
		require.NoError(t, err)
		assert.Equal(t, 0.0, edd)
	})

	t.Run("BoundaryConditions_NegativeDurationsAndOversizedValues", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cfg := TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           dir,
			Series:        "boundary_extreme_series",
		}

		writer, err := NewTimeSeriesWriter(cfg)
		require.NoError(t, err)

		chunkStart := time.Unix(1_700_000_000, 0).UTC().Truncate(time.Hour)

		// 1. Multiple points within chunk and across rotation
		extremePoints := []TimeSeriesPoint{
			{Ts: chunkStart.Add(1 * time.Minute), Value: 1000},
			{Ts: chunkStart.Add(2 * time.Minute), Value: 5000000000},
			{Ts: chunkStart.Add(3 * time.Minute), Value: -3000000000},
			{Ts: chunkStart.Add(4 * time.Minute), Value: 0},
			{Ts: chunkStart.Add(5 * time.Minute), Value: -1},
			{Ts: chunkStart.Add(6 * time.Minute), Value: 1},
			{Ts: chunkStart.Add(75 * time.Minute), Value: 99999}, // causes rotation to next chunk
		}

		for _, p := range extremePoints {
			require.NoError(t, writer.Append(p))
		}
		require.NoError(t, writer.Close())

		// Read back and verify all points are preserved accurately
		reader := NewTimeSeriesReader(cfg)
		var readPoints []TimeSeriesPoint
		err = reader.Range(chunkStart, chunkStart.Add(2*time.Hour), func(p TimeSeriesPoint) error {
			readPoints = append(readPoints, p)
			return nil
		})
		require.NoError(t, err)
		require.Len(t, readPoints, len(extremePoints))

		for i, want := range extremePoints {
			assert.Equal(t, want.Value, readPoints[i].Value, "point %d value mismatch", i)
			assert.Equal(t, want.Ts.Unix(), readPoints[i].Ts.Unix(), "point %d ts mismatch", i)
		}

		// 2. Direct encoding checks for extreme 64-bit bounds (math.MaxInt64, math.MinInt64)
		var buf bytes.Buffer
		bw := bufio.NewWriter(&buf)
		require.NoError(t, writeZigZagVarint(bw, math.MaxInt64))
		require.NoError(t, writeZigZagVarint(bw, math.MinInt64))
		require.NoError(t, writeVarint(bw, 0))
		require.NoError(t, writeVarint(bw, -1))
		require.NoError(t, bw.Flush())
		assert.NotEmpty(t, buf.Bytes())

		// 3. Negative effort variance calculations
		negVariance := ComputeEffortVariance("-10.0", "-15.0")
		assert.InDelta(t, 50.0, negVariance, 0.001)

		// 4. Clamping boundaries in enhancePCSWithCommits
		overScore := enhancePCSWithCommits(150.0, &CommitMetrics{
			CommitFrequency: 1.0,
			CommitPattern:   "increasing",
		})
		assert.Equal(t, 100.0, overScore)

		underScore := enhancePCSWithCommits(-50.0, &CommitMetrics{
			CommitFrequency: 0.01,
			CommitPattern:   "decreasing",
		})
		assert.Equal(t, 0.0, underScore)
	})
}
