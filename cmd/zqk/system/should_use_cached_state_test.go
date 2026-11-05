package system

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Tier-1 instance_validation must not be a permanent cache hit (fail-closed
// DependentsLookup poison).
func TestShouldUseCachedState_RejectsTier1InstanceValidation(t *testing.T) {
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
			Category: "instance_validation",
			Message:  "status: Precondition not met for status 'complete'",
		}},
	}
	if shouldUseCachedState("X-1", path, state, false) {
		t.Fatal("Tier-1 instance_validation must force revalidation")
	}
	state.Issues = nil
	if !shouldUseCachedState("X-1", path, state, false) {
		t.Fatal("clean cached state should be usable")
	}
}

func TestShouldUseCachedState_RejectsTier1ReferenceAndTraceability(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := testkit.WriteTestObjectStandalone(t, dir, "id: CRIT-1\nkind: criteria\n")

	for _, category := range []string{"reference", "reference_integrity", "traceability"} {
		state := &validation.ValidationState{
			ObjectID:      "CRIT-1",
			ObjectKind:    "criteria",
			FilePath:      path,
			LastValidated: time.Now().Add(time.Hour),
			Issues: []validation.ValidationIssue{{
				Tier:     1,
				Category: category,
				Message:  "missing or broken cross-object relationship",
			}},
		}
		if shouldUseCachedState("CRIT-1", path, state, false) {
			t.Fatalf("Tier-1 %s must force revalidation rather than serving stale cached error", category)
		}
	}
}

func TestShouldUseCachedState_BypassCacheWithIssues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := testkit.WriteTestObjectStandalone(t, dir, "id: GOAL-1\nkind: goal\n")

	state := &validation.ValidationState{
		ObjectID:      "GOAL-1",
		ObjectKind:    "goal",
		FilePath:      path,
		LastValidated: time.Now().Add(time.Hour),
		Issues: []validation.ValidationIssue{{
			Tier:     2,
			Category: "lifecycle",
			Message:  "minor warning",
		}},
	}
	// Normal check without bypass allows non-blocking warnings from cache
	if !shouldUseCachedState("GOAL-1", path, state, false) {
		t.Fatal("normal check should allow warning from cache")
	}
	// When bypassCacheWithIssues is true (e.g. --refresh-cache or --auto-fix), must force revalidation
	if shouldUseCachedState("GOAL-1", path, state, true) {
		t.Fatal("bypassCacheWithIssues must force revalidation for any state with issues")
	}
}
