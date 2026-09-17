package lifecycle

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestTryEmitAllAcceptanceCriteriaMetForBacklogItem_AppendsCriterionSatisfied(t *testing.T) {
	// TRACK: BLI-REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "BLI-1",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       objects.ObjectStatusInProgress,
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
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, projectRoot, "BLI-1", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllAcceptanceCriteriaMetForBacklogItem &&
			ev.Scope[scopeBacklogItemID] == "BLI-1" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected criterion_satisfied event for all_acceptance_criteria_met_for_backlog_item")
	}
}

func TestTryEmitForBacklogItemsContainingCriterion_LoadsViaCriteriaRefsIndex(t *testing.T) {
	// TRACK: BLI-REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-link",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "BLI-target",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       objects.ObjectStatusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-link"},
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitForBacklogItemsContainingCriterion(ctx, projectRoot, "CRIT-link", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllAcceptanceCriteriaMetForBacklogItem &&
			ev.Scope[scopeBacklogItemID] == "BLI-target" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected backlog criterion satisfied via backlog_item.criteria_refs reverse index")
	}
}
