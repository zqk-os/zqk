package lifecycle

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// A shared-lane policy marked Fail: closed refuses an archive hop while a live lineage sibling
// still points at the shared object. Two gaps let it admit the hop on evidence it never obtained:
// a dependent whose read failed was treated as absent, and a dependent with no status was treated
// as dormant. Both are absence of evidence, and in a fail-closed membrane that has to refuse.
//
// Reachability, so these are not hypothetical: of the four shared-mode declarations in the
// lifecycle plane, workstream archived is the only one at Fail: closed. The other three
// (roadmap, strategic_plan, workstream_transition) are explicitly Fail: open.

func workstreamSharedLifecycles() staticLifecycles {
	lc := storyLifecycles()
	lc[kindWorkstream] = sharedArchiveLifecycle(objects.ShockwaveFailClosed, objects.KindWorkstreamTransition)
	return lc
}

func liveWorkstream() map[string]map[string]any {
	return map[string]map[string]any{
		"WS-1": {
			objects.FieldKeyID:     "WS-1",
			objects.FieldKeyKind:   kindWorkstream,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}
}

func archiveWorkstream(t *testing.T, store *memMembraneStore, lc staticLifecycles, deps func(string) []string) error {
	t.Helper()
	_, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(),
		store, lc, deps, []string{"WS-1"}, objects.ObjectStatusArchived)
	return err
}

func dependsOnWorkstream(depIDs ...string) func(string) []string {
	return func(id string) []string {
		if id == "WS-1" {
			return depIDs
		}
		return nil
	}
}

func TestSharedFailClosed_blocksWhenDependentCannotBeRead(t *testing.T) {
	t.Parallel()
	// PRI-GHOST is reachable through the dependency index but absent from the store, so the
	// read fails. Previously the error was discarded and the hop proceeded.
	err := archiveWorkstream(t, &memMembraneStore{objs: liveWorkstream()},
		workstreamSharedLifecycles(), dependsOnWorkstream("PRI-GHOST"))
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("unreadable dependent must block a fail-closed shared hop, got %v", err)
	}
	if blocked.BlockingID != "WS-1" {
		t.Errorf("blocking id = %q, want the seed WS-1", blocked.BlockingID)
	}
}

func TestSharedFailClosed_blocksWhenDependentHasNoStatus(t *testing.T) {
	t.Parallel()
	objs := liveWorkstream()
	objs["PRI-BLANK"] = map[string]any{
		objects.FieldKeyID:     "PRI-BLANK",
		objects.FieldKeyKind:   kindPriorityPlan,
		objects.FieldKeyStatus: "",
	}
	err := archiveWorkstream(t, &memMembraneStore{objs: objs},
		workstreamSharedLifecycles(), dependsOnWorkstream("PRI-BLANK"))
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("dependent with no status must block a fail-closed shared hop, got %v", err)
	}
}

// TestSharedFailClosed_stillAdmitsGenuinelyDormantDependents guards against overcorrection. The
// barrier exists to refuse *live* siblings; if it also refused terminal ones, no workstream could
// ever be archived and the only remedy would be to turn the policy back to open.
func TestSharedFailClosed_stillAdmitsGenuinelyDormantDependents(t *testing.T) {
	t.Parallel()
	for _, status := range []string{
		objects.ObjectStatusComplete,
		objects.ObjectStatusCompleted,
		objects.ObjectStatusCancelled,
		objects.ObjectStatusArchived,
	} {
		objs := liveWorkstream()
		objs["PRI-1"] = map[string]any{
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyKind:   kindPriorityPlan,
			objects.FieldKeyStatus: status,
		}
		if err := archiveWorkstream(t, &memMembraneStore{objs: objs},
			workstreamSharedLifecycles(), dependsOnWorkstream("PRI-1")); err != nil {
			t.Errorf("dependent at %q is dormant and must not block the hop: %v", status, err)
		}
	}
}

// TestSharedFailOpen_stillAdmitsUnreadableDependent keeps the strictness scoped to the declared
// policy. roadmap, strategic_plan, and workstream_transition archived all declare Fail: open, and
// that choice belongs to the lifecycle author rather than to this function.
func TestSharedFailOpen_stillAdmitsUnreadableDependent(t *testing.T) {
	t.Parallel()
	lc := storyLifecycles()
	lc[kindWorkstream] = sharedArchiveLifecycle(objects.ShockwaveFailOpen, objects.KindWorkstreamTransition)
	if err := archiveWorkstream(t, &memMembraneStore{objs: liveWorkstream()}, lc,
		dependsOnWorkstream("PRI-GHOST")); err != nil {
		t.Errorf("fail-open policy must not acquire fail-closed strictness: %v", err)
	}
}
