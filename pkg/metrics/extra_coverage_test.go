package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestResourceAvailability_Get(t *testing.T) {
	res := GetResourceAvailability()
	require.NotNil(t, res)
	assert.Greater(t, res.TotalGoroutines, 0)
	assert.Greater(t, res.UsedGoroutines, 0)
}

func TestStatusHistoryAggregator_GetMetricType(t *testing.T) {
	agg := NewStatusHistoryMetricAggregator(storage.NewNoopObjectStorage())
	assert.Equal(t, MetricTypeStatusHistory, agg.GetMetricType())
	transitions := map[string]int{
		"a->b": 10,
		"b->c": 5,
		"c->d": 1,
	}
	top := getTopTransitions(transitions, 2)
	assert.Len(t, top, 2)
	assert.Equal(t, 10, top["a->b"])
	assert.Equal(t, 5, top["b->c"])
}

func TestMetricPipeline_SubscribersAndTraits(t *testing.T) {
	mp := NewMetricPipeline(storage.NewNoopObjectStorage())
	var called bool
	mp.AddSubscriber(func(ev map[string]any) {
		called = true
	})

	// Sample with nil samplerRegistry
	mp.SetSamplerRegistry(nil)
	_, err := mp.Sample(map[string]any{"kind": "test"})
	assert.Error(t, err)

	// getTraitRegistry
	tr := mp.getTraitRegistry()
	assert.NotNil(t, tr)

	// Add samplerRegistry and sample
	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() { _ = reg.StopAll() })
	mp.SetSamplerRegistry(reg)
	assert.Equal(t, reg, mp.GetSamplerRegistry())

	_, err = mp.Sample(map[string]any{"kind": "backlog_item"})
	assert.NoError(t, err)
	assert.True(t, called)

	// HasReadableTrait
	assert.False(t, HasReadableTrait(nil))
	assert.False(t, HasReadableTrait(map[string]any{"traits": []any{"other"}}))
	assert.True(t, HasReadableTrait(map[string]any{"traits": []any{"readable"}}))

	// getDefaultAggregator
	assert.NotNil(t, mp.getDefaultAggregator(MetricTypeScalar))
	assert.NotNil(t, mp.getDefaultAggregator(MetricTypeList))
	assert.NotNil(t, mp.getDefaultAggregator(MetricTypeStatusHistory))
	assert.Nil(t, mp.getDefaultAggregator("unsupported_type"))

	// RegisterAggregator
	customAgg := NewStatusHistoryMetricAggregator(storage.NewNoopObjectStorage())
	mp.RegisterAggregator(customAgg)
	assert.Equal(t, customAgg, mp.aggregators[MetricTypeStatusHistory])

	// CollectMetrics without aggregator found
	_, err = mp.CollectMetrics(context.Background(), &AggregationConfig{
		MetricType: "unsupported_type",
	})
	assert.ErrorContains(t, err, "no aggregator found")
}

func TestAsyncValidationBaseMetric_BuildAndPersist(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	// nil checks
	_, err := BuildAsyncValidationBaseMetricInstance(nil, "op-1", "BM-1", []byte("{}"))
	assert.Error(t, err)
	vm := validation.NewValidationMetrics()
	_, err = BuildAsyncValidationBaseMetricInstance(vm, "op-1", "", []byte("{}"))
	assert.Error(t, err)

	// success build
	inst, err := BuildAsyncValidationBaseMetricInstance(vm, "op-1", "BM-1", []byte("{\"valid\":true}"))
	assert.NoError(t, err)
	assert.NotNil(t, inst)

	// persist async
	store := storage.NewNoopObjectStorage()
	PersistAsyncValidationMetricsToStorageAsync(store, vm, "op-1", nil)
}

func TestCacheValidation_BuildAndFlush(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	now := time.Now().UTC()
	meta := storage.CacheMetricsEncodeMeta{
		WindowStart:           now.Add(-time.Hour),
		WindowEnd:             now,
		AsyncStrategyKinds:    2,
		KindValidationMetrics: 3,
		TotalValidationsSum:   100,
	}

	inst, err := BuildCacheValidationBaseMetricInstance("BAS-test-1", []byte("{\"test\":true}"), meta)
	assert.NoError(t, err)
	assert.NotNil(t, inst)

	// Long payload to trigger truncation
	bigPayload := make([]byte, 300*1024)
	for i := range bigPayload {
		bigPayload[i] = 'a'
	}
	instBig, err := BuildCacheValidationBaseMetricInstance("BAS-test-big", bigPayload, meta)
	assert.NoError(t, err)
	assert.NotNil(t, instBig)

	store := storage.NewNoopObjectStorage()
	PersistCacheValidationMetricsJSONAsync(nil, []byte("{}"), nil, meta)
	PersistCacheValidationMetricsJSONAsync(store, []byte("{}"), nil, meta)
	FlushCacheValidationMetricsToStorage(store, nil)
}

