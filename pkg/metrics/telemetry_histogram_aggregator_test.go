package metrics

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// mockStorageProviderWithCreate extends MockStorageProvider to support Create calls
type mockStorageProviderWithCreate struct {
	storage.ObjectStorageProvider
	objects []map[string]any
	created []map[string]any
	err     error
}

func (m *mockStorageProviderWithCreate) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &storage.QueryResult{Objects: m.objects}, nil
}

func (m *mockStorageProviderWithCreate) Create(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error {
	if m.err != nil {
		return m.err
	}
	if obj != nil {
		obj[objects.FieldKeyID] = "MET-12345"
	}
	m.created = append(m.created, obj)
	return nil
}

func TestTelemetryHistogram_BucketingAndEdgeCases(t *testing.T) {
	// Zero/empty bucket configurations
	hNil := NewPrometheusHistogram(nil)
	require.NotNil(t, hNil)
	hNil.Observe(42.0)
	sum, count := hNil.GetStats()
	assert.Equal(t, 42.0, sum)
	assert.Equal(t, uint64(1), count)
	bounds, counts := hNil.GetBuckets()
	assert.Empty(t, bounds)
	assert.Empty(t, counts)

	hEmpty := NewPrometheusHistogram([]float64{})
	require.NotNil(t, hEmpty)
	hEmpty.Observe(10.5)
	sum, count = hEmpty.GetStats()
	assert.Equal(t, 10.5, sum)
	assert.Equal(t, uint64(1), count)

	// Negative values, zeroes, exact boundary hits, and extreme values
	buckets := []float64{-10.0, 0.0, 5.0, 10.0, 50.0, 100.0}
	h := NewPrometheusHistogram(buckets)

	// Initial empty state
	sum, count = h.GetStats()
	assert.Equal(t, 0.0, sum)
	assert.Equal(t, uint64(0), count)
	bounds, counts = h.GetBuckets()
	assert.Equal(t, buckets, bounds)
	assert.Equal(t, []uint64{0, 0, 0, 0, 0, 0}, counts)

	// Negative value below lowest bound
	h.Observe(-20.0) // <= -10.0 (all buckets increment)
	// Value exactly on negative bound
	h.Observe(-10.0) // <= -10.0 (all buckets increment)
	// Value between negative and zero
	h.Observe(-5.0) // > -10.0, <= 0.0 (buckets from 0.0 upwards increment)
	// Exactly zero
	h.Observe(0.0) // <= 0.0 (buckets from 0.0 upwards increment)
	// Value exactly on 5.0 bound
	h.Observe(5.0) // <= 5.0 (buckets from 5.0 upwards increment)
	// Value on 10.0 bound
	h.Observe(10.0) // <= 10.0 (buckets from 10.0 upwards increment)
	// Value between 10.0 and 50.0
	h.Observe(25.0) // <= 50.0, <= 100.0
	// Extreme value exceeding all bounds
	h.Observe(500.0) // Not counted in any bucket!
	h.Observe(math.MaxFloat64)

	bounds, counts = h.GetBuckets()
	assert.Equal(t, len(buckets), len(bounds))
	// -10 bucket has: -20, -10 => 2
	assert.Equal(t, uint64(2), counts[0])
	// 0.0 bucket has: -20, -10, -5, 0 => 4
	assert.Equal(t, uint64(4), counts[1])
	// 5.0 bucket has: -20, -10, -5, 0, 5 => 5
	assert.Equal(t, uint64(5), counts[2])
	// 10.0 bucket has: -20, -10, -5, 0, 5, 10 => 6
	assert.Equal(t, uint64(6), counts[3])
	// 50.0 bucket has: -20, -10, -5, 0, 5, 10, 25 => 7
	assert.Equal(t, uint64(7), counts[4])
	// 100.0 bucket has: same 7
	assert.Equal(t, uint64(7), counts[5])

	sum, count = h.GetStats()
	assert.Equal(t, uint64(9), count)
	assert.Greater(t, sum, 500.0)
}

func TestTelemetryHistogram_ConcurrentRecording(t *testing.T) {
	buckets := []float64{10.0, 50.0, 100.0, 200.0, 500.0}
	h := NewPrometheusHistogram(buckets)

	const workers = 8
	const opsPerWorker = 100

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		wID := w
		goroutinelabels.NewGoroutine("test_histogram_concurrent", fmt.Sprintf("worker-%d", wID)).
			StartWithContext(context.Background(), func(ctx context.Context) error {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					val := float64(i * 5)
					h.Observe(val)
					if i%20 == 0 {
						_, _ = h.GetBuckets()
						_, _ = h.GetStats()
					}
				}
				return nil
			})
	}

	wg.Wait()

	sum, count := h.GetStats()
	assert.Equal(t, uint64(workers*opsPerWorker), count)
	assert.Greater(t, sum, 0.0)

	bounds, counts := h.GetBuckets()
	assert.Equal(t, buckets, bounds)
	assert.Equal(t, len(buckets), len(counts))
	for i := 1; i < len(counts); i++ {
		assert.GreaterOrEqual(t, counts[i], counts[i-1], "cumulative histogram counts must be non-decreasing")
	}
}

