package metrics

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

type mockStorageWithError struct {
	storage.ObjectStorageProvider
	listErr error
	objs    []map[string]any
}

func (m *mockStorageWithError) List(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return &storage.QueryResult{Objects: m.objs}, nil
}

func (m *mockStorageWithError) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	obj[objects.FieldKeyID] = "BAS-mock-metric-id"
	return nil
}

func TestDiscoverMetricFields_And_InitializeSamplerConfig(t *testing.T) {
	mp := NewMetricPipeline(storage.NewNoopObjectStorage())

	// Non-existent ontology
	_, err := mp.DiscoverMetricFields("nonexistent_kind_xyz_123")
	assert.Error(t, err)

	// Existing kind
	fields, err := mp.DiscoverMetricFields("backlog_item")
	assert.NoError(t, err)
	_ = fields

	// InitializeSamplerConfig on non-existent config (succeeds by design)
	tmpDir := t.TempDir()
	err = mp.InitializeSamplerConfig(tmpDir)
	assert.NoError(t, err)

	// Malformed config file
	cfgDir := filepath.Join(tmpDir, ".zqk", "metrics")
	require.NoError(t, fileutil.MkdirAll(cfgDir, paths.DirPerm755))
	cfgPath := filepath.Join(cfgDir, "sampler_config.yaml")
	require.NoError(t, fileutil.WriteFile(cfgPath, []byte("invalid: yaml: ["), paths.FilePerm644))
	err = mp.InitializeSamplerConfig(tmpDir)
	assert.Error(t, err)

	// Invalid default flush interval in config
	badCfg := `
defaults:
  flush_interval: "not-a-duration"
`
	require.NoError(t, fileutil.WriteFile(cfgPath, []byte(badCfg), paths.FilePerm644))
	err = mp.InitializeSamplerConfig(tmpDir)
	assert.Error(t, err)

	// Valid config file
	validCfg := `
defaults:
  enabled: true
  flush_interval: "5s"
  batch_size: 10
  max_batch_size: 50
`
	require.NoError(t, fileutil.WriteFile(cfgPath, []byte(validCfg), paths.FilePerm644))
	err = mp.InitializeSamplerConfig(tmpDir)
	assert.NoError(t, err)
}

