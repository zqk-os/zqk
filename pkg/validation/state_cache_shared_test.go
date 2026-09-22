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

	// Empty projectRoot
	if err := EnsureValidationCacheReady(""); err != nil {
		t.Errorf("expected nil on empty projectRoot, got %v", err)
	}

	// EnsureValidationCacheReady on temp project
	if err := EnsureValidationCacheReady(tempDir); err != nil {
		t.Fatalf("EnsureValidationCacheReady failed: %v", err)
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

	// Retrieve
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

