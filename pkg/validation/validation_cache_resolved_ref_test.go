package validation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestValidationCache_ResolvedReferenceBypassesStaleCachedError verifies GitHub Issue #939:
// When an object previously cached a Tier-1 reference or traceability error, subsequent
// checks must NOT treat the stale error as a permanent cache hit. Instead, it must be
// re-evaluated so resolved cross-object references do not produce false-positive Tier 1 blockers.
func TestValidationCache_ResolvedReferenceBypassesStaleCachedError(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "CRIT-001.yaml")
	fileContent := []byte("id: CRIT-001\nkind: criteria\ncategory: test\n")
	if err := fileutil.WriteFile(filePath, fileContent, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	fileTime := time.Now().Add(-10 * time.Second)
	_ = fileutil.Chtimes(filePath, fileTime, fileTime)

	av := NewAsyncValidator(pkgctx.NewSystemContext(), tempDir, 2, time.Hour)
	defer av.Stop()

	// 1. Simulate an earlier check caching a Tier-1 traceability/reference error
	staleState := &ValidationState{
		ObjectID:      "CRIT-001",
		ObjectKind:    "criteria",
		FilePath:      filePath,
		LastValidated: time.Now().Add(-5 * time.Second),
		Checksum:      av.computeChecksum(filePath),
		Issues: []ValidationIssue{
			{
				Tier:     1,
				Category: "traceability",
				Message:  "Test criterion CRIT-001 has category=\"test\" but is not bound to any test_case object",
			},
		},
	}
	av.stateCache.Set(staleState)

	// Verify state is in cache
	cached, ok := av.GetCachedState("CRIT-001")
	if !ok || len(cached.Issues) != 1 {
		t.Fatalf("expected cached state with 1 issue, got ok=%v, cached=%v", ok, cached)
	}

	// 2. ShouldEnqueue must NOT treat the stale Tier-1 reference/traceability error as a cache hit
	if !av.ShouldEnqueue("CRIT-001", "criteria", filePath) {
		t.Fatal("ShouldEnqueue must return true (force re-enqueue) when cached state has Tier-1 reference/traceability issues")
	}

	// 3. Enqueue must NOT return true (cache hit); it must enqueue the task for re-validation
	cacheHit := av.Enqueue("CRIT-001", "criteria", filePath, 1)
	if cacheHit {
		t.Fatal("Enqueue must return false (cache miss / enqueued) when cached state has Tier-1 reference/traceability issues")
	}

	// 4. ValidateNow must re-validate rather than serving the stale cached error
	resolvedState := &ValidationState{
		ObjectID:      "CRIT-001",
		ObjectKind:    "criteria",
		FilePath:      filePath,
		LastValidated: time.Now(),
		Checksum:      av.computeChecksum(filePath),
		Issues:        nil, // Resolved: no issues!
	}
	av.SetValidationFunc(func(ctx context.Context, id, kind, path string, data []byte) (*ValidationState, error) {
		return resolvedState, nil
	})

	nowState, err := av.ValidateNow(context.Background(), "CRIT-001", "criteria", filePath)
	if err != nil {
		t.Fatalf("ValidateNow failed: %v", err)
	}
	if len(nowState.Issues) != 0 {
		t.Fatalf("expected 0 issues after resolved reference re-validation, got %d issues: %+v", len(nowState.Issues), nowState.Issues)
	}

	// 5. Now that the resolved state (clean) is in cache, clean states CAN be cached
	if av.ShouldEnqueue("CRIT-001", "criteria", filePath) {
		t.Fatal("ShouldEnqueue must return false (cache hit) for clean, up-to-date state")
	}
}

// TestValidationCache_ReferenceCategoriesInvalidate ensures all reference-related
// issue categories (reference, GhostRef, traceability, reference_integrity) properly
// trigger cache re-enqueue and bypass.
func TestValidationCache_ReferenceCategoriesInvalidate(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "REQ-001.yaml")
	fileContent := []byte("id: REQ-001\nkind: requirement\n")
	if err := fileutil.WriteFile(filePath, fileContent, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	fileTime := time.Now().Add(-10 * time.Second)
	_ = fileutil.Chtimes(filePath, fileTime, fileTime)

	av := NewAsyncValidator(pkgctx.NewSystemContext(), tempDir, 2, time.Hour)
	defer av.Stop()

	testCategories := []string{
		"reference",
		"GhostRef",
		"GHOSTREF",
		"traceability",
		"reference_integrity",
		"REFERENCE",
	}

	for _, cat := range testCategories {
		t.Run(cat, func(t *testing.T) {
			av.InvalidateCache("REQ-001")
			st := &ValidationState{
				ObjectID:      "REQ-001",
				ObjectKind:    "requirement",
				FilePath:      filePath,
				LastValidated: time.Now(),
				Checksum:      av.computeChecksum(filePath),
				Issues: []ValidationIssue{
					{Tier: 1, Category: cat, Message: "missing target reference"},
				},
			}
			av.stateCache.Set(st)

			if !av.ShouldEnqueue("REQ-001", "requirement", filePath) {
				t.Fatalf("category %q should invalidate cache hit in ShouldEnqueue", cat)
			}
		})
	}

	// Tier 2 or 3 reference issues should NOT invalidate the cache (they are warnings/info)
	av.InvalidateCache("REQ-001")
	nonBlocking := &ValidationState{
		ObjectID:      "REQ-001",
		ObjectKind:    "requirement",
		FilePath:      filePath,
		LastValidated: time.Now(),
		Checksum:      av.computeChecksum(filePath),
		Issues: []ValidationIssue{
			{Tier: 2, Category: "reference", Message: "minor reference warning"},
		},
	}
	av.stateCache.Set(nonBlocking)

	if av.ShouldEnqueue("REQ-001", "requirement", filePath) {
		t.Fatal("Tier-2 reference warning should not force re-enqueue when mtime is unchanged")
	}
}

// TestValidationCache_ContentHashOrModtimeChangeInvalidates verifies that
// updating an object's file content or modtime invalidates the cached validation state.
func TestValidationCache_ContentHashOrModtimeChangeInvalidates(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "BLI-001.yaml")
	fileContent := []byte("id: BLI-001\nkind: backlog_item\ntitle: Initial\n")
	if err := fileutil.WriteFile(filePath, fileContent, paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	av := NewAsyncValidator(pkgctx.NewSystemContext(), tempDir, 2, time.Hour)
	defer av.Stop()

	info, err := fileutil.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat test file: %v", err)
	}

	// Initial clean validation state matching the file's current modtime
	av.stateCache.Set(&ValidationState{
		ObjectID:      "BLI-001",
		ObjectKind:    "backlog_item",
		FilePath:      filePath,
		LastValidated: info.ModTime(),
		Checksum:      av.computeChecksum(filePath),
		Issues:        nil,
	})

	// Unchanged file -> Cache hit
	if av.ShouldEnqueue("BLI-001", "backlog_item", filePath) {
		t.Fatal("expected cache hit for unmodified file")
	}

	// Modify file content (updates mtime and checksum)
	time.Sleep(50 * time.Millisecond) // Ensure mtime advances
	newContent := []byte("id: BLI-001\nkind: backlog_item\ntitle: Updated\n")
	if err := fileutil.WriteFile(filePath, newContent, paths.FilePerm600); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}
	now := time.Now().Add(100 * time.Millisecond)
	_ = fileutil.Chtimes(filePath, now, now)

	// Should now be cache miss (modtime is after LastValidated)
	if !av.ShouldEnqueue("BLI-001", "backlog_item", filePath) {
		t.Fatal("expected ShouldEnqueue=true after file modification")
	}
}