func TestScalarMetricAggregator_Comprehensive(t *testing.T) {
	now := time.Now()
	mock := &mockStorageProviderWithCreate{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"latency":                 float64(10.5),
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-8 * time.Minute).Format(time.RFC3339),
				"latency":                 float32(20.5),
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339),
				"latency":                 int(30),
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-4 * time.Minute).Format(time.RFC3339),
				"latency":                 int64(40),
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
				"latency":                 int32(50),
			},
			{
				// Non-numeric should be skipped
				objects.FieldKeyCreatedAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
				"latency":                 "not-a-number",
			},
			{
				// Missing field should be skipped
				objects.FieldKeyCreatedAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
				"other_field":             100.0,
			},
			{
				// Outside window via created_at
				objects.FieldKeyCreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				"latency":                 999.0,
			},
			{
				// Inside window created_at, but updated_at outside window
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				objects.FieldKeyUpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339),
				"latency":                 888.0,
			},
			{
				// Valid updated_at inside window, no created_at
				objects.FieldKeyUpdatedAt: now.Add(-3 * time.Minute).Format(time.RFC3339),
				"latency":                 int(10),
			},
		},
	}

	agg := NewScalarMetricAggregator(mock)
	require.Equal(t, MetricTypeScalar, agg.GetMetricType())

	cfg := &AggregationConfig{
		ObjectKind:   "test_task",
		FieldName:    "latency",
		WindowStart:  now.Add(-30 * time.Minute),
		WindowEnd:    now.Add(30 * time.Minute),
		Aggregations: []string{"sum", "avg", "average", "mean", "min", "minimum", "max", "maximum", "count"},
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Valid values: 10.5, 20.5, 30, 40, 50, 10 = sum: 161
	assert.InDelta(t, 161.0, res.Aggregations["sum"].(float64), 0.001)
	assert.InDelta(t, 161.0/6.0, res.Aggregations["avg"].(float64), 0.001)
	assert.Equal(t, 10.0, res.Aggregations["min"])
	assert.Equal(t, 50.0, res.Aggregations["max"])
	assert.Equal(t, 6, res.Aggregations["count"])

	// Test default aggregations when empty
	cfgDefault := &AggregationConfig{
		ObjectKind:  "test_task",
		FieldName:   "latency",
		WindowStart: now.Add(-30 * time.Minute),
		WindowEnd:   now.Add(30 * time.Minute),
	}
	resDefault, err := agg.Aggregate(context.Background(), cfgDefault)
	require.NoError(t, err)
	assert.NotNil(t, resDefault.Aggregations["sum"])
	assert.NotNil(t, resDefault.Aggregations["avg"])
	assert.NotNil(t, resDefault.Aggregations["min"])
	assert.NotNil(t, resDefault.Aggregations["max"])
	assert.NotNil(t, resDefault.Aggregations["count"])

	// Storage error
	mockErr := &mockStorageProviderWithCreate{err: fmt.Errorf("storage disk failure")}
	aggErr := NewScalarMetricAggregator(mockErr)
	_, err = aggErr.Aggregate(context.Background(), cfg)
	require.Error(t, err)

	// Test createMetricObject directly
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()
	metricID, err := agg.createMetricObject(context.Background(), cfg, res.Aggregations, res.ObjectCount)
	assert.NoError(t, err)
	assert.NotEmpty(t, metricID)

	// Test isInWindow edge cases
	assert.True(t, isInWindow(map[string]any{}, now.Add(-1*time.Hour), now.Add(1*time.Hour)))
	assert.True(t, isInWindow(map[string]any{objects.FieldKeyCreatedAt: "invalid-time"}, now.Add(-1*time.Hour), now.Add(1*time.Hour)))
}

