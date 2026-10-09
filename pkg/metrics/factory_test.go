package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockMetricStorage struct {
	storage.ObjectStorageProvider
	created []map[string]any
	fail    bool
}

func (m *mockMetricStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if m.fail {
		return errfmt.Errorf("simulated create error")
	}
	if _, ok := obj[objects.FieldKeyID]; !ok {
		obj[objects.FieldKeyID] = "MET-MOCK-ID-123"
	}
	m.created = append(m.created, obj)
	return nil
}

func TestMetricFactory_CreateMetric_DisabledKind(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)
	metricsrecording.DenyKind("disabled_metric_kind")
	t.Cleanup(func() {
		metricsrecording.AllowKind("disabled_metric_kind")
	})

	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	id, err := factory.CreateMetric(context.Background(), &MetricCreationRequest{
		Kind: "disabled_metric_kind",
	})
	require.NoError(t, err)
	assert.Empty(t, id)
}

func TestMetricFactory_CreateMetric_UnsupportedBuilder(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	id, err := factory.CreateMetric(context.Background(), &MetricCreationRequest{
		Kind: "",
	})
	assert.Error(t, err)
	assert.Empty(t, id)
}

func TestMetricFactory_CreateMetric_SuccessAndFailure(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	now := time.Now().UTC()
	req := &MetricCreationRequest{
		Kind:        objects.KindCommandMetric,
		Title:       "Test Command Metric",
		MetricType:  MetricTypeSystem,
		Source:      MetricSourcePipeline,
		Tags:        []string{"test", "factory"},
		WindowStart: now.Add(-time.Hour),
		WindowEnd:   now,
		EventCount:  10,
		Fields: map[string]any{
			"command_name": "test-command",
		},
	}

	// 1. Success
	id, err := factory.CreateMetric(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	require.Len(t, mockStore.created, 1)
	assert.Equal(t, "Test Command Metric", mockStore.created[0][objects.FieldKeyTitle])

	// 2. Storage failure
	mockStore.fail = true
	_, err = factory.CreateMetric(context.Background(), req)
	assert.Error(t, err)
}

func TestMetricFactory_CreateMetricAsync(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	now := time.Now().UTC()
	req := &MetricCreationRequest{
		Kind:        objects.KindCommandMetric,
		Title:       "Async Command Metric",
		MetricType:  MetricTypePerformance,
		Source:      MetricSourcePipeline,
		WindowStart: now.Add(-time.Minute),
		WindowEnd:   now,
		EventCount:  1,
	}

	// 1. Async with callback
	var wg sync.WaitGroup
	wg.Add(1)
	factory.CreateMetricAsync(context.Background(), req, func(metricID string, err error) {
		defer wg.Done()
		assert.NoError(t, err)
		assert.NotEmpty(t, metricID)
	})
	wg.Wait()

	// 2. Async without callback (success path)
	factory.CreateMetricAsync(context.Background(), req, nil)
	time.Sleep(50 * time.Millisecond)

	// 3. Async without callback with failure
	failStore := &mockMetricStorage{fail: true}
	failFactory := NewMetricFactory(failStore)
	failFactory.CreateMetricAsync(context.Background(), req, nil)
	time.Sleep(50 * time.Millisecond)

	// 4. Async with disabled kind
	metricsrecording.DenyKind("disabled_async_kind")
	t.Cleanup(func() {
		metricsrecording.AllowKind("disabled_async_kind")
	})
	disabledReq := &MetricCreationRequest{Kind: "disabled_async_kind"}
	var wg2 sync.WaitGroup
	wg2.Add(1)
	factory.CreateMetricAsync(context.Background(), disabledReq, func(metricID string, err error) {
		defer wg2.Done()
		assert.NoError(t, err)
		assert.Empty(t, metricID)
	})
	wg2.Wait()
}

func TestMetricFactory_CreateMetricFromConfig_And_Async(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	cfg := &AggregationConfig{
		ObjectKind: objects.KindCommandMetric,
		MetricType: MetricTypePerformance,
	}

	// 1. Synchronous CreateMetricFromConfig
	id, err := factory.CreateMetricFromConfig(context.Background(), cfg, "Config Metric", 5, nil, map[string]any{
		"extra_field": "val1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// 2. Async with callback
	var wg sync.WaitGroup
	wg.Add(1)
	factory.CreateMetricFromConfigAsync(context.Background(), cfg, "Config Metric Async", 3, nil, nil, func(metricID string, err error) {
		defer wg.Done()
		assert.NoError(t, err)
		assert.NotEmpty(t, metricID)
	})
	wg.Wait()

	// 3. Async without callback with failure
	failStore := &mockMetricStorage{fail: true}
	failFactory := NewMetricFactory(failStore)
	failFactory.CreateMetricFromConfigAsync(context.Background(), cfg, "Config Metric Async Fail", 1, nil, nil, nil)
	time.Sleep(50 * time.Millisecond)

	// 4. Disabled kind skips cleanly
	metricsrecording.DenyKind("disabled_cfg_kind")
	t.Cleanup(func() {
		metricsrecording.AllowKind("disabled_cfg_kind")
	})
	disabledCfg := &AggregationConfig{ObjectKind: "disabled_cfg_kind"}
	idDisabled, errDisabled := factory.CreateMetricFromConfig(context.Background(), disabledCfg, "Disabled", 1, nil, nil)
	require.NoError(t, errDisabled)
	assert.Empty(t, idDisabled)

	var wg2 sync.WaitGroup
	wg2.Add(1)
	factory.CreateMetricFromConfigAsync(context.Background(), disabledCfg, "Disabled Async", 1, nil, nil, func(metricID string, err error) {
		defer wg2.Done()
		assert.NoError(t, err)
		assert.Empty(t, metricID)
	})
	wg2.Wait()
}

func TestMetricFactory_CreateMetricFromInstanceAsync(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	inst := map[string]any{
		objects.FieldKeyID:   "MET-INST-001",
		objects.FieldKeyKind: objects.KindCommandMetric,
	}

	// 1. Success with callback
	var wg sync.WaitGroup
	wg.Add(1)
	factory.CreateMetricFromInstanceAsync(context.Background(), inst, func(metricID string, err error) {
		defer wg.Done()
		assert.NoError(t, err)
		assert.Equal(t, "MET-INST-001", metricID)
	})
	wg.Wait()

	// 2. Missing ID in instance returns error
	noIDStore := &mockMetricStorage{fail: false}
	// override Create to not inject ID
	noIDFactory := NewMetricFactory(&noIDStorage{})
	noIDInst := map[string]any{
		objects.FieldKeyKind: objects.KindCommandMetric,
	}
	var wg2 sync.WaitGroup
	wg2.Add(1)
	noIDFactory.CreateMetricFromInstanceAsync(context.Background(), noIDInst, func(metricID string, err error) {
		defer wg2.Done()
		assert.Error(t, err)
		assert.Empty(t, metricID)
	})
	wg2.Wait()

	_ = noIDStore

	// 3. Failure without callback logs warning
	failStore := &mockMetricStorage{fail: true}
	failFactory := NewMetricFactory(failStore)
	failFactory.CreateMetricFromInstanceAsync(context.Background(), inst, nil)
	time.Sleep(50 * time.Millisecond)

	// 4. Disabled kind
	metricsrecording.DenyKind("disabled_inst_kind")
	t.Cleanup(func() {
		metricsrecording.AllowKind("disabled_inst_kind")
	})
	disabledInst := map[string]any{
		objects.FieldKeyKind: "disabled_inst_kind",
	}
	var wg3 sync.WaitGroup
	wg3.Add(1)
	factory.CreateMetricFromInstanceAsync(context.Background(), disabledInst, func(metricID string, err error) {
		defer wg3.Done()
		assert.NoError(t, err)
		assert.Empty(t, metricID)
	})
	wg3.Wait()
}

type noIDStorage struct {
	storage.ObjectStorageProvider
}

func (n *noIDStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func TestMetricFactory_SetCommonFields_And_BuildInstance_NilEdgeCases(t *testing.T) {
	t.Parallel()
	mockStore := &mockMetricStorage{}
	factory := NewMetricFactory(mockStore)

	// nil builder and req handling
	factory.setCommonFields(nil, nil)
	factory.setCommonFields(nil, &MetricCreationRequest{})

	// buildInstance nil builder returns error
	_, err := factory.buildInstance(nil)
	assert.Error(t, err)

	// createBuilder empty kind returns nil
	assert.Nil(t, factory.createBuilder("", "1.0.0"))
}

// Suppress unused imports
var _ = koi.ID
