package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// MetricsSnapshot is the interface that all metric snapshots must implement
type MetricsSnapshot interface {
	// GetTotalOperations returns the total number of operations for title generation
	GetTotalOperations() int64
}

// MetricsCollector is the interface for collecting metrics and creating metric objects
type MetricsCollector interface {
	// CollectMetrics creates a metric object from current metrics snapshot
	CollectMetrics(
		ctx context.Context,
		secCtx *pkgctx.SecurityContext,
		windowStart, windowEnd time.Time,
	) (string, error)

	// CollectAndReset collects metrics and resets the metrics counter
	CollectAndReset(
		ctx context.Context,
		secCtx *pkgctx.SecurityContext,
		windowStart, windowEnd time.Time,
	) (string, error)
}

// MetricBuilderConfig configures how to build a metric object
type MetricBuilderConfig struct {
	// Kind is the metric object kind (e.g., "file_lock_metric", "cas_metric")
	Kind string

	// TitlePrefix is the prefix for the metric title (e.g., "File Lock Metrics", "CAS Metrics")
	TitlePrefix string

	// MetricType is the metric_type field value (e.g., "performance", "system")
	MetricType string

	// Source is the source field value (e.g., "file_lock_system", "cas_system")
	Source string

	// Tags are the tags for the metric
	Tags []string

	// IDPrefix is the prefix for metric IDs (e.g., "FLM", "CASM")
	IDPrefix string

	// GetSnapshot is a function that returns the current metrics snapshot
	GetSnapshot func() MetricsSnapshot

	// ResetMetrics is a function that resets the metrics
	ResetMetrics func()

	// BuildMetricObject is a function that builds the metric object from snapshot
	// It receives the builder (already configured with common fields) and snapshot
	BuildMetricObject func(builder any, snapshot MetricsSnapshot, windowStart, windowEnd time.Time) error
}

// BaseMetricsCollector provides a reusable base for metrics collectors
type BaseMetricsCollector struct {
	storage ObjectStorageProvider
	config  MetricBuilderConfig
}

// NewBaseMetricsCollector creates a new base metrics collector
func NewBaseMetricsCollector(storage ObjectStorageProvider, config MetricBuilderConfig) *BaseMetricsCollector {
	return &BaseMetricsCollector{
		storage: storage,
		config:  config,
	}
}

// CollectMetrics creates a metric object from current metrics snapshot
func (c *BaseMetricsCollector) CollectMetrics(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	snapshot := c.config.GetSnapshot()

	// Generate title
	totalOps := snapshot.GetTotalOperations()
	title := fmt.Sprintf(ConstMiscSDOperationsFromSToS,
		c.config.TitlePrefix,
		totalOps,
		zqktime.FormatLayoutUTC(windowStart, zqktime.LayoutDateTimeSpace),
		zqktime.FormatLayoutUTC(windowEnd, zqktime.LayoutDateTimeSpace))

	now := time.Now().UTC()
	createdAt := now.Format(time.RFC3339)
	windowStartStr := zqktime.FormatRFC3339UTC(windowStart)
	windowEndStr := zqktime.FormatRFC3339UTC(windowEnd)

	// Get instance builder from registry
	schemaVersion, err := instance_builders.SchemaVersionForKind(c.config.Kind)
	if err != nil {
		return "", errfmt.Errorf(ConstMiscFailedToGetLatestSchemaVersionForSW, c.config.Kind, err)
	}

	// Create builder instance (this is type-specific, so we'll need a factory)
	// For now, we'll use a generic approach with SetField
	builder := createMetricBuilder(c.config.Kind, schemaVersion)
	if builder == nil {
		return "", errfmt.Errorf(ConstMiscUnsupportedMetricKindS, c.config.Kind)
	}

	// Generate ID
	metricID := c.generateMetricID(ctx, now)

	// Set common fields
	setCommonMetricFields(builder, metricID, title, c.config.MetricType, c.config.Source, c.config.Tags,
		totalOps, windowStartStr, windowEndStr, createdAt)

	if c.config.BuildMetricObject == nil {
		return "", errfmt.Errorf(ConstMiscFailedToBuildMetricObject)
	}
	if err := c.config.BuildMetricObject(builder, snapshot, windowStart, windowEnd); err != nil {
		return "", errfmt.Newf(ConstMiscFailedToBuildMetricObject).Wrap(err)
	}

	// Build the instance
	instance, err := buildMetricInstance(builder)
	if err != nil {
		return "", errfmt.Newf(ConstMiscFailedToBuildMetricInstance).Wrap(err)
	}

	// Persist the metric on the caller goroutine. The previous implementation still
	// blocked until storage.Create finished but spawned one metrics_collector_async
	// goroutine per collection, so parallel collectors could grow goroutine count
	// without bound when no process-wide goroutine budget is configured.
	release := func() {}
	if bud := goroutinelabels.DefaultBudget(); bud != nil {
		rel, err := bud.Reserve(1)
		if err != nil {
			return "", errfmt.Newf(ConstMiscMetricCreationGoroutineBudgetExceeded).Wrap(err)
		}
		release = rel
	}
	defer release()

	base := ctx
	if base == nil {
		base = context.Background() // Background: request-or-shutdown derived
	}
	createCtx, cancel := context.WithTimeout(base, 30*time.Second)
	defer cancel()

	secCtxCreate := pkgctx.NewSystemSecurityContext()
	if err := c.storage.Create(createCtx, secCtxCreate, instance); err != nil {
		return "", errfmt.Newf(ConstMiscFailedToCreateMetric).Wrap(err)
	}

	return extractMetricInstanceID(instance)
}