func TestOrderedListMetricAggregator_Comprehensive(t *testing.T) {
	now := time.Now()
	mock := &mockStorageProviderWithCreate{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"pipeline":                []any{"checkout", "build", "test", "deploy"},
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-8 * time.Minute).Format(time.RFC3339),
				"pipeline":                []any{"checkout", "build", "test", "lint", "deploy"},
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339),
				"pipeline":                []any{"checkout", "build", "test", "deploy"},
			},
			{
				// Single item sequence (no transitions)
				objects.FieldKeyCreatedAt: now.Add(-4 * time.Minute).Format(time.RFC3339),
				"pipeline":                []any{"init"},
			},
			{
				// Non-list field skipped
				objects.FieldKeyCreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
				"pipeline":                "not-a-list",
			},
			{
				// Outside window
				objects.FieldKeyCreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				"pipeline":                []any{"outside"},
			},
		},
	}

	agg := NewOrderedListMetricAggregator(mock)
	require.Equal(t, MetricTypeOrderedList, agg.GetMetricType())

	cfg := &AggregationConfig{
		ObjectKind:   "pipeline_run",
		FieldName:    "pipeline",
		WindowStart:  now.Add(-30 * time.Minute),
		WindowEnd:    now.Add(30 * time.Minute),
		Aggregations: []string{"sequence_count", "avg_sequence_length", "transitions", "transition_frequency", "common_sequences"},
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 4, res.Aggregations["sequence_count"])
	assert.InDelta(t, (4.0+5.0+4.0+1.0)/4.0, res.Aggregations["avg_sequence_length"].(float64), 0.001)

	transitions := res.Aggregations[objects.FieldKeyTransitions].(map[string]int)
	assert.Equal(t, 3, transitions["checkout->build"])
	assert.Equal(t, 3, transitions["build->test"])

	commonSeqs := res.Aggregations["common_sequences"].(map[string]int)
	assert.NotNil(t, commonSeqs)

	// Test findCommonSequences with various lengths
	seqs := [][]string{
		{"a", "b", "c", "d"},
		{"a", "b", "c", "d"},
		{"x", "y"},
	}
	common := findCommonSequences(seqs, 2)
	assert.Greater(t, common["[a b]"], 1)
	assert.Greater(t, common["[b c]"], 1)

	// Test empty sequences
	assert.Empty(t, findCommonSequences(nil, 2))

	// Test createMetricObject
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()
	id, err := agg.createMetricObject(context.Background(), cfg, res.Aggregations, res.ObjectCount)
	assert.NoError(t, err)
	assert.NotEmpty(t, id)

	// Error handling
	mockErr := &mockStorageProviderWithCreate{err: fmt.Errorf("query error")}
	aggErr := NewOrderedListMetricAggregator(mockErr)
	_, err = aggErr.Aggregate(context.Background(), cfg)
	require.Error(t, err)
}