func TestApplySamplerConfig_Comprehensive(t *testing.T) {
	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() { _ = reg.StopAll() })

	// 1. Invalid default flush interval
	cfg1 := &SamplerConfigFile{}
	cfg1.Defaults.FlushInterval = "bad-interval"
	err := ApplySamplerConfig(reg, cfg1, "")
	assert.Error(t, err)

	// 2. ProfileRefs with missing profile
	cfg2 := &SamplerConfigFile{
		ProfileRefs: []string{"nonexistent_profile_ref"},
	}
	cfg2.Defaults.FlushInterval = "1s"
	err = ApplySamplerConfig(reg, cfg2, "")
	assert.Error(t, err)

	// 3. Profiles inline with invalid namespace_id
	cfg3 := &SamplerConfigFile{
		Profiles: map[string]map[string]any{
			"prof1": {
				"namespace_id": "invalid:namespace:foo",
			},
		},
	}
	cfg3.Defaults.FlushInterval = "1s"
	err = ApplySamplerConfig(reg, cfg3, "")
	assert.Error(t, err)

	// 4. Profiles inline with legacy namespace_id suffix
	cfg4 := &SamplerConfigFile{
		Profiles: map[string]map[string]any{
			"prof_legacy": {
				"namespace_id": "custom:kernel:metrics",
				"spec": map[string]any{
					"batch_size": 20,
				},
			},
		},
	}
	cfg4.Defaults.FlushInterval = "1s"
	err = ApplySamplerConfig(reg, cfg4, "")
	assert.NoError(t, err)

	// 5. Profiles inline with non-existent extends
	cfg5 := &SamplerConfigFile{
		Profiles: map[string]map[string]any{
			"prof_child": {
				"namespace_id": paths.MetricsNamespaceID,
				"extends":      "nonexistent_parent_123",
			},
		},
	}
	cfg5.Defaults.FlushInterval = "1s"
	err = ApplySamplerConfig(reg, cfg5, "")
	assert.Error(t, err)

	// 6. Profiles inline with invalid spec (applySamplerProfile fails on bad flush_interval)
	cfg6 := &SamplerConfigFile{
		Profiles: map[string]map[string]any{
			"prof_bad_spec": {
				"namespace_id": paths.MetricsNamespaceID,
				"spec": map[string]any{
					"flush_interval": "not-valid-dur",
				},
			},
		},
	}
	cfg6.Defaults.FlushInterval = "1s"
	err = ApplySamplerConfig(reg, cfg6, "")
	assert.Error(t, err)

	// 7. Legacy Samplers direct configuration
	// 7a. Invalid flush interval in sampler
	cfg7a := &SamplerConfigFile{}
	cfg7a.Defaults.FlushInterval = "1s"
	cfg7a.Samplers = append(cfg7a.Samplers, struct {
		ObjectKind      string `yaml:"object_kind"`
		FieldName       string `yaml:"field_name,omitempty"`
		MetricType      string `yaml:"metric_type"`
		Enabled         bool   `yaml:"enabled"`
		BatchSize       int    `yaml:"batch_size"`
		MaxBatchSize    int    `yaml:"max_batch_size"`
		FlushInterval   string `yaml:"flush_interval"`
		GroupByObjectID bool   `yaml:"group_by_object_id"`
		Description     string `yaml:"description,omitempty"`
	}{
		ObjectKind:    "task",
		FlushInterval: "bad-time",
	})
	err = ApplySamplerConfig(reg, cfg7a, "")
	assert.Error(t, err)

	// 7b. Valid direct samplers covering opt-in, opt-out, defaults
	cfg7b := &SamplerConfigFile{
		OptOut: []string{"denied_kind"},
		OptIn:  []string{"allowed_kind", "denied_kind"},
	}
	cfg7b.Defaults.FlushInterval = "2s"
	cfg7b.Defaults.BatchSize = 10
	cfg7b.Defaults.MaxBatchSize = 50
	cfg7b.Defaults.Enabled = true
	cfg7b.Defaults.GroupByObjectID = false
	cfg7b.Samplers = append(cfg7b.Samplers,
		struct {
			ObjectKind      string `yaml:"object_kind"`
			FieldName       string `yaml:"field_name,omitempty"`
			MetricType      string `yaml:"metric_type"`
			Enabled         bool   `yaml:"enabled"`
			BatchSize       int    `yaml:"batch_size"`
			MaxBatchSize    int    `yaml:"max_batch_size"`
			FlushInterval   string `yaml:"flush_interval"`
			GroupByObjectID bool   `yaml:"group_by_object_id"`
			Description     string `yaml:"description,omitempty"`
		}{
			ObjectKind:      "allowed_kind",
			MetricType:      MetricTypeScalar,
			FlushInterval:   "3s",
			BatchSize:       5,
			MaxBatchSize:    25,
			Enabled:         true,
			GroupByObjectID: true,
		},
		struct {
			ObjectKind      string `yaml:"object_kind"`
			FieldName       string `yaml:"field_name,omitempty"`
			MetricType      string `yaml:"metric_type"`
			Enabled         bool   `yaml:"enabled"`
			BatchSize       int    `yaml:"batch_size"`
			MaxBatchSize    int    `yaml:"max_batch_size"`
			FlushInterval   string `yaml:"flush_interval"`
			GroupByObjectID bool   `yaml:"group_by_object_id"`
			Description     string `yaml:"description,omitempty"`
		}{
			ObjectKind: "denied_kind", // in opt-out list -> becomes disabled
			MetricType: MetricTypeScalar,
		},
		struct {
			ObjectKind      string `yaml:"object_kind"`
			FieldName       string `yaml:"field_name,omitempty"`
			MetricType      string `yaml:"metric_type"`
			Enabled         bool   `yaml:"enabled"`
			BatchSize       int    `yaml:"batch_size"`
			MaxBatchSize    int    `yaml:"max_batch_size"`
			FlushInterval   string `yaml:"flush_interval"`
			GroupByObjectID bool   `yaml:"group_by_object_id"`
			Description     string `yaml:"description,omitempty"`
		}{
			ObjectKind: "other_kind", // not in opt-in list -> becomes disabled
			MetricType: MetricTypeScalar,
		},
	)
	err = ApplySamplerConfig(reg, cfg7b, "")
	assert.NoError(t, err)

	// 7c. Direct sampler with Defaults.Enabled = false
	cfg7c := &SamplerConfigFile{}
	cfg7c.Defaults.FlushInterval = "2s"
	cfg7c.Defaults.BatchSize = 10
	cfg7c.Defaults.MaxBatchSize = 50
	cfg7c.Defaults.Enabled = false
	cfg7c.Samplers = append(cfg7c.Samplers, struct {
		ObjectKind      string `yaml:"object_kind"`
		FieldName       string `yaml:"field_name,omitempty"`
		MetricType      string `yaml:"metric_type"`
		Enabled         bool   `yaml:"enabled"`
		BatchSize       int    `yaml:"batch_size"`
		MaxBatchSize    int    `yaml:"max_batch_size"`
		FlushInterval   string `yaml:"flush_interval"`
		GroupByObjectID bool   `yaml:"group_by_object_id"`
		Description     string `yaml:"description,omitempty"`
	}{
		ObjectKind: "forced_disabled",
		MetricType: MetricTypeScalar,
		Enabled:    true,
	})
	err = ApplySamplerConfig(reg, cfg7c, "")
	assert.NoError(t, err)
}

