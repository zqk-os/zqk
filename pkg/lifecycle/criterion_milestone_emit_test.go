package lifecycle

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestTryEmitAllCriteriaCompleteForMilestone_AppendsCriterionSatisfied(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: shared Memgraph + fixed fixture IDs.

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "MIL-1",
		objects.FieldKeyKind:         objects.KindMilestone,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1", "CRIT-2"},
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-1",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-2",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: statusComplete,
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitAllCriteriaCompleteForMilestone(ctx, projectRoot, "MIL-1", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllCriteriaCompleteForMilestone &&
			ev.Scope[scopeMilestoneID] == "MIL-1" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected criterion_satisfied event for all_criteria_complete_for_milestone")
	}
}

func TestTryEmitForMilestonesContainingCriterion_OnlySatisfiedMilestonesEmit(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: shared Memgraph + fixed fixture IDs.

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "MIL-satisfied",
		objects.FieldKeyKind:         objects.KindMilestone,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-target", "CRIT-ok"},
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "MIL-unsatisfied",
		objects.FieldKeyKind:         objects.KindMilestone,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-target", "CRIT-bad"},
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-target",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-ok",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: statusComplete,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-bad",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: statusInProgress,
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitForMilestonesContainingCriterion(ctx, projectRoot, "CRIT-target", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	emitted := map[string]bool{}
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllCriteriaCompleteForMilestone {
			emitted[ev.Scope[scopeMilestoneID]] = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !emitted["MIL-satisfied"] {
		t.Fatal("expected emit for MIL-satisfied")
	}
	if emitted["MIL-unsatisfied"] {
		t.Fatal("did not expect emit for MIL-unsatisfied")
	}
}

func TestTryEmitAllBacklogItemsCompleteForMilestone_AppendsCriterionSatisfied(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: shared Memgraph + fixed fixture IDs.

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:            "BLI-test-1",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyStatus:        statusComplete,
		objects.FieldKeyMilestoneRefs: []any{"MIL-test-backlog"},
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:            "BLI-test-2",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyStatus:        statusArchived,
		objects.FieldKeyMilestoneRefs: []any{"MIL-test-backlog"},
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitAllBacklogItemsCompleteForMilestone(ctx, projectRoot, "MIL-test-backlog", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllBacklogCompleteForMilestone &&
			ev.Scope[scopeMilestoneID] == "MIL-test-backlog" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected criterion_satisfied event for all_backlog_items_complete_for_milestone")
	}
}

func TestTryEmitAllBacklogItemsCompleteForMilestone_ScalarMilestoneRef(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "BLI-scalar-1",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       statusComplete,
		objects.FieldKeyMilestoneRef: "MIL-scalar-backlog",
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "BLI-scalar-2",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       statusArchived,
		objects.FieldKeyMilestoneRef: "MIL-scalar-backlog",
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitAllBacklogItemsCompleteForMilestone(ctx, projectRoot, "MIL-scalar-backlog", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllBacklogCompleteForMilestone &&
			ev.Scope[scopeMilestoneID] == "MIL-scalar-backlog" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected criterion_satisfied event for all_backlog_items_complete_for_milestone via scalar milestone_ref")
	}
}

