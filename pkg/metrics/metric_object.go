package metrics

import (
	"context"
	"sort"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// MetricObjectConfig configures how a metric object is created
type MetricObjectConfig struct {
	// Source is the metric source (e.g., MetricSourcePipeline, MetricSourcePipelineSampler)
	Source string
	// Sampled indicates if this is a sampled metric
	Sampled bool
	// AdditionalTags are extra tags to include beyond the standard tags
	AdditionalTags []string
	// NamespaceID overrides the default namespace (optional)
	NamespaceID string
	// OriginSystem overrides the default origin system (optional)
	OriginSystem string
	// OriginProject overrides the default origin project (optional)
	OriginProject string
}

// DefaultMetricObjectConfig returns default configuration for metric objects
func DefaultMetricObjectConfig() *MetricObjectConfig {
	return &MetricObjectConfig{
		Source:         MetricSourcePipeline,
		Sampled:        false,
		AdditionalTags: nil,
		NamespaceID:    DefaultMetricNamespace,
		OriginSystem:   DefaultMetricOriginSystem,
		OriginProject:  DefaultMetricOriginProject,
	}
}

// CreateBaseMetricObject creates the base structure for a metric object
// This centralizes the common fields and eliminates duplication across aggregators
func CreateBaseMetricObject(
	config *AggregationConfig,
	title string,
	eventCount int,
	objConfig *MetricObjectConfig,
) map[string]any {
	if objConfig == nil {
		objConfig = DefaultMetricObjectConfig()
	}

	// Build tags
	tags := []string{
		MetricTagSystem,
		MetricTagPipeline,
	}
	if objConfig.Sampled {
		tags = append(tags, MetricTagSampled)
	}
	if config.MetricType != emptyValue {
		tags = append(tags, config.MetricType)
	}
	if config.ObjectKind != emptyValue {
		tags = append(tags, config.ObjectKind)
	}
	if config.FieldName != emptyValue {
		tags = append(tags, config.FieldName)
	}
	tags = append(tags, objConfig.AdditionalTags...)

	// Set namespace and origin (use defaults if not overridden)
	namespaceID := objConfig.NamespaceID
	if namespaceID == emptyValue {
		namespaceID = DefaultMetricNamespace
	}
	originSystem := objConfig.OriginSystem
	if originSystem == emptyValue {
		originSystem = DefaultMetricOriginSystem
	}
	originProject := objConfig.OriginProject
	if originProject == emptyValue {
		originProject = DefaultMetricOriginProject
	}

	now := time.Now().UTC()

	metricObj := map[string]any{
		objects.FieldKeyKind:            MetricKindBaseMetric,
		objects.FieldKeyTitle:           title,
		objects.FieldKeyMetricType:      MetricTypeSystem,
		objects.FieldKeySource:          objConfig.Source,
		objects.FieldKeyTags:            tags,
		objects.FieldKeyCollectionCount: eventCount,
		objects.FieldKeyFirstSeen:       config.WindowStart.Format(time.RFC3339),
		objects.FieldKeyLastSeen:        config.WindowEnd.Format(time.RFC3339),
		objects.FieldKeyCreatedAt:       now.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:       MetricCreatedBySystem,
		objects.FieldKeyStatus:          MetricStatusImplemented,
		objects.FieldKeyNamespaceID:     namespaceID,
		objects.FieldKeyOriginProject:   originProject,
		objects.FieldKeyOriginSystem:    originSystem,
		objects.FieldKeySchemaVersion:   MetricSchemaVersion,

		// Aggregation metadata
		objects.FieldKeyObjectKind:         config.ObjectKind,
		objects.FieldKeyFieldName:          config.FieldName,
		objects.FieldKeyMetricTypeSpecific: config.MetricType,
		objects.FieldKeyObjectCount:        eventCount,
		objects.FieldKeyWindowStart:        config.WindowStart.Format(time.RFC3339),
		objects.FieldKeyWindowEnd:          config.WindowEnd.Format(time.RFC3339),
	}

	// Add sampled flag if this is a sampled metric
	if objConfig.Sampled {
		metricObj[objects.FieldKeySampled] = true
	}

	return metricObj
}

// QueryAggregationObjects queries storage for objects matching the aggregation config filter.
func QueryAggregationObjects(ctx context.Context, st storage.ObjectStorageProvider, config *AggregationConfig) ([]map[string]any, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	filter := storage.ListFilter{
		Kind:    config.ObjectKind,
		Filters: config.Filters,
		Limit:   0,
	}

	result, err := st.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to list objects").Wrap(err)
	}
	return result.Objects, nil
}

