package metrics

import (
	"time"

	"github.com/lanceman/zqk/pkg/objects"
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