func TestGraphProvider_BuildAndFlush(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	now := time.Now().UTC()
	meta := GraphProviderPersistMeta{
		WindowStart:     now.Add(-time.Hour),
		WindowEnd:       now,
		OperationKinds:  4,
		QueryKinds:      2,
		TotalOperations: 50,
	}

	inst, err := BuildGraphProviderMetricsBaseMetricInstance("BAS-graph-1", []byte("{\"ops\":[]}"), meta)
	assert.NoError(t, err)
	assert.NotNil(t, inst)

	// Long payload to trigger truncation
	bigPayload := make([]byte, 300*1024)
	for i := range bigPayload {
		bigPayload[i] = 'x'
	}
	instBig, err := BuildGraphProviderMetricsBaseMetricInstance("BAS-graph-big", bigPayload, meta)
	assert.NoError(t, err)
	assert.NotNil(t, instBig)

	store := storage.NewNoopObjectStorage()
	PersistGraphProviderMetricsJSONAsync(nil, []byte("{}"), nil, meta)
	PersistGraphProviderMetricsJSONAsync(store, []byte("{}"), nil, meta)
	FlushGraphProviderMetricsToStorage(store, nil)
}

func TestOrderedListAggregator_Sequences(t *testing.T) {
	assert.Equal(t, 0.0, averageSequenceLength(nil))
	seqs := [][]string{
		{"a", "b", "c"},
		{"a", "b"},
	}
	assert.Equal(t, 2.5, averageSequenceLength(seqs))

	allSeqs := [][]string{
		{"a", "b", "c"},
		{"a", "b", "d"},
		{"a", "b", "c"},
	}
	common := findCommonSequences(allSeqs, 2)
	assert.NotEmpty(t, common)
	assert.Equal(t, 3, common["[a b]"])
}

func TestSampler_GetBatchKey(t *testing.T) {
	s := &Sampler{
		config: &SamplerConfig{
			GroupByObjectID: true,
		},
	}
	assert.Equal(t, "obj-123", s.getBatchKey(map[string]any{"object_id": "obj-123"}))
	assert.Equal(t, "tgt-456", s.getBatchKey(map[string]any{"target_id": "tgt-456"}))
	assert.Equal(t, "backlog_item", s.getBatchKey(map[string]any{"kind": "backlog_item"}))
	assert.Equal(t, "global", s.getBatchKey(map[string]any{}))

	sNoGroup := &Sampler{
		config: &SamplerConfig{
			GroupByObjectID: false,
		},
	}
	assert.Equal(t, "global", sNoGroup.getBatchKey(map[string]any{"object_id": "obj-123"}))
}

func TestApplySamplerProfile(t *testing.T) {
	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() { _ = reg.StopAll() })

	cfgFile := &SamplerConfigFile{
		OptOut: []string{"opt_out_kind"},
		OptIn:  []string{"included_kind"},
	}
	cfgFile.Defaults.Enabled = true
	cfgFile.Defaults.BatchSize = 10
	cfgFile.Defaults.MaxBatchSize = 20
	cfgFile.Defaults.GroupByObjectID = true

	// 1. Empty applies_to and not default
	p1 := &SamplerProfile{
		Name:         "p1",
		ResolvedSpec: map[string]any{},
	}
	err := applySamplerProfile(reg, p1, cfgFile, 5*time.Second)
	assert.NoError(t, err)

	// 2. Default profile
	pDefault := &SamplerProfile{
		Name: "pDefault",
		ResolvedSpec: map[string]any{
			objects.FieldKeyIsDefault: true,
		},
	}
	err = applySamplerProfile(reg, pDefault, cfgFile, 5*time.Second)
	assert.NoError(t, err)

	// 3. Profile with invalid flush_interval
	pBadInterval := &SamplerProfile{
		Name: "pBad",
		ResolvedSpec: map[string]any{
			objects.FieldKeyFlushInterval: "invalid-duration",
		},
	}
	err = applySamplerProfile(reg, pBadInterval, cfgFile, 5*time.Second)
	assert.Error(t, err)

	// 4. Complete profile with applies_to (covering opt-in and opt-out)
	pComplete := &SamplerProfile{
		Name: "pComplete",
		ResolvedSpec: map[string]any{
			objects.FieldKeyFlushInterval:   "10s",
			objects.FieldKeyBatchSize:       5,
			objects.FieldKeyMaxBatchSize:    15,
			objects.FieldKeyEnabled:         true,
			objects.FieldKeyGroupByObjectID: false,
			objects.FieldKeyMetricType:      MetricTypeScalar,
			objects.FieldKeyAppliesTo:       []any{"opt_out_kind", "skipped_not_in_optin", "included_kind"},
		},
	}
	err = applySamplerProfile(reg, pComplete, cfgFile, 5*time.Second)
	assert.NoError(t, err)
}