func TestValidationBaseMetric_TruncateAndTitles(t *testing.T) {
	// truncateTag branches
	assert.Equal(t, "", truncateTag("foo", 0))
	assert.Equal(t, "", truncateTag("foo", -1))
	assert.Equal(t, "abc", truncateTag("abc", 5))
	assert.Equal(t, "ab", truncateTag("abcdef", 2))
	assert.Equal(t, "abc", truncateTag("abcdef", 3))
	assert.Equal(t, "ab...", truncateTag("abcdefgh", 5))

	// asyncValidationMetricTitle short string
	assert.Equal(t, "Async check: 10 validated, 2 failed of 12", asyncValidationMetricTitle(10, 2, 12))

	// BuildAsyncValidationBaseMetricInstance with zero EndTime and empty operationID
	vm := validation.NewValidationMetrics()
	vm.StartTime = time.Now().UTC()
	vm.EndTime = time.Time{} // zero time branch
	inst, err := BuildAsyncValidationBaseMetricInstance(vm, "", "BAS-test-zero-end", []byte("{}"))
	assert.NoError(t, err)
	assert.NotNil(t, inst)

	// PersistAsyncValidationMetricsToStorageAsync early exits
	PersistAsyncValidationMetricsToStorageAsync(nil, vm, "op", nil)
	PersistAsyncValidationMetricsToStorageAsync(storage.NewNoopObjectStorage(), nil, "op", nil)

	// When recording is disabled
	PersistAsyncValidationMetricsToStorageAsync(storage.NewNoopObjectStorage(), vm, "op", nil)
}

func TestProcessLifecycleJSONL_Coverage(t *testing.T) {
	// Early exit on empty root or nil row
	AppendProcessLifecycleJSONL("", map[string]any{"id": "1"})
	AppendProcessLifecycleJSONL("/tmp", nil)

	// Append with pre-existing ts_rfc3339nano
	tmpDir := t.TempDir()
	row := map[string]any{
		objects.FieldKeyID: "PLC-001",
		"ts_rfc3339nano":   time.Now().UTC().Format(time.RFC3339Nano),
		"event":            ProcessLifecycleEventCriteriaAutovalidateBatch,
	}
	AppendProcessLifecycleJSONL(tmpDir, row)

	// Append failure path (e.g. project root is a regular file)
	regularFile := filepath.Join(tmpDir, "somefile.txt")
	require.NoError(t, fileutil.WriteFile(regularFile, []byte("data"), paths.FilePerm644))
	AppendProcessLifecycleJSONL(regularFile, row)

	// TruncateProcessLifecycleDetail branches
	assert.Equal(t, "", TruncateProcessLifecycleDetail("", 10))
	assert.Equal(t, "text", TruncateProcessLifecycleDetail("text", 0))
	assert.Equal(t, "text", TruncateProcessLifecycleDetail("text", -5))
	assert.Equal(t, "hello", TruncateProcessLifecycleDetail("hello", 10))
	assert.Equal(t, "hell…", TruncateProcessLifecycleDetail("hello world", 4))
}