func extractMetricInstanceID(instance map[string]any) (string, error) {
	id, ok := instance[objects.FieldKeyID].(string)
	if !ok || id == "" {
		return "", errfmt.Errorf(ConstMiscMetricIdNotSetAfterCreation)
	}
	return id, nil
}

// CollectAndReset collects metrics and resets the metrics counter
func (c *BaseMetricsCollector) CollectAndReset(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	metricID, err := c.CollectMetrics(ctx, secCtx, windowStart, windowEnd)
	if err != nil {
		return "", err
	}

	// Reset metrics for next collection period
	c.config.ResetMetrics()

	return metricID, nil
}

// generateMetricID generates a unique ID for the metric
// Uses thread-safe batch generator (via generateID) for non-CAS kinds, consistent with other ID generation
// CAS-enabled kinds use timestamp-based IDs because CAS uses hash-based filenames
func (c *BaseMetricsCollector) generateMetricID(ctx context.Context, now time.Time) string {
	if fileStorage, ok := c.storage.(*FileObjectStorage); ok {
		if fileStorage.usesContentAddressableStorage(c.config.Kind) {
			// Use nanosecond-based timestamp ID for CAS-enabled kinds
			// CAS stores files with hash-based names, so sequence-based ID generation can't scan the directory
			return fmt.Sprintf("%s-%d", c.config.IDPrefix, now.UnixNano())
		}
		// Use thread-safe batch generator for non-CAS kinds (consistent with audit IDs and other sequential IDs)
		generatedID, err := fileStorage.generateID(ctx, c.config.Kind)
		if err != nil {
			// Fallback to timestamp-based ID if generation fails
			return fmt.Sprintf("%s-%d", c.config.IDPrefix, now.UnixNano())
		}
		return generatedID
	}
	// For graph storage or other backends, use timestamp-based ID
	return fmt.Sprintf("%s-%d", c.config.IDPrefix, now.UnixNano())
}

// createMetricBuilder creates a spec-backed builder for kind.
func createMetricBuilder(kind string, schemaVersion string) instance_builders.InstanceBuilder {
	if kind == "" {
		return nil
	}
	return instance_builders.NewForKind(kind, schemaVersion)
}

// setCommonMetricFields sets common fields on the builder
func setCommonMetricFields(builder instance_builders.InstanceBuilder, metricID, title, metricType, source string, tags []string,
	totalOps int64, windowStart, windowEnd, _ string) {
	if builder == nil {
		return
	}
	builder.SetID(metricID)
	builder.SetField(MetricFieldTitle, title).
		SetField(MetricFieldMetricType, metricType).
		SetField(MetricFieldSource, source).
		SetField(MetricFieldTags, tags).
		SetField(MetricFieldCollectionCount, totalOps).
		SetField(MetricFieldFirstSeen, windowStart).
		SetField(MetricFieldLastSeen, windowEnd)
}

// buildMetricInstance builds the instance from the builder
func buildMetricInstance(builder instance_builders.InstanceBuilder) (map[string]any, error) {
	if builder == nil {
		return nil, errfmt.Errorf(ConstMiscUnsupportedBuilderType)
	}
	return builder.Build()
}