func TestSamplerRegistry_SystemFallback(t *testing.T) {
	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() { _ = reg.StopAll() })

	assert.True(t, registrySamplerIsSystemKind("", MetricTypeSystem))
	assert.False(t, registrySamplerIsSystemKind("field", MetricTypeScalar))

	// Fill to maxSamplers
	for i := 0; i < maxSamplers; i++ {
		kind := fmt.Sprintf("kind_%d", i)
		cfg := &SamplerConfig{
			ObjectKind: kind,
			FieldName:  "f",
			MetricType: MetricTypeScalar,
		}
		_ = reg.RegisterSampler(cfg)
	}

	// Now GetOrCreateSampler for system metric when atCap -> uses fallback!
	s, err := reg.GetOrCreateSampler("sys_kind", "", MetricTypeSystem)
	assert.NoError(t, err)
	assert.NotNil(t, s)
}

type mockPipelineAgg struct {
	mType string
}

func (m *mockPipelineAgg) GetMetricType() string { return m.mType }
func (m *mockPipelineAgg) Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
	return &AggregationResult{MetricType: m.mType}, nil
}

func TestMetricPipeline_DiscoverAndSampling(t *testing.T) {
	store := storage.NewNoopObjectStorage()
	mp := NewMetricPipeline(store)
	_ = mp.getTraitRegistry()

	// Register mock aggregator so CollectMetrics doesn't touch store.List
	mock := &mockPipelineAgg{mType: MetricTypeScalar}
	mp.RegisterAggregator(mock)

	// CollectMetrics with base == nil
	res, err := mp.CollectMetrics(nil, &AggregationConfig{
		MetricType: MetricTypeScalar,
		ObjectKind: "backlog_item",
		FieldName:  "estimated_effort",
	})
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Configure valid sampler in registry
	reg := NewSamplerRegistry(store)
	t.Cleanup(func() { _ = reg.StopAll() })
	cfg := DefaultSamplerConfig()
	cfg.ObjectKind = "backlog_item"
	cfg.FieldName = "estimated_effort"
	cfg.MetricType = MetricTypeScalar
	cfg.Enabled = true
	err = reg.RegisterSampler(cfg)
	require.NoError(t, err)

	mp.SetSamplerRegistry(reg)

	_, err = mp.CollectMetrics(context.Background(), &AggregationConfig{
		MetricType: MetricTypeScalar,
		ObjectKind: "backlog_item",
		FieldName:  "estimated_effort",
	})
	assert.ErrorContains(t, err, "sampling is enabled")
}

type mockStorageForAgg struct {
	storage.ObjectStorageProvider
	objs []map[string]any
}

func (m *mockStorageForAgg) List(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: m.objs}, nil
}

func TestScalarAggregator_DefaultAndExplicitAggregations(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStorageForAgg{
		objs: []map[string]any{
			{"val": 10.0, objects.FieldKeyCreatedAt: now.Format(time.RFC3339)},
			{"val": 20.0, objects.FieldKeyCreatedAt: now.Format(time.RFC3339)},
			{"val": 30.0, objects.FieldKeyCreatedAt: now.Format(time.RFC3339)},
		},
	}
	agg := NewScalarMetricAggregator(store)

	// 1. Explicit aggregations: sum, avg, min, max, count
	cfgExplicit := &AggregationConfig{
		ObjectKind:   "test",
		FieldName:    "val",
		Aggregations: []string{"sum", "avg", "min", "max", "count"},
		WindowStart:  now.Add(-time.Hour),
		WindowEnd:    now.Add(time.Hour),
	}
	resExp, err := agg.Aggregate(context.Background(), cfgExplicit)
	require.NoError(t, err)
	assert.Equal(t, 60.0, resExp.Aggregations["sum"])
	assert.Equal(t, 20.0, resExp.Aggregations["avg"])
	assert.Equal(t, 10.0, resExp.Aggregations["min"])
	assert.Equal(t, 30.0, resExp.Aggregations["max"])
	assert.Equal(t, 3, resExp.Aggregations["count"])

	// 2. Default aggregations (Aggregations == nil)
	cfgDefault := &AggregationConfig{
		ObjectKind:   "test",
		FieldName:    "val",
		Aggregations: nil,
		WindowStart:  now.Add(-time.Hour),
		WindowEnd:    now.Add(time.Hour),
	}
	resDef, err := agg.Aggregate(context.Background(), cfgDefault)
	require.NoError(t, err)
	assert.Equal(t, 60.0, resDef.Aggregations["sum"])
	assert.Equal(t, 3, resDef.Aggregations["count"])
}

