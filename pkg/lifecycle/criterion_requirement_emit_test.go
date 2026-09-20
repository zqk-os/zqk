package lifecycle

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestTryEmitAllCriteriaCompleteForRequirement_AppendsCriterionSatisfied(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "REQ-TEST-001",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       statusActive,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-REQ-1", "CRIT-REQ-2"},
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-REQ-1",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-REQ-2",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: statusComplete,
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitAllCriteriaCompleteForRequirement(ctx, projectRoot, "REQ-TEST-001", getStorage)

	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		t.Fatalf("GetOrCreateLifecycleWAL: %v", err)
	}

	found := false
	replayErr := wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		if ev.EventType == EventTypeCriterionSatisfied &&
			ev.CriterionID == criterionAllCriteriaCompleteForRequirement &&
			ev.Scope[scopeRequirementID] == "REQ-TEST-001" {
			found = true
		}
		return nil
	})
	if replayErr != nil {
		t.Fatalf("ReplayFrom: %v", replayErr)
	}
	if !found {
		t.Fatal("expected criterionAllCriteriaCompleteForRequirement event in WAL")
	}

	reqObj, err := realStorage.Read(sysCtx, secCtx, "REQ-TEST-001")
	if err != nil {
		t.Fatalf("read requirement: %v", err)
	}
	if got := reqObj[objects.FieldKeyStatus]; got != statusComplete {
		t.Fatalf("requirement status = %v, want %s", got, statusComplete)
	}
}

func TestTryEmitForRequirementsContainingCriterion_LoadsViaCriteriaRefsIndex(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := context.Background()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, projectRoot)
	sysCtx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:           "REQ-TEST-002",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       statusActive,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-REQ-LINK"},
	})
	mustCreateCASVisible(t, realStorage, sysCtx, secCtx, map[string]any{
		objects.FieldKeyID:     "CRIT-REQ-LINK",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: statusComplete,
	})

	getStorage := func(string) (storage.ObjectStorageProvider, bool) { return realStorage, true }

	TryEmitForRequirementsContainingCriterion(ctx, projectRoot, "CRIT-REQ-LINK", getStorage)

	reqObj, err := realStorage.Read(sysCtx, secCtx, "REQ-TEST-002")
	if err != nil {
		t.Fatalf("read requirement: %v", err)
	}
	if got := reqObj[objects.FieldKeyStatus]; got != statusComplete {
		t.Fatalf("requirement status = %v, want %s", got, statusComplete)
	}
}
