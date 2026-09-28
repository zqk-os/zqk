package metrics

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	errUnsupportedMetricKindFmt     = "unsupported metric kind: %s"
	errBuildMetricInstanceFmt       = "failed to build metric instance: %w"
	errCreateMetricFmt              = "failed to create metric: %w"
	errMetricIDMissingAfterCreate   = "metric ID not set after creation"
	errUnsupportedBuilderType       = "unsupported builder type"
	metricIDFmt                     = "MET-%d"
	kindFileLockMetric              = objects.KindFileLockMetric
	kindCommandMetric               = objects.KindCommandMetric
	goroutineMetricFactoryAsync     = "metric_factory_async"
	goroutineMetricFactoryAsyncCfg  = "metric_factory_async_config"
	goroutineMetricFactoryAsyncInst = "metric_factory_async_instance"
	goroutineMetricCreateFmt        = "creating metric: %s"
	goroutineMetricFromCfgFmt       = "creating metric from config: %s"
	goroutineMetricFromInstFmt      = "creating metric from instance: %s"
	logWarnMetricAsyncCreateFailed  = "Failed to create metric asynchronously"
	logWarnMetricCfgCreateFailed    = "Failed to create metric from config asynchronously"
	logWarnMetricInstCreateFailed   = "Failed to create metric from instance asynchronously"
	logFieldKind                    = "kind"
	logFieldTitle                   = "title"
	logFieldObjectKind              = "object_kind"
)

// MetricFactory provides a standardized factory pattern for creating metric objects
// This ensures consistency across all metric creation points
type MetricFactory struct {
	storageProvider storage.ObjectStorageProvider
}

// NewMetricFactory creates a new metric factory
func NewMetricFactory(storageProvider storage.ObjectStorageProvider) *MetricFactory {
	return &MetricFactory{
		storageProvider: storageProvider,
	}
}

// MetricCreationRequest represents a request to create a metric
type MetricCreationRequest struct {
	Kind          string              // Metric kind (e.g., "file_lock_metric", "command_metric")
	Title         string              // Metric title
	MetricType    string              // Metric type (e.g., MetricTypeSystem, MetricTypePerformance)
	Source        string              // Metric source (e.g., MetricSourcePipeline)
	Tags          []string            // Metric tags
	Fields        map[string]any      // Metric-specific fields
	WindowStart   time.Time           // Measurement window start
	WindowEnd     time.Time           // Measurement window end
	EventCount    int                 // Number of events/operations
	AdditionalCfg *MetricObjectConfig // Additional configuration (optional)
}

// CreateMetric creates a metric object using the factory pattern
// This standardizes metric creation and ensures all metrics use consistent patterns
func (f *MetricFactory) CreateMetric(ctx context.Context, req *MetricCreationRequest) (string, error) {
	if !metricsrecording.EnabledForKind(req.Kind) {
		return "", nil
	}
	schemaVersion, err := instance_builders.SchemaVersionForKind(req.Kind)
	if err != nil {
		schemaVersion = objects.DefaultSchemaVersion
	}

	// Create builder using factory
	builder := f.createBuilder(req.Kind, schemaVersion)
	if builder == nil {
		return "", errfmt.Errorf(errUnsupportedMetricKindFmt, req.Kind)
	}

	// Set common fields using factory pattern
	f.setCommonFields(builder, req)

	// Build instance
	instance, err := f.buildInstance(builder)
	if err != nil {
		return "", errfmt.Errorf(errBuildMetricInstanceFmt, err)
	}

	// Create via storage
	secCtx := pkgctx.NewSystemSecurityContext()
	if err := f.storageProvider.Create(ctx, secCtx, instance); err != nil {
		return "", errfmt.Errorf(errCreateMetricFmt, err)
	}

	// Return ID
	id, ok := instance[objects.FieldKeyID].(string)
	if !ok {
		return "", errors.New(errMetricIDMissingAfterCreate)
	}

	return id, nil
}

// CreateMetricAsync creates a metric object asynchronously (non-blocking)
// The callback is invoked with the metric ID and any error that occurred
// This ensures metrics creation never blocks the calling goroutine
func (f *MetricFactory) CreateMetricAsync(
	ctx context.Context,
	req *MetricCreationRequest,
	callback func(metricID string, err error),
) {
	// Execute metric creation in a goroutine (fire-and-forget pattern)
	goroutinelabels.NewGoroutine(goroutineMetricFactoryAsync, fmt.Sprintf(goroutineMetricCreateFmt, req.Kind)).
		StartWithContext(ctx, func(ctx context.Context) error {
			if !metricsrecording.EnabledForKind(req.Kind) {
				if callback != nil {
					callback("", nil)
				}
				return nil
			}
			metricID, err := f.CreateMetric(ctx, req)
			if callback != nil {
				callback(metricID, err)
			} else if err != nil {
				// Log error if no callback provided
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Warn(logWarnMetricAsyncCreateFailed).
					Kind(req.Kind).
					String(logFieldTitle, req.Title).
					WithError(err).
					Log()
			}
			return nil
		})
}