func TestCacheAndGraphValidation_TitlesAndErrors(t *testing.T) {
	// cacheValidationMetricTitle
	title := cacheValidationMetricTitle(10, 20, 30)
	assert.NotEmpty(t, title)

	// BuildCacheValidationBaseMetricInstance error on empty metric ID
	now := time.Now().UTC()
	meta := storage.CacheMetricsEncodeMeta{
		WindowStart: now,
		WindowEnd:   now,
	}
	_, err := BuildCacheValidationBaseMetricInstance("", []byte("{}"), meta)
	assert.Error(t, err)

	// Large payload truncation (> 256KB)
	largePayload := make([]byte, 300*1024)
	for i := range largePayload {
		largePayload[i] = 'a'
	}
	inst, err := BuildCacheValidationBaseMetricInstance("BAS-trunc-cache", largePayload, meta)
	assert.NoError(t, err)
	assert.NotNil(t, inst)

	// PersistCacheValidationMetricsJSONAsync early exits and error logging
	PersistCacheValidationMetricsJSONAsync(nil, []byte("{}"), nil, meta)

	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	PersistCacheValidationMetricsJSONAsync(storage.NewNoopObjectStorage(), []byte("{}"), logger, meta)

	// graphProviderMetricTitle
	graphTitle := graphProviderMetricTitle(10, 20, 30)
	assert.NotEmpty(t, graphTitle)

	// BuildGraphProviderMetricsBaseMetricInstance error on empty metric ID
	graphMeta := GraphProviderPersistMeta{
		WindowStart: now,
		WindowEnd:   now,
	}
	_, err = BuildGraphProviderMetricsBaseMetricInstance("", []byte("{}"), graphMeta)
	assert.Error(t, err)

	// Large payload truncation for graph provider
	instGraph, err := BuildGraphProviderMetricsBaseMetricInstance("BAS-trunc-graph", largePayload, graphMeta)
	assert.NoError(t, err)
	assert.NotNil(t, instGraph)

	// PersistGraphProviderMetricsJSONAsync early exits and error logging
	PersistGraphProviderMetricsJSONAsync(nil, []byte("{}"), nil, graphMeta)
	PersistGraphProviderMetricsJSONAsync(storage.NewNoopObjectStorage(), []byte("{}"), logger, graphMeta)
}

func TestSampler_EvictionAndDisabledKind(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	cfg := DefaultSamplerConfig()
	cfg.ObjectKind = "sampler_evict_test"
	cfg.MetricType = MetricTypeScalar
	cfg.BatchSize = 100
	cfg.GroupByObjectID = true

	s, err := NewSampler(cfg, storage.NewNoopObjectStorage(), NewScalarMetricAggregator(storage.NewNoopObjectStorage()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Stop() })

	// Sample with empty kind and target_kind
	_, err = s.Sample(map[string]any{
		objects.FieldKeyTargetKind: "sampler_evict_test",
		"val":                      1.0,
	})
	assert.NoError(t, err)

	// Sample with empty kind and empty target_kind (falls back to config.ObjectKind)
	_, err = s.Sample(map[string]any{
		"val": 2.0,
	})
	assert.NoError(t, err)

	// Sample with kind denied
	metricsrecording.DenyKind("denied_sample_kind")
	t.Cleanup(func() { metricsrecording.ClearDeniedKinds() })
	ok, err := s.Sample(map[string]any{
		objects.FieldKeyKind: "denied_sample_kind",
	})
	assert.NoError(t, err)
	assert.False(t, ok)

	// Sample when config.Enabled == false -> calls createMetricImmediately
	storeWithID := &mockStorageWithError{}
	sDisabled, err := NewSampler(&SamplerConfig{
		Enabled:      false,
		ObjectKind:   "disabled_sampler",
		MetricType:   MetricTypeScalar,
		BatchSize:    10,
		MaxBatchSize: 20,
	}, storeWithID, NewScalarMetricAggregator(storeWithID))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sDisabled.Stop() })

	_, err = sDisabled.Sample(map[string]any{
		objects.FieldKeyKind: "disabled_sampler",
		"val":                5.0,
	})
	assert.NoError(t, err)

	// Trigger batch eviction: fill s.batches beyond maxSamplerBatchKeys
	for i := 0; i <= maxSamplerBatchKeys+5; i++ {
		_, err := s.Sample(map[string]any{
			objects.FieldKeyKind: "sampler_evict_test",
			"object_id":          fmt.Sprintf("obj_%d", i),
			"val":                float64(i),
		})
		assert.NoError(t, err)
	}
}

