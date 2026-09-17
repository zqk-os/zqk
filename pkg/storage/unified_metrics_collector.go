package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// MetricsEventEmitter is an interface for emitting metric collection events
// This avoids import cycles by using dependency injection
type MetricsEventEmitter interface {
	// EmitMetricCollected emits an event when metrics are collected
	EmitMetricCollected(ctx context.Context, metricType, metricID string, windowStart, windowEnd time.Time, err error)
}

// UnifiedMetricsCollector bundles all metric collectors and emits via coordinator
// This provides a single component that manages all metrics collection and routing
type UnifiedMetricsCollector struct {
	storage      ObjectStorageProvider
	eventEmitter MetricsEventEmitter // Optional event emitter (avoids import cycle)
	tsdb         TSDBProvider        // Optional TSDB provider for time-series persistence

	// Individual collectors
	casCollector      MetricsCollector
	fileLockCollector MetricsCollector
	auditCollector    MetricsCollector

	// Async collectors (for high-volume scenarios)
	casAsyncCollector      *AsyncMetricsCollector
	fileLockAsyncCollector *AsyncMetricsCollector

	mu sync.RWMutex
}

// UnifiedMetricsCollectorConfig configures the unified metrics collector
type UnifiedMetricsCollectorConfig struct {
	Storage      ObjectStorageProvider
	EventEmitter MetricsEventEmitter // Optional - if nil, events are not emitted
	TSDB         TSDBProvider        // Optional - if set, metrics are persisted to TSDB

	// Enable async collection for high-volume metrics
	EnableAsyncCAS      bool
	EnableAsyncFileLock bool

	// Buffer sizes for async collectors
	AsyncBufferSize int
}

// NewUnifiedMetricsCollector creates a new unified metrics collector
func NewUnifiedMetricsCollector(config UnifiedMetricsCollectorConfig) *UnifiedMetricsCollector {
	if config.AsyncBufferSize <= 0 {
		config.AsyncBufferSize = 100 // Default buffer size
	}

	collector := &UnifiedMetricsCollector{
		storage:      config.Storage,
		eventEmitter: config.EventEmitter,
		tsdb:         config.TSDB,
	}

	// Initialize individual collectors
	collector.casCollector = caspkg.NewObjectStorageMetricsCollector(config.Storage)
	collector.fileLockCollector = NewFileLockMetricsCollector(config.Storage)
	// Use NewAuditMetricsCollector to ensure storage is set (global one may not have storage)
	collector.auditCollector = NewAuditMetricsCollector(config.Storage)

	// Initialize async collectors if enabled
	if config.EnableAsyncCAS {
		collector.casAsyncCollector = NewAsyncMetricsCollector(collector.casCollector, config.AsyncBufferSize)
	}
	if config.EnableAsyncFileLock {
		collector.fileLockAsyncCollector = NewAsyncMetricsCollector(collector.fileLockCollector, config.AsyncBufferSize)
	}

	return collector
}

// CollectAllMetrics collects metrics from all collectors and emits via coordinator
// This is the main entry point for periodic metric collection (e.g., hourly, daily)
func (c *UnifiedMetricsCollector) CollectAllMetrics(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) ([]string, error) {
	var metricIDs []string
	var errors []error

	// Collect CAS metrics
	if c.casAsyncCollector != nil {
		// Use async collector for high-volume CAS metrics
		err := c.casAsyncCollector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd, func(metricID string, err error) {
			if err == nil {
				metricIDs = append(metricIDs, metricID)
				c.emitMetricCollectedEvent(ctx, "cas_metric", metricID, windowStart, windowEnd, nil)
			} else {
				errors = append(errors, err)
				c.emitMetricCollectedEvent(ctx, "cas_metric", "", windowStart, windowEnd, err)
			}
		})
		if err != nil {
			errors = append(errors, errfmt.Newf(ConstMiscFailedToQueueCasMetricsCollection).Wrap(err))
		}
	} else {
		// Use synchronous collector
		metricID, err := c.casCollector.CollectAndReset(ctx, secCtx, windowStart, windowEnd)
		if err != nil {
			errors = append(errors, errfmt.Newf(ConstMiscFailedToCollectCasMetrics).Wrap(err))
			c.emitMetricCollectedEvent(ctx, "cas_metric", "", windowStart, windowEnd, err)
		} else {
			metricIDs = append(metricIDs, metricID)
			c.emitMetricCollectedEvent(ctx, "cas_metric", metricID, windowStart, windowEnd, nil)
		}
	}

	// Collect file lock metrics
	if c.fileLockAsyncCollector != nil {
		// Use async collector for high-volume file lock metrics
		err := c.fileLockAsyncCollector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd, func(metricID string, err error) {
			if err == nil {
				metricIDs = append(metricIDs, metricID)
				c.emitMetricCollectedEvent(ctx, objects.KindFileLockMetric, metricID, windowStart, windowEnd, nil)
			} else {
				errors = append(errors, err)
				c.emitMetricCollectedEvent(ctx, objects.KindFileLockMetric, "", windowStart, windowEnd, err)
			}
		})
		if err != nil {
			errors = append(errors, errfmt.Newf(ConstMiscFailedToQueueFileLockMetricsCollection).Wrap(err))
		}
	} else {
		// Use synchronous collector
		metricID, err := c.fileLockCollector.CollectAndReset(ctx, secCtx, windowStart, windowEnd)
		if err != nil {
			errors = append(errors, errfmt.Newf(ConstMiscFailedToCollectFileLockMetrics).Wrap(err))
			c.emitMetricCollectedEvent(ctx, objects.KindFileLockMetric, "", windowStart, windowEnd, err)
		} else {
			metricIDs = append(metricIDs, metricID)
			c.emitMetricCollectedEvent(ctx, objects.KindFileLockMetric, metricID, windowStart, windowEnd, nil)
		}
	}

	// Collect audit metrics (typically lower volume, synchronous is fine)
	metricID, err := c.auditCollector.CollectAndReset(ctx, secCtx, windowStart, windowEnd)
	if err != nil {
		errors = append(errors, errfmt.Newf(ConstMiscFailedToCollectAuditMetrics).Wrap(err))
		c.emitMetricCollectedEvent(ctx, "audit_metric", "", windowStart, windowEnd, err)
	} else {
		metricIDs = append(metricIDs, metricID)
		c.emitMetricCollectedEvent(ctx, "audit_metric", metricID, windowStart, windowEnd, nil)
	}

	// Return combined results
	if len(errors) > 0 {
		return metricIDs, errfmt.Errorf(ConstMiscSomeMetricsCollectionFailedV, errors)
	}

	return metricIDs, nil
}