func TestStatusHistoryAggregator_DefaultAndExplicitAggregations(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStorageForAgg{
		objs: []map[string]any{
			{
				"status":                  []any{"open", "in_progress", "done"},
				objects.FieldKeyCreatedAt: now.Format(time.RFC3339),
			},
		},
	}
	agg := NewStatusHistoryMetricAggregator(store)

	// 1. Explicit aggregations
	cfgExplicit := &AggregationConfig{
		ObjectKind:   "task",
		FieldName:    "status",
		Aggregations: []string{"state_distribution", "state_durations", "transitions", "common_transitions", "avg_history_length"},
		WindowStart:  now.Add(-time.Hour),
		WindowEnd:    now.Add(time.Hour),
	}
	resExp, err := agg.Aggregate(context.Background(), cfgExplicit)
	require.NoError(t, err)
	assert.NotNil(t, resExp.Aggregations["state_distribution"])
	assert.NotNil(t, resExp.Aggregations["state_durations"])
	assert.NotNil(t, resExp.Aggregations[objects.FieldKeyTransitions])
	assert.NotNil(t, resExp.Aggregations["common_transitions"])
	assert.NotNil(t, resExp.Aggregations["avg_history_length"])

	// 2. Default aggregations
	cfgDefault := &AggregationConfig{
		ObjectKind:   "task",
		FieldName:    "status",
		Aggregations: nil,
		WindowStart:  now.Add(-time.Hour),
		WindowEnd:    now.Add(time.Hour),
	}
	resDef, err := agg.Aggregate(context.Background(), cfgDefault)
	require.NoError(t, err)
	assert.NotNil(t, resDef.Aggregations["state_distribution"])
	assert.NotNil(t, resDef.Aggregations[objects.FieldKeyTransitions])
	assert.NotNil(t, resDef.Aggregations["avg_history_length"])
}

func TestSampler_AggregateEventsAndCounts(t *testing.T) {
	s := &Sampler{
		batches: map[string]*SamplerBatch{
			"b1": {
				Events: []map[string]any{
					{objects.FieldKeyEventType: "audit", objects.FieldKeySeverity: "info"},
					{objects.FieldKeyEventType: "audit", objects.FieldKeySeverity: "warn"},
				},
			},
		},
	}
	assert.Equal(t, 1, s.GetBatchCount())
	assert.Equal(t, 2, s.GetTotalPendingEvents())

	res, err := s.aggregateEvents(s.batches["b1"].Events, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, res[aggregationCountMetric])
	assert.Equal(t, map[string]int{"audit": 2}, res[objects.FieldKeyEventTypeCounts])
	assert.Equal(t, map[string]int{"info": 1, "warn": 1}, res["severity_counts"])
}

func TestMetricPipeline_DetectMetricType(t *testing.T) {
	mp := &MetricPipeline{}
	assert.Equal(t, MetricTypeStatusHistory, mp.detectMetricType([]string{MetricTypeStatusHistory}))
	assert.Equal(t, MetricTypeOrderedList, mp.detectMetricType([]string{MetricTypeOrderedList}))
	assert.Equal(t, MetricTypeList, mp.detectMetricType([]string{MetricTypeList}))
	assert.Equal(t, MetricTypeScalar, mp.detectMetricType([]string{MetricTypeScalar}))
	assert.Equal(t, "", mp.detectMetricType([]string{"unknown_trait"}))
}

func TestSamplerRegistry_UnregisterAndFlushAll(t *testing.T) {
	sr := NewSamplerRegistry(storage.NewNoopObjectStorage())
	cfg := DefaultSamplerConfig()
	cfg.ObjectKind = "task"
	cfg.FieldName = "effort"
	cfg.MetricType = MetricTypeScalar

	err := sr.RegisterSampler(cfg)
	require.NoError(t, err)

	// FlushAll with active sampler
	err = sr.FlushAll()
	require.NoError(t, err)

	// Unregister sampler
	err = sr.UnregisterSampler("task", "effort", MetricTypeScalar)
	require.NoError(t, err)

	// Unregister non-existent sampler returns nil
	err = sr.UnregisterSampler("nonexistent", "field", MetricTypeScalar)
	require.NoError(t, err)
}
