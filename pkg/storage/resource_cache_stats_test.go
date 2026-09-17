package storage

import (
	"context"
	"testing"
)

func TestResourceCache_LifetimeCounters(t *testing.T) {
	t.Parallel()

	var nilRc *ResourceCache[string]
	hNil, mNil, cNil := nilRc.GetResourceCacheStats()
	if hNil != 0 || mNil != 0 || cNil != 0 {
		t.Fatalf("expected nil stats (0, 0, 0), got (%d, %d, %d)", hNil, mNil, cNil)
	}

	rc := &ResourceCache[string]{}

	ctx := context.Background()

	// 1. GetOrCreate first time -> miss (1), creation (1)
	val1, err := rc.GetOrCreate(ctx, "k1", func(ctx context.Context, key string) (string, error) {
		return "val-1", nil
	})
	if err != nil || val1 != "val-1" {
		t.Fatalf("unexpected GetOrCreate result: val=%s err=%v", val1, err)
	}

	h1, m1, c1 := rc.GetResourceCacheStats()
	if h1 != 0 || m1 != 1 || c1 != 1 {
		t.Fatalf("expected stats (0, 1, 1), got (%d, %d, %d)", h1, m1, c1)
	}

	// 2. GetOrCreate second time -> hit (1)
	val2, err := rc.GetOrCreate(ctx, "k1", func(ctx context.Context, key string) (string, error) {
		return "val-2", nil
	})
	if err != nil || val2 != "val-1" {
		t.Fatalf("unexpected GetOrCreate second result: val=%s err=%v", val2, err)
	}

	h2, m2, c2 := rc.GetResourceCacheStats()
	if h2 != 1 || m2 != 1 || c2 != 1 {
		t.Fatalf("expected stats (1, 1, 1), got (%d, %d, %d)", h2, m2, c2)
	}

	// 3. Get existing -> hit (2)
	val3, found := rc.Get("k1")
	if !found || val3 != "val-1" {
		t.Fatalf("expected Get hit for k1")
	}

	// 4. Get missing -> miss (2)
	_, foundMissing := rc.Get("missing")
	if foundMissing {
		t.Fatalf("expected Get miss for missing key")
	}

	hFinal, mFinal, cFinal := rc.GetResourceCacheStats()
	if hFinal != 2 || mFinal != 2 || cFinal != 1 {
		t.Fatalf("expected final stats (2, 2, 1), got (%d, %d, %d)", hFinal, mFinal, cFinal)
	}
}

func TestStorageProviderCache_LifetimeCounters(t *testing.T) {
	t.Parallel()

	var nilSpc *StorageProviderCache
	hNil, mNil, cNil := nilSpc.GetStorageProviderCacheStats()
	if hNil != 0 || mNil != 0 || cNil != 0 {
		t.Fatalf("expected nil StorageProviderCache stats (0, 0, 0), got (%d, %d, %d)", hNil, mNil, cNil)
	}

	spc := NewStorageProviderCache()
	if spc == nil {
		t.Fatalf("expected NewStorageProviderCache non-nil")
	}

	hInit, mInit, cInit := spc.GetStorageProviderCacheStats()
	if hInit != 0 || mInit != 0 || cInit != 0 {
		t.Fatalf("expected initial StorageProviderCache stats (0, 0, 0), got (%d, %d, %d)", hInit, mInit, cInit)
	}
}
