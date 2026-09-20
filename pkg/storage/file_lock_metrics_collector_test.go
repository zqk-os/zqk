package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestFileLockMetricsCollector_LifetimeCounters(t *testing.T) {
	mock := &mockStorageProvider{}
	collector := NewFileLockMetricsCollector(mock)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	// Initial counters should be 0
	col, met := collector.GetCollectorStats()
	if col != 0 || met != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", col, met)
	}

	// Attempt collection
	_, _ = collector.CollectMetrics(ctx, secCtx, windowStart, windowEnd)

	col, _ = collector.GetCollectorStats()
	if col != 1 {
		t.Errorf("expected collections=1, got %d", col)
	}
}
