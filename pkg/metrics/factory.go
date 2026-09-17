package metrics

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/objects"
	commandMetricEnum "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/command_metric"
	fileLockMetricEnum "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/file_lock_metric"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
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
	registry        *instance_builders.VersionedInstanceBuilderRegistry
}

// NewMetricFactory creates a new metric factory
func NewMetricFactory(storageProvider storage.ObjectStorageProvider) *MetricFactory {
	registry := instance_builders.GetGlobalRegistry()
	return &MetricFactory{
		storageProvider: storageProvider,
		registry:        registry,
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
	// Get schema version from registry
	schemaVersion, err := f.registry.GetLatestVersion(req.Kind)
	if err != nil {
		// Fallback to default schema version
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

// createBuilder creates the appropriate builder for a metric kind
func (f *MetricFactory) createBuilder(kind, schemaVersion string) any {
	switch kind {
	case kindFileLockMetric:
		return bldr_instance_v1.NewFileLockMetricInstanceBuilder(schemaVersion)
	case kindCommandMetric:
		return bldr_instance_v1.NewCommandMetricInstanceBuilder(schemaVersion)
	default:
		return nil
	}
}

// setCommonFields sets common fields on the builder
func (f *MetricFactory) setCommonFields(builder any, req *MetricCreationRequest) {
	// Generate ID (simplified - in production would use proper ID generation)
	metricID := fmt.Sprintf(metricIDFmt, time.Now().UnixNano())

	// Use type assertion to set fields based on builder type
	switch b := builder.(type) {
	case *bldr_instance_v1.FileLockMetricInstanceBuilder:
		b.ID(metricID).
			Title(req.Title).
			MetricType(fileLockMetricEnum.MetricType(req.MetricType)).
			Source(req.Source).
			Tags(req.Tags).
			CollectionCount(req.EventCount).
			FirstSeen(zqktime.FormatRFC3339UTC(req.WindowStart)).
			LastSeen(zqktime.FormatRFC3339UTC(req.WindowEnd))
		// Set metric-specific fields
		for k, v := range req.Fields {
			b.SetField(k, v)
		}
	case *bldr_instance_v1.CommandMetricInstanceBuilder:
		b.ID(metricID).
			Title(req.Title).
			MetricType(commandMetricEnum.MetricType(req.MetricType)).
			Source(req.Source).
			Tags(req.Tags).
			CollectionCount(req.EventCount).
			FirstSeen(zqktime.FormatRFC3339UTC(req.WindowStart)).
			LastSeen(zqktime.FormatRFC3339UTC(req.WindowEnd))
		// Set metric-specific fields
		for k, v := range req.Fields {
			b.SetField(k, v)
		}
	}
}

// buildInstance builds the instance from the builder
func (f *MetricFactory) buildInstance(builder any) (map[string]any, error) {
	type buildable interface {
		Build() (map[string]any, error)
	}
	b, ok := builder.(buildable)
	if !ok {
		return nil, errors.New(errUnsupportedBuilderType)
	}
	return b.Build()
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
