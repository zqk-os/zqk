package metrics

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// Metric object kind constants
const (
	// MetricKindBaseMetric is the kind for base_metric objects
	MetricKindBaseMetric = objects.KindBaseMetric
)

// Metric type constants
const (
	// MetricTypeSystem is the metric_type value for system metrics
	MetricTypeSystem = "system"
	// MetricTypePerformance is the metric_type value for performance metrics
	MetricTypePerformance = "performance"

	// MetricTypeScalar is the metric_type / trait name for scalar metric fields and aggregators.
	MetricTypeScalar = "scalar_metric"
	// MetricTypeList is the metric_type / trait name for list metric fields and aggregators.
	MetricTypeList = "list_metric"
	// MetricTypeOrderedList is the metric_type / trait name for ordered list metric fields and aggregators.
	MetricTypeOrderedList = "ordered_list_metric"
	// MetricTypeStatusHistory is the metric_type / trait name for status history metric fields and aggregators.
	MetricTypeStatusHistory = "status_history_metric"
)

// Metric status constants
const (
	// MetricStatusImplemented is the status value for implemented metrics
	MetricStatusImplemented = "implemented"
)

// Metric schema version constants
// Note: Uses objects.DefaultSchemaVersion to avoid duplication
var (
	// MetricSchemaVersion is the schema version for metric objects
	// Uses objects.DefaultSchemaVersion to ensure consistency
	MetricSchemaVersion = objects.DefaultSchemaVersion
)

// Metric source constants
const (
	// MetricSourcePipeline is the source value for metrics pipeline
	MetricSourcePipeline = "metrics_pipeline"
	// MetricSourcePipelineSampler is the source value for sampled metrics pipeline
	MetricSourcePipelineSampler = "metrics_pipeline_sampler"
	// MetricSourceAsyncValidation is the source for async system check validation metrics persisted to base_metric.
	MetricSourceAsyncValidation = "async_validation"
	// MetricSourceGraphProvider is the source for pkg/graph/provider DefaultMetricsCollector snapshots.
	MetricSourceGraphProvider = "graph_provider"
	// MetricSourceCASValidationCache is the source for CAS index async cache + per-kind validation metrics snapshots.
	MetricSourceCASValidationCache = "cas_validation_cache"
)

// Metric account constants
// Note: Uses pkgctx.SystemAccountID to avoid duplication
var (
	// MetricCreatedBySystem is the created_by value for system-created metrics
	// Uses pkgctx.SystemAccountID to ensure consistency
	MetricCreatedBySystem = pkgctx.SystemAccountID
)

// Metric tag constants
const (
	// MetricTagSystem is the "system" tag used in metric tags
	MetricTagSystem = "system"
	// MetricTagPipeline is the "metrics_pipeline" tag used in metric tags
	MetricTagPipeline = "metrics_pipeline"
	// MetricTagSampled is the "sampled" tag used in sampled metrics
	MetricTagSampled = "sampled"
)

// Metric origin constants (replaces dependency on validation package)
const (
	// DefaultMetricNamespace is the default namespace for metrics
	// This matches validation.DefaultNamespaceKernel but avoids import dependency
	DefaultMetricNamespace = "zqk:kernel"
	// DefaultMetricOriginSystem is the default origin system for metrics
	// This matches validation.DefaultOriginSystem but avoids import dependency
	DefaultMetricOriginSystem = "zqk"
	// DefaultMetricOriginProject is the default origin project for metrics
	// This matches validation.DefaultOriginProject but avoids import dependency
	DefaultMetricOriginProject = "zqk"
)
