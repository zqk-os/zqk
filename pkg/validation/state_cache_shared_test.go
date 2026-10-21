package validation

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSharedValidationCache_Helpers(t *testing.T) {
	tempDir := t.TempDir()
	t.Cleanup(func() {
		StopSharedValidationCacheForTest(tempDir)
	})

	// Empty projectRoot
	if SharedValidationStateCache("") != nil {
		t.Error("expected nil SharedValidationStateCache on empty projectRoot")
	}
	if err := EnsureValidationCacheReady(""); err != nil {
		t.Errorf("expected nil on empty projectRoot, got %v", err)
	}

	// EnsureValidationCacheReady on temp project
	if err := EnsureValidationCacheReady(tempDir); err != nil {
		t.Fatalf("EnsureValidationCacheReady failed: %v", err)
	}

	shared := SharedValidationStateCache(tempDir)
	if shared == nil {
		t.Fatal("expected SharedValidationStateCache for non-empty projectRoot")
	}
	if SharedValidationStateCache(tempDir) != shared {
		t.Fatal("SharedValidationStateCache must return the same singleton")
	}
	if aged := ApplySharedValidationCacheMaxAge(tempDir, time.Hour); aged != shared {
		t.Fatal("ApplySharedValidationCacheMaxAge must return the same singleton")
	}
	if ApplySharedValidationCacheMaxAge("", time.Hour) != nil {
		t.Error("expected nil ApplySharedValidationCacheMaxAge on empty projectRoot")
	}

	// Test with CacheModeTestSync
	ctx := pkgctx.WithCacheMode(context.Background(), pkgctx.CacheModeTestSync)

	// Upsert state
	state := &ValidationState{
		ObjectID:      "BLI-TEST-1",
		ObjectKind:    objects.KindBacklogItem,
		LastValidated: time.Now(),
		Issues:        []ValidationIssue{},
	}
	UpdateObjectInGlobalValidationCacheWithContext(ctx, tempDir, state)

	// Retrieve via exported accessor and test helper (same singleton)
	direct, ok := shared.Get("BLI-TEST-1")
	if !ok || direct == nil || direct.ObjectID != "BLI-TEST-1" {
		t.Fatalf("expected state via SharedValidationStateCache Get: ok=%v got=%v", ok, direct)
	}
	got, ok := GetValidationStateForTest(ctx, tempDir, "BLI-TEST-1")
	if !ok || got == nil {
		t.Fatalf("expected state to be found in cache")
	}
	if got.ObjectID != "BLI-TEST-1" {
		t.Errorf("expected ObjectID BLI-TEST-1, got %s", got.ObjectID)
	}

	// Invalidate
	InvalidateObjectsInGlobalValidationCache(tempDir, []string{"BLI-TEST-1"})

	// Verify invalidated
	_, ok = GetValidationStateForTest(ctx, tempDir, "BLI-TEST-1")
	if ok {
		t.Errorf("expected object to be invalidated from cache")
	}
	if _, ok = shared.Get("BLI-TEST-1"); ok {
		t.Error("SharedValidationStateCache must observe InvalidateObjects*")
	}

	// InvalidateObjectsInGlobalValidationCacheWithContext with empty IDs
	InvalidateObjectsInGlobalValidationCacheWithContext(ctx, tempDir, []string{})
}

func TestValidationStateCache_Methods(t *testing.T) {
	tempDir := t.TempDir()
	c := NewValidationStateCache(tempDir, time.Hour)

	// Set & Get
	s1 := &ValidationState{
		ObjectID:      "BLI-A",
		ObjectKind:    objects.KindBacklogItem,
		LastValidated: time.Now(),
		Issues: []ValidationIssue{
			{Category: "policy", Tier: 1},
		},
	}
	s2 := &ValidationState{
		ObjectID:      "GOAL-B",
		ObjectKind:    objects.KindGoal,
		LastValidated: time.Now(),
		Issues: []ValidationIssue{
			{Category: "lifecycle", Tier: 2},
		},
	}
	c.Set(s1)
	c.Set(s2)

	all := c.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 states in GetAll, got %d", len(all))
	}

	// InvalidateByCategory
	removed := c.InvalidateByCategory("policy")
	if removed != 1 {
		t.Errorf("expected 1 removed by category, got %d", removed)
	}

	// InvalidateByKind
	c.InvalidateByKind(objects.KindGoal)
	if len(c.GetAll()) != 0 {
		t.Errorf("expected empty cache after InvalidateByKind")
	}

	// Clear
	c.Set(s1)
	c.Clear()
	if _, ok := c.Get("BLI-A"); ok {
		t.Errorf("expected empty cache after Clear")
	}
}
