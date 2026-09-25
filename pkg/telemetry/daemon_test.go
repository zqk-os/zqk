package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

func TestTelemetryDaemon_Aggregate(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	daemon := NewDaemon(logger)

	err := daemon.Aggregate(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	summary := daemon.GetSummary()
	if summary["status"] != "healthy" {
		t.Fatalf("Expected healthy status, got %v", summary["status"])
	}
}

func TestTelemetryDaemon_AggregateWithSpansAndMetrics(t *testing.T) {
	ctx := context.Background()
	mgr := GlobalManager()

	// Clear any previous records
	for _, h := range mgr.GetHooks() {
		if inMem, ok := h.(*InMemoryHook); ok {
			inMem.Clear()
		}
	}

	// Record a span
	spanCtx, finish := mgr.StartSpan(ctx, "test_operation", map[string]string{"env": "test"})
	finish(nil)

	// Record a metric
	mgr.RecordMetric(spanCtx, "test_metric", 42.0, map[string]string{"unit": "count"})

	logger := logging.GetLoggerFromProfile("system")
	daemon := NewDaemon(logger)

	if err := daemon.Aggregate(ctx); err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	summary := daemon.GetSummary()
	totalSpans, ok := summary["total_spans"].(int)
	if !ok || totalSpans < 1 {
		t.Fatalf("Expected at least 1 total_span, got %v", summary["total_spans"])
	}

	totalMetrics, ok := summary["total_metrics"].(int)
	if !ok || totalMetrics < 1 {
		t.Fatalf("Expected at least 1 total_metric, got %v", summary["total_metrics"])
	}
}

func TestTelemetryDaemon_RunBackgroundGC(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	daemon := NewDaemon(logger)

	// Use a tiny interval and a short timeout context to ensure it runs without hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Should block until context is cancelled, then return
	daemon.RunBackgroundGC(ctx, []string{"."}, time.Hour, 2*time.Millisecond)
}