func TestStatusHistoryMetricAggregator_Comprehensive(t *testing.T) {
	now := time.Now()
	mock := &mockStorageProviderWithCreate{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"status_history":          []any{"draft", "planned", "in_progress", "complete"},
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				"status_history":          []any{"draft", "planned", "in_progress", "blocked", "in_progress", "complete"},
			},
			{
				// Non-list skipped
				objects.FieldKeyCreatedAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
				"status_history":          "invalid",
			},
		},
	}

	agg := NewStatusHistoryMetricAggregator(mock)
	require.Equal(t, MetricTypeStatusHistory, agg.GetMetricType())

	cfg := &AggregationConfig{
		ObjectKind:   "backlog_item",
		FieldName:    "status_history",
		WindowStart:  now.Add(-30 * time.Minute),
		WindowEnd:    now.Add(30 * time.Minute),
		Aggregations: []string{"state_distribution", "state_durations", "transitions", "transition_frequency", "common_transitions", "avg_history_length"},
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)

	stateDist := res.Aggregations["state_distribution"].(map[string]int)
	assert.Equal(t, 2, stateDist["draft"])
	assert.Equal(t, 2, stateDist["planned"])
	assert.Equal(t, 3, stateDist["in_progress"])
	assert.Equal(t, 2, stateDist["complete"])
	assert.Equal(t, 1, stateDist["blocked"])

	durations := res.Aggregations["state_durations"].(map[string]float64)
	assert.Greater(t, durations["in_progress"], 0.0)

	topTrans := res.Aggregations["common_transitions"].(map[string]int)
	assert.NotNil(t, topTrans)

	// Default aggregations
	cfgDef := &AggregationConfig{
		ObjectKind:  "backlog_item",
		FieldName:   "status_history",
		WindowStart: now.Add(-30 * time.Minute),
		WindowEnd:   now.Add(30 * time.Minute),
	}
	resDef, err := agg.Aggregate(context.Background(), cfgDef)
	require.NoError(t, err)
	assert.NotNil(t, resDef.Aggregations["state_distribution"])
	assert.NotNil(t, resDef.Aggregations[objects.FieldKeyTransitions])
	assert.NotNil(t, resDef.Aggregations["avg_history_length"])

	// Test getTopTransitions helper
	transMap := map[string]int{
		"a->b": 10,
		"b->c": 20,
		"c->d": 5,
	}
	top := getTopTransitions(transMap, 2)
	assert.Len(t, top, 2)
	assert.Equal(t, 20, top["b->c"])
	assert.Equal(t, 10, top["a->b"])

	// Test createMetricObject
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()
	id, err := agg.createMetricObject(context.Background(), cfg, res.Aggregations, res.ObjectCount)
	assert.NoError(t, err)
	assert.NotEmpty(t, id)

	// Query error
	mockErr := &mockStorageProviderWithCreate{err: fmt.Errorf("fail")}
	aggErr := NewStatusHistoryMetricAggregator(mockErr)
	_, err = aggErr.Aggregate(context.Background(), cfg)
	require.Error(t, err)
}

func TestListMetricAggregator_Comprehensive(t *testing.T) {
	now := time.Now()
	mock := &mockStorageProviderWithCreate{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"tags":                    []any{"go", "metrics", "test", 42, 3.14, true},
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				"tags":                    []any{"go", "telemetry", "test", 42},
			},
			{
				// Non-list skipped
				objects.FieldKeyCreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
				"tags":                    123,
			},
			{
				// Outside window
				objects.FieldKeyCreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
				"tags":                    []any{"outside"},
			},
		},
	}

	agg := NewListMetricAggregator(mock)
	require.Equal(t, MetricTypeList, agg.GetMetricType())

	cfg := &AggregationConfig{
		ObjectKind:   "tag_test",
		FieldName:    "tags",
		WindowStart:  now.Add(-30 * time.Minute),
		WindowEnd:    now.Add(30 * time.Minute),
		Aggregations: []string{"count", "unique_count", "distinct_count", "frequency", "distribution", "top_values"},
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, 10, res.Aggregations["count"])
	assert.Greater(t, res.Aggregations["unique_count"].(int), 3)

	freq := res.Aggregations[objects.FieldKeyFrequency].(map[string]int)
	assert.Equal(t, 2, freq["go"])
	assert.Equal(t, 2, freq["test"])
	assert.Equal(t, 2, freq["42"])
	assert.Equal(t, 1, freq["3.14"])

	topVals := res.Aggregations["top_values"].(map[string]int)
	assert.NotNil(t, topVals)

	// Default aggregations
	cfgDef := &AggregationConfig{
		ObjectKind:  "tag_test",
		FieldName:   "tags",
		WindowStart: now.Add(-30 * time.Minute),
		WindowEnd:   now.Add(30 * time.Minute),
	}
	resDef, err := agg.Aggregate(context.Background(), cfgDef)
	require.NoError(t, err)
	assert.Equal(t, 10, resDef.Aggregations["count"])

	// Test createMetricObject
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()
	id, err := agg.createMetricObject(context.Background(), cfg, res.Aggregations, res.ObjectCount)
	assert.NoError(t, err)
	assert.NotEmpty(t, id)

	// Query error
	mockErr := &mockStorageProviderWithCreate{err: fmt.Errorf("storage error")}
	aggErr := NewListMetricAggregator(mockErr)
	_, err = aggErr.Aggregate(context.Background(), cfg)
	require.Error(t, err)
}