// CreateMetricFromConfigAsync creates a metric asynchronously using CreateBaseMetricObject pattern
// This is a convenience method for existing code that uses CreateBaseMetricObject
func (f *MetricFactory) CreateMetricFromConfigAsync(
	ctx context.Context,
	config *AggregationConfig,
	title string,
	eventCount int,
	objConfig *MetricObjectConfig,
	additionalFields map[string]any,
	callback func(metricID string, err error),
) {
	// Execute metric creation in a goroutine (fire-and-forget pattern)
	goroutinelabels.NewGoroutine(goroutineMetricFactoryAsyncCfg, fmt.Sprintf(goroutineMetricFromCfgFmt, config.ObjectKind)).
		StartWithContext(ctx, func(ctx context.Context) error {
			if !metricsrecording.EnabledForKind(config.ObjectKind) {
				if callback != nil {
					callback("", nil)
				}
				return nil
			}
			metricID, err := f.CreateMetricFromConfig(ctx, config, title, eventCount, objConfig, additionalFields)
			if callback != nil {
				callback(metricID, err)
			} else if err != nil {
				// Log error if no callback provided
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Warn(logWarnMetricCfgCreateFailed).
					String(logFieldObjectKind, config.ObjectKind).
					String(logFieldTitle, title).
					WithError(err).
					Log()
			}
			return nil
		})
}

// CreateMetricFromInstanceAsync creates a metric asynchronously from a pre-built instance
// This is used by BaseMetricsCollector which builds instances using instance builders
// The instance should already have all fields set, including ID
func (f *MetricFactory) CreateMetricFromInstanceAsync(
	ctx context.Context,
	instance map[string]any,
	callback func(metricID string, err error),
) {
	// Execute metric creation in a goroutine (fire-and-forget pattern)
	goroutinelabels.NewGoroutine(goroutineMetricFactoryAsyncInst, fmt.Sprintf(goroutineMetricFromInstFmt, instance[objects.FieldKeyKind])).
		StartWithContext(ctx, func(ctx context.Context) error {
			kind, _ := instance[objects.FieldKeyKind].(string)
			if !metricsrecording.EnabledForKind(kind) {
				if callback != nil {
					callback("", nil)
				}
				return nil
			}
			secCtx := pkgctx.NewSystemSecurityContext()
			if err := f.storageProvider.Create(ctx, secCtx, instance); err != nil {
				if callback != nil {
					callback("", errfmt.Errorf(errCreateMetricFmt, err))
				} else {
					logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
					logging.Fluent(logger).Warn(logWarnMetricInstCreateFailed).
						String(logFieldKind, fmt.Sprintf("%v", instance[objects.FieldKeyKind])).
						WithError(err).
						Log()
				}
				return nil
			}

			// Return ID
			id, ok := instance[objects.FieldKeyID].(string)
			if !ok {
				err := errors.New(errMetricIDMissingAfterCreate)
				if callback != nil {
					callback("", err)
				}
				return nil
			}

			if callback != nil {
				callback(id, nil)
			}
			return nil
		})
}

// createBuilder returns a spec-backed builder for kind.
func (f *MetricFactory) createBuilder(kind, schemaVersion string) instance_builders.InstanceBuilder {
	if kind == "" {
		return nil
	}
	return instance_builders.NewForKind(kind, schemaVersion)
}

// setCommonFields sets common fields on the builder
func (f *MetricFactory) setCommonFields(builder instance_builders.InstanceBuilder, req *MetricCreationRequest) {
	if builder == nil || req == nil {
		return
	}
	metricID := fmt.Sprintf(metricIDFmt, time.Now().UnixNano())
	windowStart := zqktime.FormatRFC3339UTC(req.WindowStart)
	windowEnd := zqktime.FormatRFC3339UTC(req.WindowEnd)
	builder.SetID(metricID).
		SetField(objects.FieldKeyTitle, req.Title).
		SetField(objects.FieldKeyMetricType, req.MetricType).
		SetField(objects.FieldKeySource, req.Source).
		SetField(objects.FieldKeyTags, req.Tags).
		SetField(objects.FieldKeyCollectionCount, req.EventCount).
		SetField(objects.FieldKeyFirstSeen, windowStart).
		SetField(objects.FieldKeyLastSeen, windowEnd)
	for k, v := range req.Fields {
		builder.SetField(k, v)
	}
}

// buildInstance builds the instance from the builder
func (f *MetricFactory) buildInstance(builder instance_builders.InstanceBuilder) (map[string]any, error) {
	if builder == nil {
		return nil, errors.New(errUnsupportedBuilderType)
	}
	return builder.Build()
}

// CreateMetricFromConfig creates a metric using CreateBaseMetricObject pattern
// This is a convenience method for existing code that uses CreateBaseMetricObject
func (f *MetricFactory) CreateMetricFromConfig(
	ctx context.Context,
	config *AggregationConfig,
	title string,
	eventCount int,
	objConfig *MetricObjectConfig,
	additionalFields map[string]any,
) (string, error) {
	if !metricsrecording.EnabledForKind(config.ObjectKind) {
		return "", nil
	}
	// Use existing CreateBaseMetricObject for consistency
	metricObj := CreateBaseMetricObject(config, title, eventCount, objConfig)

	// Add additional fields
	maps.Copy(metricObj, additionalFields)

	// Create via storage
	secCtx := pkgctx.NewSystemSecurityContext()
	if err := f.storageProvider.Create(ctx, secCtx, metricObj); err != nil {
		return "", errfmt.Errorf(errCreateMetricFmt, err)
	}

	// Return ID
	id, ok := metricObj[objects.FieldKeyID].(string)
	if !ok {
		return "", errors.New(errMetricIDMissingAfterCreate)
	}

	return id, nil
}
