package system

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Tier-3 cache-coherence issues describe the object-id-cache at validation time, not the object.
// Persisting them made every later run replay stale "CacheLag" for objects that were otherwise
// cache hits, so the noise survived --refresh-cache and only --clear-cache cleared it.
// TRACK: BLI-1786387465409533000-45bd780c
func TestConvertCheckResultToValidationState_DropsCacheCoherenceIssues(t *testing.T) {
	t.Parallel()
	result := &CheckResult{
		ObjectID:   "BLI-1",
		ObjectKind: "backlog_item",
		FilePath:   "p1.yaml",
		Issues: []Issue{
			{Tier: 3, Category: categoryCacheLag, Message: "CacheLag: Referenced object GOAL-1 exists in storage but is missing from object-id-cache."},
			{Tier: 3, Category: categoryCacheCoherence, Message: "Excluded from blocking check: object-id-cache not yet trued after CAS mutation"},
			{Tier: 1, Category: categoryIntegrity, Message: "hash missing"},
		},
	}

	state := convertCheckResultToValidationState(result)

	if len(state.Issues) != 1 {
		t.Fatalf("want only the durable issue persisted, got %d: %+v", len(state.Issues), state.Issues)
	}
	if state.Issues[0].Category != categoryIntegrity {
		t.Fatalf("want %s persisted, got %s", categoryIntegrity, state.Issues[0].Category)
	}
}

// Entries written by an older binary must force revalidation so they are rewritten clean,
// rather than replaying until the user runs --clear-cache.
func TestShouldUseCachedState_RejectsCacheCoherenceIssues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := testkit.WriteTestObjectStandalone(t, dir, "id: X-1\n")

	for _, category := range []string{categoryCacheLag, categoryCacheCoherence} {
		state := &validation.ValidationState{
			ObjectID:      "X-1",
			ObjectKind:    "priority_plan",
			FilePath:      path,
			LastValidated: time.Now().Add(time.Hour),
			Issues: []validation.ValidationIssue{{
				Tier:     3,
				Category: category,
				Message:  "stale cache-coherence entry from an earlier run",
			}},
		}
		if shouldUseCachedState("X-1", path, state, false) {
			t.Fatalf("cached %s issue must force revalidation", category)
		}
	}
}

func TestShouldUseCachedState_RejectsGhostRef(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := testkit.WriteTestObjectStandalone(t, dir, "id: X-1\n")
	state := &validation.ValidationState{
		ObjectID:      "X-1",
		ObjectKind:    "priority_plan",
		FilePath:      path,
		LastValidated: time.Now().Add(time.Hour),
		Issues: []validation.ValidationIssue{{
			Tier:     1,
			Category: categoryGhostRef,
			Message:  "GhostRef: Referenced object PRI-SYM-005 does not exist",
		}},
	}
	if shouldUseCachedState("X-1", path, state, false) {
		t.Fatal("cached GhostRef must force revalidation after object-id-cache refresh")
	}
}