func TestTimeSeriesWriter_AndReader_EdgeCases(t *testing.T) {
	// Writer validation errors
	_, err := NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: 0, Dir: "/tmp", Series: "s1"})
	assert.Error(t, err)

	_, err = NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: time.Hour, Dir: "", Series: "s1"})
	assert.Error(t, err)

	_, err = NewTimeSeriesWriter(TimeSeriesConfig{ChunkDuration: time.Hour, Dir: "/tmp", Series: ""})
	assert.Error(t, err)

	// Valid creation and appending across chunk rotation
	tmpDir := t.TempDir()
	cfg := TimeSeriesConfig{
		ChunkDuration: time.Minute,
		Dir:           tmpDir,
		Series:        "api/latency:p99",
	}

	w, err := NewTimeSeriesWriter(cfg)
	require.NoError(t, err)
	require.NotNil(t, w)

	baseTime := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	// Point 1: at baseTime
	err = w.Append(TimeSeriesPoint{Ts: baseTime.Add(5 * time.Second), Value: 100})
	require.NoError(t, err)

	// Point 2: decreasing value (tests zigzag negative delta)
	err = w.Append(TimeSeriesPoint{Ts: baseTime.Add(15 * time.Second), Value: 80})
	require.NoError(t, err)

	// Point 3: earlier timestamp than base (dt < 0 check)
	err = w.Append(TimeSeriesPoint{Ts: baseTime.Add(-10 * time.Second), Value: 90})
	require.NoError(t, err)

	// Point 4: across chunk boundary (triggers rotateChunk)
	err = w.Append(TimeSeriesPoint{Ts: baseTime.Add(75 * time.Second), Value: 150})
	require.NoError(t, err)

	// Point 5: another point in second chunk
	err = w.Append(TimeSeriesPoint{Ts: baseTime.Add(85 * time.Second), Value: 200})
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	// TimeSeriesReader
	reader := NewTimeSeriesReader(cfg)
	require.NotNil(t, reader)

	var readPoints []TimeSeriesPoint
	err = reader.Range(baseTime, baseTime.Add(2*time.Minute), func(p TimeSeriesPoint) error {
		readPoints = append(readPoints, p)
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(readPoints), 4)

	// Reader invalid ChunkDuration
	badReader := NewTimeSeriesReader(TimeSeriesConfig{ChunkDuration: 0})
	err = badReader.Range(baseTime, baseTime.Add(time.Hour), func(p TimeSeriesPoint) error { return nil })
	assert.Error(t, err)

	// Reader non-existent directory/series
	emptyReader := NewTimeSeriesReader(TimeSeriesConfig{
		ChunkDuration: time.Minute,
		Dir:           t.TempDir(),
		Series:        "non_existent",
	})
	err = emptyReader.Range(baseTime, baseTime.Add(time.Minute), func(p TimeSeriesPoint) error { return nil })
	assert.NoError(t, err)

	// Reader callback early exit on error
	errStop := fmt.Errorf("stop early")
	err = reader.Range(baseTime, baseTime.Add(2*time.Minute), func(p TimeSeriesPoint) error {
		return errStop
	})
	assert.ErrorIs(t, err, errStop)

	// SanitizeSeriesName
	assert.Equal(t, "api_latency_p99_v1", SanitizeSeriesName("api/latency:p99.v1"))
	assert.Equal(t, "foo_bar_123_", SanitizeSeriesName("foo@bar#123!"))
}

