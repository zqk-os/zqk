package agentclaim

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestTryClaim_stampsBranchRefAndBaseSha(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	orig := gitOutput
	t.Cleanup(func() { gitOutput = orig })
	gitOutput = func(_ string, args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return "integration/pri-r20", nil
		}
		return "abc123def456", nil
	}

	store := &claimMemStore{objs: map[string]map[string]any{
		"ATK-1": {
			objects.FieldKeyID:          "ATK-1",
			objects.FieldKeyKind:        objects.KindAgentTask,
			objects.FieldKeyDescription: "Advance BLI-CEF-R20-BRANCH-REF-SPEC-001",
		},
		"BLI-CEF-R20-BRANCH-REF-SPEC-001": {
			objects.FieldKeyID:              "BLI-CEF-R20-BRANCH-REF-SPEC-001",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyPriorityPlanRef: "PRI-CEF-R20-BRANCH-PROVENANCE-001",
		},
		"PRI-CEF-R20-BRANCH-PROVENANCE-001": {
			objects.FieldKeyID:   "PRI-CEF-R20-BRANCH-PROVENANCE-001",
			objects.FieldKeyKind: objects.KindPriorityPlan,
		},
	}}

	res, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a", ClaimOptions{ProjectRoot: t.TempDir()})
	if err != nil || !res.Claimed {
		t.Fatalf("claim: %#v err=%v", res, err)
	}

	bli := store.objs["BLI-CEF-R20-BRANCH-REF-SPEC-001"]
	if bli[objects.FieldKeyBranchRef] != "integration/pri-r20" || bli[objects.FieldKeyBaseSha] != "abc123def456" {
		t.Fatalf("BLI provenance = %#v", bli)
	}
	pri := store.objs["PRI-CEF-R20-BRANCH-PROVENANCE-001"]
	if pri[objects.FieldKeyBranchRef] != "integration/pri-r20" || pri[objects.FieldKeyBaseSha] != "abc123def456" {
		t.Fatalf("PRI provenance = %#v", pri)
	}

	// Re-claim must not move an already-recorded location.
	gitOutput = func(string, ...string) (string, error) { return "other", nil }
	if _, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a", ClaimOptions{ProjectRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if bli[objects.FieldKeyBranchRef] != "integration/pri-r20" {
		t.Fatalf("re-claim overwrote branch_ref: %#v", bli)
	}
}

func TestClaimTargetIDs_fromForRefAndLists(t *testing.T) {
	t.Parallel()
	got := claimTargetIDs(map[string]any{
		objects.FieldKeyRelatedObjectRefs: []string{"BLI-ONE", "CRIT-SKIP"},
		objects.FieldKeyTitle:             "SWARM: PRI-TWO extra",
	}, ClaimOptions{ForRef: "BLI-FOR"})
	want := []string{"BLI-FOR", "BLI-ONE", "PRI-TWO"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}
