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
	t.Parallel()

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 "ITEM-1",
		objects.FieldKeyKind:               objects.KindBacklogItem,
		objects.FieldKeyStatus:             "in_progress",
		objects.FieldKeyAcceptanceCriteria: []any{"CRIT-1", "CRIT-2"},
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-1",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-2",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: "complete",
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, projectRoot, "ITEM-1", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllAcceptanceCriteriaMetForBacklogItem &&
			ev.Scope[scopeBacklogItemID] == "ITEM-1" {
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

func TestTryEmitForBacklogItemsContainingCriterion_LoadsBacklogRefs(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 "CRIT-link",
		objects.FieldKeyKind:               objects.KindCriteria,
		objects.FieldKeyStatus:             objects.ObjectStatusValidated,
		objects.FieldKeyBacklogItemRefs:    []any{"ITEM-target"},
		objects.FieldKeyCriteriaRefs:       nil,
		objects.FieldKeyAcceptanceCriteria: nil,
	})
	_ = realStorage.Create(sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 "ITEM-target",
		objects.FieldKeyKind:               objects.KindBacklogItem,
		objects.FieldKeyStatus:             "in_progress",
		objects.FieldKeyAcceptanceCriteria: []any{"CRIT-link"},
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
			ev.Scope[scopeBacklogItemID] == "ITEM-target" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected backlog criterion satisfied via criterion.backlog_item_refs path")
	}
}

func TestCritIDsFromBacklogAcceptanceList_IgnoresFreeformBlock(t *testing.T) {
	t.Parallel()
	raw := []any{
		"CRIT-abc",
		"Informative reference — not a criterion id",
		"CRIT-def\nextra line ignored",
	}
	got := critIDsFromBacklogAcceptanceList(raw)
	if len(got) != 2 || got[0] != "CRIT-abc" || got[1] != "CRIT-def" {
		t.Fatalf("got %#v", got)
	}
}