func TestSamplerRegistry_EdgeCasesAndFallbacks(t *testing.T) {
	mock := &mockStorageProviderWithCreate{}
	reg := NewSamplerRegistry(mock)
	require.NotNil(t, reg)

	// Distinct count on empty registry
	assert.Equal(t, 0, reg.distinctSamplerCountLocked())
	assert.Equal(t, 0, reg.samplerRefCountLocked(nil))

	// GetOrCreateSampler standard
	s1, err := reg.GetOrCreateSampler("test_kind", "latency", MetricTypeScalar)
	require.NoError(t, err)
	require.NotNil(t, s1)

	// Reusing existing sampler
	s1Reuse, err := reg.GetOrCreateSampler("test_kind", "latency", MetricTypeScalar)
	require.NoError(t, err)
	assert.Same(t, s1, s1Reuse)

	// Get aggregator for various types
	assert.Equal(t, MetricTypeScalar, reg.getAggregatorForMetricType(MetricTypeScalar).GetMetricType())
	assert.Equal(t, MetricTypeList, reg.getAggregatorForMetricType(MetricTypeList).GetMetricType())
	assert.Equal(t, MetricTypeOrderedList, reg.getAggregatorForMetricType(MetricTypeOrderedList).GetMetricType())
	assert.Equal(t, MetricTypeStatusHistory, reg.getAggregatorForMetricType(MetricTypeStatusHistory).GetMetricType())
	assert.NotNil(t, reg.getAggregatorForMetricType("unknown_metric_type"))

	// FlushAll
	err = reg.FlushAll()
	assert.NoError(t, err)

	// distinctNonNilSamplersLocked
	samplers := reg.distinctNonNilSamplersLocked()
	assert.Len(t, samplers, 1)

	// Unregister existing sampler
	err = reg.UnregisterSampler("test_kind", "latency", MetricTypeScalar)
	assert.NoError(t, err)

	// Unregister non-existent sampler
	err = reg.UnregisterSampler("no_such_kind", "field", MetricTypeScalar)
	assert.NoError(t, err)

	// GetOrCreatePipelineSampleSampler
	pipelineSampler, err := reg.GetOrCreatePipelineSampleSampler()
	require.NoError(t, err)
	require.NotNil(t, pipelineSampler)

	// StopAll
	err = reg.StopAll()
	assert.NoError(t, err)
}

func TestMetricPipeline_TelemetryAndSubscribers(t *testing.T) {
	mock := &mockStorageProviderWithCreate{}
	pipeline := NewMetricPipeline(mock)
	require.NotNil(t, pipeline)

	// AddSubscriber
	var receivedEvent map[string]any
	pipeline.AddSubscriber(func(evt map[string]any) {
		receivedEvent = evt
	})

	testEvent := map[string]any{
		objects.FieldKeyKind: "test_kind",
		"message":            "hello telemetry",
	}
	_, err := pipeline.Sample(testEvent)
	assert.NoError(t, err)
	assert.Equal(t, "test_kind", receivedEvent[objects.FieldKeyKind])

	// Trait detection and aggregators
	assert.Equal(t, MetricTypeStatusHistory, pipeline.detectMetricType([]string{MetricTypeStatusHistory, MetricTypeScalar}))
	assert.Equal(t, MetricTypeOrderedList, pipeline.detectMetricType([]string{MetricTypeOrderedList}))
	assert.Equal(t, MetricTypeList, pipeline.detectMetricType([]string{MetricTypeList}))
	assert.Equal(t, MetricTypeScalar, pipeline.detectMetricType([]string{MetricTypeScalar}))
	assert.Equal(t, "", pipeline.detectMetricType([]string{"unknown_trait"}))

	assert.Equal(t, MetricTypeScalar, pipeline.getDefaultAggregator(MetricTypeScalar).GetMetricType())
	assert.Equal(t, MetricTypeList, pipeline.getDefaultAggregator(MetricTypeList).GetMetricType())
	assert.Equal(t, MetricTypeOrderedList, pipeline.getDefaultAggregator(MetricTypeOrderedList).GetMetricType())
	assert.Equal(t, MetricTypeStatusHistory, pipeline.getDefaultAggregator(MetricTypeStatusHistory).GetMetricType())
	assert.Nil(t, pipeline.getDefaultAggregator("non_existent"))

	// HasReadableTrait
	assert.True(t, HasReadableTrait(map[string]any{"traits": []any{"readable"}}))
	assert.False(t, HasReadableTrait(map[string]any{"traits": []any{"write_only"}}))
	assert.False(t, HasReadableTrait(nil))

	// Register custom aggregator and CollectMetrics
	customAgg := NewScalarMetricAggregator(mock)
	pipeline.RegisterAggregator(customAgg)

	// CollectMetrics without sampler
	cfg := &AggregationConfig{
		ObjectKind:   "test_kind",
		FieldName:    "latency",
		MetricType:   MetricTypeScalar,
		WindowStart:  time.Now().Add(-time.Hour),
		WindowEnd:    time.Now(),
		Aggregations: []string{"count"},
	}
	res, err := pipeline.CollectMetrics(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, res)

	// CollectMetrics with unknown type and no default aggregator
	cfgUnknown := &AggregationConfig{
		ObjectKind: "test_kind",
		MetricType: "unsupported_metric_type",
	}
	_, err = pipeline.CollectMetrics(context.Background(), cfgUnknown)
	assert.Error(t, err)

	// Sample with uninitialized registry
	brokenPipeline := &MetricPipeline{}
	_, err = brokenPipeline.Sample(testEvent)
	assert.Error(t, err)

	// Trait registry lazy load
	assert.NotNil(t, pipeline.getTraitRegistry())
}

