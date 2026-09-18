package cas_test

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestCASMetricsCollector_LifetimeCounters(t *testing.T) {
	tmpDir := t.TempDir()
	storage.CopyObjectSpecsFromModuleOrSkipForTest(t, tmpDir)
	storageFactory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("storage.NewStorageFactory: %v", err)
	}
	collector := caspkg.NewCASMetricsCollector(storageFactory.GetStorage())
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