// AsyncMetricsCollector provides async metrics collection for any MetricsCollector
type AsyncMetricsCollector struct {
	collector  MetricsCollector
	batchChan  chan *asyncMetricsBatch
	stopChan   chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	enabled    bool
	stopped    bool
	stopOnce   sync.Once // Ensure Wait() is only called once to prevent WaitGroup reuse
	bufferSize int
}

type asyncMetricsBatch struct {
	windowStart time.Time
	windowEnd   time.Time
	ctx         context.Context
	secCtx      *pkgctx.SecurityContext
	callback    func(string, error)
}

// NewAsyncMetricsCollector creates a new async metrics collector
// This is a generic wrapper that works with any MetricsCollector
func NewAsyncMetricsCollector(collector MetricsCollector, bufferSize int) *AsyncMetricsCollector {
	if bufferSize <= 0 {
		bufferSize = 100 // Default buffer size
	}

	asyncCollector := &AsyncMetricsCollector{
		collector:  collector,
		batchChan:  make(chan *asyncMetricsBatch, bufferSize),
		stopChan:   make(chan struct{}),
		enabled:    true,
		bufferSize: bufferSize,
	}

	// Start background worker
	asyncBud := goroutinelabels.DefaultBudget()
	asyncWorkerBuilder := goroutinelabels.NewGoroutine(ConstMiscMetricsFrameworkAsyncWorker, ConstMiscProcessingAsyncMetricsCollection).
		WithWaitGroup(&asyncCollector.wg)
	if asyncBud != nil {
		asyncWorkerBuilder = asyncWorkerBuilder.WithBudget(asyncBud)
	}
	asyncWorkerBuilder.StartSimple(asyncCollector.worker)

	return asyncCollector
}

// CollectMetricsAsync queues metrics collection to be processed asynchronously
func (c *AsyncMetricsCollector) CollectMetricsAsync(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
	callback func(metricID string, err error),
) error {
	return concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameAsyncMetricsCollectorEnqueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !c.enabled {
			return nil
		}
		select {
		case c.batchChan <- &asyncMetricsBatch{
			windowStart: windowStart,
			windowEnd:   windowEnd,
			ctx:         ctx,
			secCtx:      secCtx,
			callback:    callback,
		}:
			return nil
		default:
			return nil
		}
	})
}

// worker processes metrics batches in the background
// Note: wg.Done() is called by the goroutinelabels wrapper, so we don't call it here
func (c *AsyncMetricsCollector) worker() {
	for {
		select {
		case <-c.stopChan:
			// Process remaining batches before stopping
			for {
				select {
				case batch := <-c.batchChan:
					c.processAsyncBatch(batch)
				default:
					return
				}
			}
		case batch := <-c.batchChan:
			c.processAsyncBatch(batch)
		}
	}
}

// processAsyncBatch processes a single metrics batch
func (c *AsyncMetricsCollector) processAsyncBatch(batch *asyncMetricsBatch) {
	// Call the collector's CollectAndReset method
	metricID, err := c.collector.CollectAndReset(batch.ctx, batch.secCtx, batch.windowStart, batch.windowEnd)

	// Invoke callback if provided
	if batch.callback != nil {
		batch.callback(metricID, err)
	}
}

// Stop stops the async collector and processes remaining batches
func (c *AsyncMetricsCollector) Stop() {
	// Use sync.Once to ensure we only stop and wait once (prevents WaitGroup reuse)
	c.stopOnce.Do(func() {
		_ = concurrency.RunInLockOrLog(&c.mu, locknames.LockNameAsyncMetricsCollectorStop, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			if !c.stopped {
				c.stopped = true
				c.enabled = false
				close(c.stopChan)
			}
			return nil
		})

		// Wait for worker to finish (only called once due to stopOnce)
		c.wg.Wait()
	})
}

// Enable enables async collection
func (c *AsyncMetricsCollector) Enable() {
	_ = concurrency.RunInLockOrLog(&c.mu, locknames.LockNameAsyncMetricsCollectorEnable, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		c.enabled = true
		return nil
	})
}

// Disable disables async collection (drops batches)
func (c *AsyncMetricsCollector) Disable() {
	_ = concurrency.RunInLockOrLog(&c.mu, locknames.LockNameAsyncMetricsCollectorDisable, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		c.enabled = false
		return nil
	})
}