func TestSamplerRegistry_CapacityAndReplacement(t *testing.T) {
	reg := NewSamplerRegistry(storage.NewNoopObjectStorage())
	t.Cleanup(func() { _ = reg.StopAll() })

	// 1. Invalid config validation
	err := reg.RegisterSampler(&SamplerConfig{
		BatchSize: -1, // invalid
	})
	assert.Error(t, err)

	// 2. Replacing an existing sampler
	cfg := DefaultSamplerConfig()
	cfg.ObjectKind = "replace_test"
	cfg.FieldName = "f1"
	cfg.MetricType = MetricTypeScalar
	err = reg.RegisterSampler(cfg)
	require.NoError(t, err)

	// Register same key again -> replaces old sampler
	cfg2 := DefaultSamplerConfig()
	cfg2.ObjectKind = "replace_test"
	cfg2.FieldName = "f1"
	cfg2.MetricType = MetricTypeScalar
	cfg2.BatchSize = 42
	err = reg.RegisterSampler(cfg2)
	require.NoError(t, err)

	// 3. GetOrCreateSampler cache hit
	s1, err := reg.GetOrCreateSampler("replace_test", "f1", MetricTypeScalar)
	assert.NoError(t, err)
	assert.NotNil(t, s1)

	// 4. Fill to maxSamplers
	for i := 0; reg.distinctSamplerCountLocked() < maxSamplers; i++ {
		c := DefaultSamplerConfig()
		c.ObjectKind = fmt.Sprintf("cap_kind_%d", i)
		c.FieldName = "f"
		c.MetricType = MetricTypeScalar
		_ = reg.RegisterSampler(c)
	}

	// 5. At capacity with non-system metric in RegisterSampler -> error
	cOverflow := DefaultSamplerConfig()
	cOverflow.ObjectKind = "overflow_kind"
	cOverflow.FieldName = "overflow_field"
	cOverflow.MetricType = MetricTypeScalar
	err = reg.RegisterSampler(cOverflow)
	assert.ErrorContains(t, err, "at capacity")
}

