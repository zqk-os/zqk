package coordination

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// MetricsCollectorAdapter adapts the coordinator to work with UnifiedMetricsCollector
// This bridges the gap between storage metrics and coordination system
type MetricsCollectorAdapter struct {
	coordinator EventCoordinator
}

// NewMetricsCollectorAdapter creates a new adapter that implements MetricsEventEmitter
func NewMetricsCollectorAdapter(coordinator EventCoordinator) *MetricsCollectorAdapter {
	return &MetricsCollectorAdapter{
		coordinator: coordinator,
	}
}

// EmitMetricCollected emits a metric collection event via coordinator
// This implements storage.MetricsEventEmitter interface
func (a *MetricsCollectorAdapter) EmitMetricCollected(
	ctx context.Context,
	metricType, metricID string,
	windowStart, windowEnd time.Time,
	err error,
) {
	if a.coordinator == nil {
		return
	}

	status := "success"
	message := fmt.Sprintf("Collected %s metrics for window %s to %s", metricType, windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339))
	if err != nil {
		status = "error"
		message = fmt.Sprintf("Failed to collect %s metrics: %v", metricType, err)
	}

	eventCtx := NewEventContext(
		fmt.Sprintf("metric_collection_%s_%d", metricType, time.Now().UnixNano()),
		"metric_collection",
		status,
	).WithContext(ctx).WithChannels(true, true, true, false)

	// Set metrics data
	eventCtx.EventData = &EventData{
		MetricsData: map[string]any{
			objects.FieldKeyMetricType:  metricType,
			"metric_id":                 metricID,
			objects.FieldKeyWindowStart: windowStart.Format(time.RFC3339),
			objects.FieldKeyWindowEnd:   windowEnd.Format(time.RFC3339),
			"message":                   message,
		},
	}

	if err != nil {
		eventCtx.Error = err
	}

	// Emit via coordinator (routes to logging, audit, and metrics channels)
	_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
}

// CreateUnifiedMetricsCollector creates a UnifiedMetricsCollector configured with coordinator
// This is a convenience function that sets up the adapter
// NOTE: This function is in coordination package to avoid import cycles
// It returns the concrete type but is only used internally
func CreateUnifiedMetricsCollector(
	coordinator EventCoordinator,
	storageProvider storage.ObjectStorageProvider,
	tsdbProvider storage.TSDBProvider,
	enableAsyncCAS bool,
	enableAsyncFileLock bool,
) any {
	adapter := NewMetricsCollectorAdapter(coordinator)
	config := storage.UnifiedMetricsCollectorConfig{
		Storage:             storageProvider,
		EventEmitter:        adapter,
		TSDB:                tsdbProvider,
		EnableAsyncCAS:      enableAsyncCAS,
		EnableAsyncFileLock: enableAsyncFileLock,
		AsyncBufferSize:     100,
	}
	return storage.NewUnifiedMetricsCollector(config)
}
