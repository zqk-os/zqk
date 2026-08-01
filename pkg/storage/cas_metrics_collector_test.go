package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestCASMetricsCollector_LifetimeCounters(t *testing.T) {
	tmpDir := t.TempDir()
	copyObjectSpecsFromModuleOrSkip(t, tmpDir)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	collector := NewCASMetricsCollector(storageFactory.GetStorage())
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	col, met := collector.GetCASCollectorStats()
	if col != 0 || met != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", col, met)
	}

	windowEnd := time.Now()
	windowStart := windowEnd.Add(-1 * time.Hour)
	_, _ = collector.CollectMetrics(ctx, secCtx, windowStart, windowEnd)

	col, _ = collector.GetCASCollectorStats()
	if col != 1 {
		t.Errorf("expected collectionsTotal=1, got %d", col)
	}
}