// CollectMetricsByType collects metrics for a specific type
func (c *UnifiedMetricsCollector) CollectMetricsByType(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	metricType string,
	windowStart, windowEnd time.Time,
) (string, error) {
	var collector MetricsCollector
	var asyncCollector *AsyncMetricsCollector

	switch metricType {
	case "cas_metric", "cas":
		collector = c.casCollector
		asyncCollector = c.casAsyncCollector
	case objects.KindFileLockMetric, "file_lock":
		collector = c.fileLockCollector
		asyncCollector = c.fileLockAsyncCollector
	case "audit_metric", "audit":
		collector = c.auditCollector
	default:
		return "", errfmt.Errorf(ConstMiscUnknownMetricTypeS, metricType)
	}

	if asyncCollector != nil {
		var resultID string
		var resultErr error
		done := make(chan struct{})

		err := asyncCollector.CollectMetricsAsync(ctx, secCtx, windowStart, windowEnd, func(metricID string, err error) {
			resultID = metricID
			resultErr = err
			c.emitMetricCollectedEvent(ctx, metricType, metricID, windowStart, windowEnd, err)
			close(done)
		})
		if err != nil {
			return "", errfmt.Newf(ConstMiscFailedToQueueMetricsCollection).Wrap(err)
		}

		// Wait for async collection to complete
		select {
		case <-done:
			return resultID, resultErr
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(30 * time.Second):
			return "", errfmt.Errorf(ConstMiscMetricsCollectionTimeoutAfter30s)
		}
	}

	metricID, err := collector.CollectAndReset(ctx, secCtx, windowStart, windowEnd)
	c.emitMetricCollectedEvent(ctx, metricType, metricID, windowStart, windowEnd, err)
	return metricID, err
}

// emitMetricCollectedEvent emits a metric collection event via event emitter and persists to TSDB if configured
func (c *UnifiedMetricsCollector) emitMetricCollectedEvent(
	ctx context.Context,
	metricType, metricID string,
	windowStart, windowEnd time.Time,
	err error,
) {
	if c.eventEmitter != nil {
		c.eventEmitter.EmitMetricCollected(ctx, metricType, metricID, windowStart, windowEnd, err)
	}

	if err == nil && c.tsdb != nil && metricID != "" {
		// Attempt to read the fully built metric object from storage so we can forward its fields
		secCtx := pkgctx.NewSystemSecurityContext()
		obj, readErr := c.storage.Read(ctx, secCtx, metricID)
		if readErr == nil {
			pt := TSDBPoint{
				Measurement: metricType,
				Tags: map[string]string{
					"metric_id": metricID,
				},
				Fields:    obj,
				Timestamp: time.Now().UTC(),
			}
			if writeErr := c.tsdb.WritePoint(ctx, pt); writeErr != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Warn("Failed to write metric to TSDB").WithError(writeErr).Log()
			}
		} else {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Failed to read metric for TSDB export").WithError(readErr).Log()
		}
	}
}

// Stop stops all async collectors
func (c *UnifiedMetricsCollector) Stop() {
	var casAsync, fileLockAsync *AsyncMetricsCollector
	_ = concurrency.RunInLockOrLog(&c.mu, locknames.LockNameUnifiedMetricsStopCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		casAsync = c.casAsyncCollector
		fileLockAsync = c.fileLockAsyncCollector
		return nil
	})

	// Stop collectors outside lock
	if casAsync != nil {
		casAsync.Stop()
	}
	if fileLockAsync != nil {
		fileLockAsync.Stop()
	}
}

// GetCollector returns a specific collector by type
func (c *UnifiedMetricsCollector) GetCollector(metricType string) MetricsCollector {
	switch metricType {
	case "cas_metric", "cas":
		return c.casCollector
	case objects.KindFileLockMetric, "file_lock":
		return c.fileLockCollector
	case "audit_metric", "audit":
		return c.auditCollector
	default:
		return nil
	}
}

// GetAsyncCollector returns a specific async collector by type
func (c *UnifiedMetricsCollector) GetAsyncCollector(metricType string) *AsyncMetricsCollector {
	switch metricType {
	case "cas_metric", "cas":
		return c.casAsyncCollector
	case objects.KindFileLockMetric, "file_lock":
		return c.fileLockAsyncCollector
	default:
		return nil
	}
}