// BuildAggregationResult builds standard AggregationResult structure from config and aggregations.
func BuildAggregationResult(config *AggregationConfig, metricID string, aggregations map[string]any, objectCount int) *AggregationResult {
	return &AggregationResult{
		MetricID:        metricID,
		ObjectKind:      config.ObjectKind,
		FieldName:       config.FieldName,
		MetricType:      config.MetricType,
		WindowStart:     config.WindowStart,
		WindowEnd:       config.WindowEnd,
		Aggregations:    aggregations,
		ObjectCount:     objectCount,
		CollectionCount: 1,
	}
}

// CreateSyncMetricObject creates a metric object via factory with a 30s timeout.
func CreateSyncMetricObject(
	ctx context.Context,
	st storage.ObjectStorageProvider,
	config *AggregationConfig,
	title string,
	objConfig *MetricObjectConfig,
	aggregations map[string]any,
	objectCount int,
) (string, error) {
	factory := NewMetricFactory(st)
	metricIDChan := make(chan string, 1)
	errChan := make(chan error, 1)

	factory.CreateMetricFromConfigAsync(
		ctx,
		config,
		title,
		objectCount,
		objConfig,
		aggregations,
		func(metricID string, err error) {
			if err != nil {
				errChan <- err
				return
			}
			metricIDChan <- metricID
		},
	)

	select {
	case id := <-metricIDChan:
		return id, nil
	case err := <-errChan:
		return "", err
	case <-time.After(30 * time.Second):
		return "", errfmt.Errorf("metric creation timeout after 30s")
	}
}

// TopNMap takes a map of string counts and returns the top N entries as a map.
func TopNMap(counts map[string]int, n int) map[string]int {
	type pair struct {
		key   string
		value int
	}
	pairs := make([]pair, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, pair{key: k, value: v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].value > pairs[j].value
	})
	topN := make(map[string]int)
	for i := 0; i < n && i < len(pairs); i++ {
		topN[pairs[i].key] = pairs[i].value
	}
	return topN
}

// ShouldFlushMetrics validates storage provider and recording status, returning active logger or false.
func ShouldFlushMetrics(sp storage.ObjectStorageProvider, logger logging.Logger) (logging.Logger, bool) {
	if sp == nil || !metricsrecording.Enabled() {
		return nil, false
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return logger, true
}

// PersistBaseMetricInstance persists a base_metric instance to storage provider with a 60-second timeout.
func PersistBaseMetricInstance(sp storage.ObjectStorageProvider, inst map[string]any, metricID, desc string, logger logging.Logger) {
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 60*time.Second)
	defer cancel()
	secCtx := pkgctx.NewSystemSecurityContext()
	if createErr := sp.Create(ctx, secCtx, inst); createErr != nil {
		if logger != nil {
			logging.Fluent(logger).Warn("Failed to persist " + desc).
				MetricID(metricID).
				WithError(createErr).
				Log()
		}
	}
}

// FormatMetricWindow formats start and end times to RFC3339 UTC strings, returning error if metricID is empty.
func FormatMetricWindow(metricID string, start, end time.Time) (string, string, error) {
	if metricID == emptyValue {
		return "", "", errfmt.Errorf("metric id is required")
	}
	return zqktime.FormatRFC3339UTC(start), zqktime.FormatRFC3339UTC(end), nil
}

// ComputeEffectiveObjectCount ensures non-negative object count with fallback when zero.
func ComputeEffectiveObjectCount(total int64, fallback int) int {
	oc := int(total)
	if oc < 0 {
		return 0
	}
	if oc == 0 {
		return fallback
	}
	return oc
}
