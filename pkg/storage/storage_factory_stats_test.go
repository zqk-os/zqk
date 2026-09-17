package storage

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestStorageFactory_LifetimeCounters(t *testing.T) {
	t.Parallel()

	var nilFactory *StorageFactory
	cNil, pNil, dNil := nilFactory.GetStorageFactoryStats()
	if pNil != 0 || dNil != 0 {
		t.Fatalf("expected nil StorageFactory stats (c, 0, 0), got (%d, %d, %d)", cNil, pNil, dNil)
	}

	ctx := context.Background()
	tmpDir := t.TempDir()
	if err := paths.EnsureProcessAndObjectSpecsLayout(tmpDir); err != nil {
		t.Fatalf("EnsureProcessAndObjectSpecsLayout failed: %v", err)
	}

	cBefore, _, _ := nilFactory.GetStorageFactoryStats()

	factory, err := NewStorageFactory(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory failed: %v", err)
	}
	t.Cleanup(func() {
		_ = factory.Shutdown(ctx)
	})

	cAfter, pInit, dInit := factory.GetStorageFactoryStats()
	if cAfter <= cBefore {
		t.Fatalf("expected factoriesCreatedTotal to increment, got before=%d after=%d", cBefore, cAfter)
	}

	// 1. GetStorageForKind registered/default
	_ = factory.GetStorageForKind("backlog_item")
	_ = factory.GetStorageForKind("unknown_kind_xyz")

	_, pAfter, dAfter := factory.GetStorageFactoryStats()
	if pAfter < pInit && dAfter < dInit {
		t.Fatalf("expected provider/default lookups to increment, got pInit=%d pAfter=%d dInit=%d dAfter=%d", pInit, pAfter, dInit, dAfter)
	}
}
