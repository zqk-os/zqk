package system

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
)

// Tier-1 instance_validation must not be a permanent cache hit (fail-closed
// DependentsLookup poison). TRACK: BLI-REDACTED
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