func TestAIMetrics_AdditionalCoverage(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. splitPair without "|"
	parts := splitPair("single_key")
	assert.Equal(t, []string{"single_key"}, parts)

	// 2. enhancePCSWithCommits branches
	cmHighFreq := &CommitMetrics{
		CommitFrequency: 6.0, // > freqHigh (5.0) -> -5 points
		CommitPattern:   "irregular",
	}
	pcsHigh := enhancePCSWithCommits(80.0, cmHighFreq)
	assert.Equal(t, 72.0, pcsHigh) // 80 - 5 - 3 = 72

	// Bounds clamping
	cmExtreme := &CommitMetrics{
		CommitFrequency: 0.1,
		CommitPattern:   "decreasing",
	}
	pcsClampMin := enhancePCSWithCommits(5.0, cmExtreme) // 5 - 10 - 5 = -10 -> clamp to 0
	assert.Equal(t, 0.0, pcsClampMin)

	pcsClampMax := enhancePCSWithCommits(99.0, &CommitMetrics{
		CommitFrequency: 1.0,
		CommitPattern:   "increasing",
	}) // 99 + 5 + 3 = 107 -> clamp to 100
	assert.Equal(t, 100.0, pcsClampMax)

	// 3. calculateBaseDB with blockers and dependencies
	storeDB := &mockStorageWithError{
		objs: []map[string]any{
			{
				objects.FieldKeyID:       "RB-01",
				objects.FieldKeyTitle:    "Blocker Item",
				objects.FieldKeySeverity: "critical",
				objects.FieldKeyType:     "blocker",
			},
			{
				objects.FieldKeyID:       "RB-02",
				objects.FieldKeyTitle:    "Dependency Item",
				objects.FieldKeySeverity: "high",
				objects.FieldKeyType:     "dependency",
			},
		},
	}
	db, err := calculateBaseDB(ctx, storeDB, secCtx, "proj-1")
	require.NoError(t, err)
	assert.Len(t, db.Blockers, 1)
	assert.Len(t, db.Dependencies, 1)

	// calculateBaseDB storage error
	storeDBErr := &mockStorageWithError{listErr: fmt.Errorf("db list error")}
	_, err = calculateBaseDB(ctx, storeDBErr, secCtx, "proj-1")
	assert.Error(t, err)

	// 4. CalculateProjectMetrics error paths
	// 4a. calculateBasePCS error (storage error)
	_, err = CalculateProjectMetrics(ctx, storeDBErr, secCtx, "proj-1", false)
	assert.ErrorContains(t, err, "failed to calculate base PCS")

	// 4b. calculateCommitMetrics int64 lines and stable commit pattern
	now := time.Now().UTC()
	storeCommits := &mockStorageWithError{
		objs: []map[string]any{
			{
				objects.FieldKeyCommitHash:   "hash1",
				objects.FieldKeyCommitDate:   now.AddDate(0, 0, -2).Format(time.RFC3339),
				objects.FieldKeyAuthor:       "alice",
				objects.FieldKeyLinesAdded:   int64(50),
				objects.FieldKeyLinesRemoved: int64(20),
			},
			{
				objects.FieldKeyCommitHash:   "hash2",
				objects.FieldKeyCommitDate:   now.AddDate(0, 0, -10).Format(time.RFC3339),
				objects.FieldKeyAuthor:       "bob",
				objects.FieldKeyLinesAdded:   int(30),
				objects.FieldKeyLinesRemoved: int(10),
			},
			{
				objects.FieldKeyCommitHash:   "hash3",
				objects.FieldKeyCommitDate:   now.AddDate(0, 0, -15).Format(time.RFC3339),
				objects.FieldKeyAuthor:       "alice",
				objects.FieldKeyLinesAdded:   float64(40),
				objects.FieldKeyLinesRemoved: float64(15),
			},
			{
				objects.FieldKeyCommitHash:   "hash4",
				objects.FieldKeyCommitDate:   now.AddDate(0, 0, -20).Format(time.RFC3339),
				objects.FieldKeyAuthor:       "charlie",
				objects.FieldKeyLinesAdded:   10,
				objects.FieldKeyLinesRemoved: 5,
			},
			{
				objects.FieldKeyCommitHash:   "hash5",
				objects.FieldKeyCommitDate:   now.AddDate(0, 0, -25).Format(time.RFC3339),
				objects.FieldKeyAuthor:       "bob",
				objects.FieldKeyLinesAdded:   10,
				objects.FieldKeyLinesRemoved: 5,
			},
		},
	}
	cMetrics, err := calculateCommitMetrics(ctx, storeCommits, secCtx, "proj-1")
	require.NoError(t, err)
	assert.Equal(t, 5, cMetrics.TotalCommits)
	assert.Equal(t, 3, len(cMetrics.Authors))
	assert.Equal(t, "stable", cMetrics.CommitPattern)
}