func TestResourceAvailability_Coverage(t *testing.T) {
	avail := GetResourceAvailability()
	require.NotNil(t, avail)
	assert.Greater(t, avail.TotalGoroutines, 0)
	assert.Greater(t, avail.UsedGoroutines, 0)
	assert.GreaterOrEqual(t, avail.AvailableCompute, 0)
	assert.Greater(t, avail.MemoryAllocBytes, uint64(0))
}

func TestValidationBaseMetric_Helpers(t *testing.T) {
	// truncateTag
	shortTag := "short_tag"
	assert.Equal(t, shortTag, truncateTag(shortTag, 64))
	assert.Equal(t, "", truncateTag(shortTag, 0))
	assert.Equal(t, "sho", truncateTag(shortTag, 3))
	assert.Equal(t, "short...", truncateTag(shortTag, 8))

	// asyncValidationMetricTitle
	title := asyncValidationMetricTitle(10, 2, 12)
	assert.Contains(t, title, "10")
	assert.Contains(t, title, "2")
	assert.Contains(t, title, "12")

	// FormatMetricWindow
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	sStr, eStr, err := FormatMetricWindow("MET-001", start, end)
	require.NoError(t, err)
	assert.NotEmpty(t, sStr)
	assert.NotEmpty(t, eStr)

	_, _, err = FormatMetricWindow("", start, end)
	assert.Error(t, err)

	// ComputeEffectiveObjectCount
	assert.Equal(t, 0, ComputeEffectiveObjectCount(-5, 10))
	assert.Equal(t, 10, ComputeEffectiveObjectCount(0, 10))
	assert.Equal(t, 25, ComputeEffectiveObjectCount(25, 10))

	// FlushCacheValidationMetricsToStorage
	mock := &mockStorageProviderWithCreate{}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	FlushCacheValidationMetricsToStorage(mock, logger)

	// FlushGraphProviderMetricsToStorage
	FlushGraphProviderMetricsToStorage(mock, logger)

	// BuildAsyncValidationBaseMetricInstance & PersistAsyncValidationMetricsToStorageAsync
	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	vm := validation.NewValidationMetrics()
	vm.SetTotalObjects(10)
	vm.IncrementValidated()
	vm.IncrementFailed()
	vm.Finalize()

	payload := []byte(`{"validated":1}`)
	inst, err := BuildAsyncValidationBaseMetricInstance(vm, "op-1", "MET-1", payload)
	require.NoError(t, err)
	assert.NotNil(t, inst)
	assert.Equal(t, objects.KindBaseMetric, inst[objects.FieldKeyKind])

	// Nil error check
	_, err = BuildAsyncValidationBaseMetricInstance(nil, "op-1", "MET-1", payload)
	assert.Error(t, err)

	_, err = BuildAsyncValidationBaseMetricInstance(vm, "op-1", "", payload)
	assert.Error(t, err)

	// PersistBaseMetricInstance
	PersistBaseMetricInstance(mock, inst, "MET-12345", "test metric", logger)

	// PersistAsyncValidationMetricsToStorageAsync
	PersistAsyncValidationMetricsToStorageAsync(mock, vm, "op-1", logger)
}

