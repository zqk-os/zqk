package storage

// This file demonstrates how to use UnifiedMetricsCollector with the coordinator

/*
Example: Using UnifiedMetricsCollector with Coordinator

1. Set up coordinator with metrics router:
```go
import (
    "github.com/zqk-os/zqk/pkg/coordination"
    "github.com/zqk-os/zqk/pkg/metrics"
    "github.com/zqk-os/zqk/pkg/storage"
)

// Create metrics pipeline (shared per project in the real scheduler/CLI)
metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)

// Create coordinator with metrics router
coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
    LoggingRouter:     &coordination.DefaultLoggingRouter{},
    AuditRouter:       coordination.NewStorageAuditRouter(projectRoot, storageProvider),
    MetricsRouter:     coordination.NewMetricPipelineRouter(metricsPipeline),
    OperationalRouter: &coordination.DefaultOperationalRouter{},
})

// Create unified metrics collector (uses adapter to avoid import cycles)
unifiedCollector := coordination.CreateUnifiedMetricsCollector(
    coordinator,
    storageProvider,
    true,  // Enable async CAS metrics (high volume)
    true,  // Enable async file lock metrics (high volume)
)

// Periodically collect all metrics (e.g., hourly)
windowStart := time.Now().Add(-1 * time.Hour)
windowEnd := time.Now()
metricIDs, err := unifiedCollector.CollectAllMetrics(ctx, secCtx, windowStart, windowEnd)
if err != nil {
    log.Printf("Some metrics collection failed: %v", err)
} else {
    log.Printf("Collected %d metric objects", len(metricIDs))
}

// Or collect specific metric type
casMetricID, err := unifiedCollector.CollectMetricsByType(ctx, secCtx, "cas_metric", windowStart, windowEnd)

// Cleanup on shutdown
unifiedCollector.Stop()
```

2. The unified collector automatically:
   - Collects metrics from all collectors (CAS, FileLock, Audit)
   - Uses async collectors for high-volume metrics (prevents blocking)
   - Emits events via coordinator (routes to logging, audit, and metrics channels)
   - Handles errors gracefully (continues collecting other metrics)

3. Coordinator routes metric collection events to:
   - Logging channel: Structured logs about metric collection
   - Audit channel: Audit trail of metric collection operations
   - Metrics channel: MetricPipeline aggregates these events

4. Benefits:
   - Single component manages all metrics
   - Unified event emission via coordinator
   - High-volume metrics use async collection (non-blocking)
   - All metric collection events are observable through coordinator
   - Easy to add new metric types (just add to unified collector)
*/