func TestTimeSeries_EdgeCasesAndCorrupt(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	cfg := TimeSeriesConfig{
		Dir:           tmpDir,
		Series:        "edge_test",
		ChunkDuration: time.Hour,
	}

	// 1. findSeriesChunks on non-existent dir
	cfgMissing := TimeSeriesConfig{Dir: filepath.Join(tmpDir, "missing")}
	chunks, err := findSeriesChunks(cfgMissing)
	assert.NoError(t, err)
	assert.Empty(t, chunks)

	// findSeriesChunks on regular file (not dir)
	fPath := filepath.Join(tmpDir, "regular_file.txt")
	require.NoError(t, fileutil.WriteFile(fPath, []byte("data"), paths.FilePerm644))
	cfgFile := TimeSeriesConfig{Dir: fPath}
	_, err = findSeriesChunks(cfgFile)
	assert.Error(t, err)

	// 2. QueryRangePoints with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	err = QueryRangePoints(canceledCtx, cfg, time.Time{}, time.Time{}, func(p TimeSeriesPoint) error { return nil })
	assert.Error(t, err)

	// 3. Rollup with invalid step and canceled context
	_, err = Rollup(ctx, cfg, time.Time{}, time.Time{}, 0)
	assert.Error(t, err)

	_, err = Rollup(canceledCtx, cfg, time.Time{}, time.Time{}, time.Minute)
	assert.Error(t, err)

	// Subdirectory and malformed chunk file in dir to test findSeriesChunks branches
	require.NoError(t, fileutil.Mkdir(filepath.Join(tmpDir, "subdir"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tmpDir, "edge_test_not_a_valid_date.chunk"), []byte("bad"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tmpDir, "other_series_20260101T0000.chunk"), []byte("other"), paths.FilePerm644))

	// 4. ListSeries on non-existent dir and regular file
	series, err := ListSeries(cfgMissing.Dir)
	assert.NoError(t, err)
	assert.Empty(t, series)

	_, err = ListSeries(fPath)
	assert.Error(t, err)

	// 5. AmbientFeed validation
	_, err = NewAmbientFeed(ctx, cfg, 0, time.Hour)
	assert.Error(t, err)
	_, err = NewAmbientFeed(ctx, cfg, 10, 0)
	assert.Error(t, err)

	// AmbientFeed empty snapshot
	feed, err := NewAmbientFeed(ctx, cfg, 10, time.Minute)
	require.NoError(t, err)
	snap, err := feed.Next()
	assert.NoError(t, err)
	assert.Equal(t, 0, snap.Count)

	// Write points to feed
	w, err := NewTimeSeriesWriter(cfg)
	require.NoError(t, err)
	now := time.Now().UTC()
	err = w.Append(TimeSeriesPoint{Ts: now, Value: 100})
	require.NoError(t, err)
	err = w.Append(TimeSeriesPoint{Ts: now.Add(time.Second), Value: 105})
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// QueryRangePoints with callback error (now that chunks exist)
	err = QueryRangePoints(ctx, cfg, time.Time{}, time.Time{}, func(p TimeSeriesPoint) error {
		return fmt.Errorf("callback error")
	})
	assert.Error(t, err)

	// QueryRangePoints skipping chunks before effectiveFrom and after effectiveTo
	futureFrom := time.Now().UTC().Add(24 * time.Hour)
	err = QueryRangePoints(ctx, cfg, futureFrom, futureFrom.Add(time.Hour), func(p TimeSeriesPoint) error {
		return nil
	})
	assert.NoError(t, err)

	pastTo := time.Now().UTC().Add(-24 * time.Hour)
	err = QueryRangePoints(ctx, cfg, pastTo.Add(-time.Hour), pastTo, func(p TimeSeriesPoint) error {
		return nil
	})
	assert.NoError(t, err)

	// Rollup over points with varying values (trigger min, max, sum updates)
	rollupRes, err := Rollup(ctx, cfg, time.Time{}, time.Time{}, time.Hour)
	assert.NoError(t, err)
	assert.NotEmpty(t, rollupRes.Buckets)

	// AmbientFeed Next with points
	snap2, err := feed.Next()
	assert.NoError(t, err)
	assert.Equal(t, 2, snap2.Count)

	// AmbientFeed with limit = 1 (truncates to tail)
	feedLimit1, err := NewAmbientFeed(ctx, cfg, 1, time.Minute)
	require.NoError(t, err)
	snapLimit1, err := feedLimit1.Next()
	assert.NoError(t, err)
	assert.Equal(t, 1, snapLimit1.Count)

	// 6. TimeSeriesWriter Close when file == nil and w == nil
	w2 := &TimeSeriesWriter{
		file: nil,
		w:    nil,
	}
	assert.NoError(t, w2.Close())
}
