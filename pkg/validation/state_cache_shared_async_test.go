package validation

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestNewAsyncValidatorUsesSharedValidationCache ensures system check and
// CRUD invalidation share one in-memory map so a small flusher Save cannot
// wipe a full check's warm cache on disk.
func TestNewAsyncValidatorUsesSharedValidationCache(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	av := NewAsyncValidator(pkgctx.NewSystemContext(), root, 2, time.Hour)
	sc := getSharedValidationCache(root)
	if sc == nil || sc.cache == nil {
		t.Fatal("expected shared validation cache for non-empty project root")
	}
	if av.stateCache != sc.cache {
		t.Fatal("AsyncValidator.stateCache must be the shared singleton, not a private cache")
	}

	state := &ValidationState{
		ObjectID:      "BLI-test-shared-cache-001",
		ObjectKind:    "backlog_item",
		FilePath:      filepath.Join(root, paths.ProcessDir, "backlog_items/x.yaml"),
		LastValidated: time.Now(),
	}
	av.stateCache.Set(state)

	got, ok := sc.cache.Get(state.ObjectID)
	if !ok || got == nil || got.ObjectID != state.ObjectID {
		t.Fatalf("shared cache missing Set from AsyncValidator: ok=%v got=%v", ok, got)
	}
}

func TestShrinkGuardSkipSave(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cache := NewValidationStateCache(root, time.Hour)

	// Seed a warm on-disk cache (> shrinkGuardMinDiskEntries).
	for i := 0; i < shrinkGuardMinDiskEntries+50; i++ {
		id := fmt.Sprintf("BLI-warm-%s-%d", filepath.Base(root), i)
		cache.Set(&ValidationState{
			ObjectID:      id,
			ObjectKind:    "backlog_item",
			FilePath:      filepath.Join(root, "o.yaml"),
			LastValidated: time.Now(),
		})
	}
	if err := cache.Save(); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	tiny := map[string]kindBucket{
		"scheduler_job": {Entries: make([]validationStateEntry, 81)},
	}
	for i := range tiny["scheduler_job"].Entries {
		tiny["scheduler_job"].Entries[i].ID = fmt.Sprintf("SCH-tiny-%d", i)
	}
	if !shrinkGuardSkipSave(cache.cacheFile, tiny) {
		t.Fatal("expected shrink guard to skip tiny overwrite of warm cache")
	}

	large := map[string]kindBucket{
		"backlog_item": {Entries: make([]validationStateEntry, shrinkGuardMinDiskEntries)},
	}
	if shrinkGuardSkipSave(cache.cacheFile, large) {
		t.Fatal("substantial snapshot must be allowed to save")
	}
}

func TestSave_SkipsOlderInMemoryWhenDiskIsNewer(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stale := NewValidationStateCache(root, time.Hour)
	stale.Set(&ValidationState{
		ObjectID:      "BLI-stale-gen",
		ObjectKind:    "backlog_item",
		FilePath:      filepath.Join(root, "stale.yaml"),
		LastValidated: time.Now(),
	})
	if err := stale.Save(); err != nil {
		t.Fatalf("stale Save: %v", err)
	}
	if err := stale.Load(); err != nil {
		t.Fatalf("stale Load: %v", err)
	}

	fresh := NewValidationStateCache(root, time.Hour)
	if err := fresh.Load(); err != nil {
		t.Fatalf("fresh Load: %v", err)
	}
	fresh.Set(&ValidationState{
		ObjectID:      "BLI-fresh-gen",
		ObjectKind:    "backlog_item",
		FilePath:      filepath.Join(root, "fresh.yaml"),
		LastValidated: time.Now(),
	})
	if err := fresh.Save(); err != nil {
		t.Fatalf("fresh Save: %v", err)
	}

	if err := stale.Save(); err != nil {
		t.Fatalf("stale Save after newer disk: %v", err)
	}
	reload := NewValidationStateCache(root, time.Hour)
	if err := reload.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := reload.Get("BLI-fresh-gen"); !ok {
		t.Fatal("stale instance must not clobber a newer on-disk generation")
	}
}

func TestClear_SuppressesLoadFromRacedRestore(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cache := NewValidationStateCache(root, time.Hour)
	cache.Set(&ValidationState{
		ObjectID:      "BLI-ghost",
		ObjectKind:    "backlog_item",
		FilePath:      filepath.Join(root, "g.yaml"),
		LastValidated: time.Now(),
		Issues:        []ValidationIssue{{Tier: 1, Category: "GhostRef", Message: "gone"}},
	})
	if err := cache.Save(); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
	poison, err := fileutil.ReadFile(cache.cacheFile)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := fileutil.WriteFile(cache.cacheFile, poison, paths.FilePerm600); err != nil {
		t.Fatalf("restore poison: %v", err)
	}
	if err := cache.Load(); err != nil {
		t.Fatalf("Load after Clear: %v", err)
	}
	if _, ok := cache.Get("BLI-ghost"); ok {
		t.Fatal("Clear must suppress Load from a raced restore of the old file")
	}
}