func TestSampler_DeepCoverage(t *testing.T) {
	// SamplerConfig.Validate
	c1 := &SamplerConfig{BatchSize: 0}
	assert.Error(t, c1.Validate())
	c2 := &SamplerConfig{BatchSize: 10, MaxBatchSize: 5}
	assert.Error(t, c2.Validate())
	c3 := &SamplerConfig{BatchSize: 5, MaxBatchSize: 10, FlushInterval: -1}
	assert.Error(t, c3.Validate())
	c4 := &SamplerConfig{BatchSize: 5, MaxBatchSize: 10, FlushInterval: time.Second}
	assert.NoError(t, c4.Validate())

	// inferAggregationObjectKind
	assert.Equal(t, "custom", inferAggregationObjectKind("custom", nil))
	assert.Equal(t, "inferred", inferAggregationObjectKind("*", []map[string]any{{objects.FieldKeyKind: "inferred"}}))
	assert.Equal(t, "target", inferAggregationObjectKind("", []map[string]any{{objects.FieldKeyTargetKind: "target"}}))
	assert.Equal(t, "*", inferAggregationObjectKind("*", nil))

	// Immediate metric creation when disabled
	mock := &mockStorageProviderWithCreate{}
	agg := NewScalarMetricAggregator(mock)
	cfg := DefaultSamplerConfig()
	cfg.Enabled = false
	cfg.ObjectKind = "backlog_item"
	cfg.FlushInterval = 0 // no ticker

	s, err := NewSampler(cfg, mock, agg)
	require.NoError(t, err)
	defer s.Stop()

	metricsrecording.EnterAllowRecording()
	defer metricsrecording.LeaveAllowRecording()

	event := map[string]any{
		objects.FieldKeyKind: "backlog_item",
		"value":              42.0,
	}
	flushed, err := s.Sample(event)
	assert.NoError(t, err)
	assert.True(t, flushed)

	// Batch count and pending events
	assert.Equal(t, 0, s.GetBatchCount())
	assert.Equal(t, 0, s.GetTotalPendingEvents())
}

func TestSamplerRegistry_CapacityAndAliasing(t *testing.T) {
	mock := &mockStorageProviderWithCreate{}
	reg := NewSamplerRegistry(mock)

	// Test samplerRefCountLocked
	s := &Sampler{}
	assert.Equal(t, 0, reg.samplerRefCountLocked(nil))
	reg.samplers["k1"] = s
	reg.samplers["k2"] = s
	assert.Equal(t, 2, reg.samplerRefCountLocked(s))
	delete(reg.samplers, "k1")
	delete(reg.samplers, "k2")

	// Test fallback alias
	reg.bindFallbackSamplerKeyLocked(s)
	assert.Same(t, s, reg.samplers[fallbackSamplerKey])

	// Test Unregister with ref count > 1
	realSampler, err := NewSampler(DefaultSamplerConfig(), mock, NewScalarMetricAggregator(mock))
	require.NoError(t, err)
	defer realSampler.Stop()

	key1 := reg.getSamplerKey("alias1", "f", "t")
	key2 := reg.getSamplerKey("alias2", "f", "t")
	reg.samplers[key1] = realSampler
	reg.samplers[key2] = realSampler
	err = reg.UnregisterSampler("alias1", "f", "t")
	assert.NoError(t, err)
	// realSampler still exists under key2, not stopped
	assert.Equal(t, 1, reg.samplerRefCountLocked(realSampler))
	err = reg.UnregisterSampler("alias2", "f", "t")
	assert.NoError(t, err)
}
