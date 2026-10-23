package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCycle142_Registry_Coverage(t *testing.T) {
	AppendReferenceLinked("", "backlog_item", "BLI-1", "priority_plan", "PRI-1", "plan_ref")
	appendLifecycleEvent("", nil)

	require.NoError(t, CloseLifecycleWAL(""))
	_, err := GetOrCreateLifecycleWAL("")
	require.Error(t, err)

	d1 := t.TempDir()
	d2 := t.TempDir()
	w1, err1 := GetOrCreateLifecycleWAL(d1)
	require.NoError(t, err1)
	require.NotNil(t, w1)
	w2, err2 := GetOrCreateLifecycleWAL(d2)
	require.NoError(t, err2)
	require.NotNil(t, w2)

	require.NoError(t, ResetLifecycleWALCache())
}

func createTestLifecycleWAL(t *testing.T, dir string) *LifecycleEventWAL {
	t.Helper()
	wal := createTestLifecycleWAL(t, dir)
	return wal
}

func newTestCriterionEnvironment(t *testing.T) (string, *mockStorageProvider, context.Context) {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { require.NoError(t, CloseLifecycleWAL(dir)) })
	return dir, newMockStorageProvider(), context.Background()
}

func TestCycle142_LifecycleEventWAL_Elevation(t *testing.T) {
	now := time.Now()
	ev1 := &LifecycleEvent{Ts: now}
	require.Equal(t, now, ev1.Timestamp())

	ev2 := &LifecycleEvent{}
	require.False(t, ev2.Timestamp().IsZero())

	var evNil *LifecycleEvent
	require.False(t, evNil.Timestamp().IsZero())

	walEmpty := &LifecycleEventWAL{}
	require.NoError(t, walEmpty.Close())

	dir := t.TempDir()
	walDir := filepath.Join(dir, paths.ProjectDataDir, paths.WalDir)
	require.NoError(t, fileutil.MkdirAll(walDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(walDir, legacyLifecycleWALFileName), []byte(""), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(walDir, legacyLifecycleWALFileName+lifecycleWALCheckpointExt), []byte(""), paths.FilePerm644))

	wal := createTestLifecycleWAL(t, dir)
	require.NoError(t, wal.Append(nil))

	canonical := filepath.Join(walDir, lifecycleWALFileName)
	_, errStat := fileutil.Stat(canonical)
	require.NoError(t, errStat)
	_, errStatCk := fileutil.Stat(canonical + lifecycleWALCheckpointExt)
	require.NoError(t, errStatCk)
}

func TestCycle142_PollLifecycleWAL_Elevation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	PollLifecycleWAL(ctx, t.TempDir(), 0, nil, nil)
	PollLifecycleWAL(context.Background(), "", 0, nil, nil)

	dir := t.TempDir()
	wal, err := GetOrCreateLifecycleWAL(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, CloseLifecycleWAL(dir)) })
	require.NotNil(t, wal)

	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelTimeout()
	PollLifecycleWAL(ctxTimeout, dir, -1, nil, nil)
}

func TestCycle142_Waker_Elevation(t *testing.T) {
	r := NewJobWakerRegistry()
	require.Equal(t, 0, r.Dispatch(nil))
	require.Equal(t, 0, r.Dispatch(&LifecycleEvent{EventType: EventTypeCriterionSatisfied}))

	ch, unreg := r.Register("job-1")
	wch, wunreg := r.RegisterWildcard()
	require.Equal(t, 2, r.Dispatch(&LifecycleEvent{ID: "job-1", EventType: EventTypeSchedulerCallback}))

	ev1 := <-ch
	require.Equal(t, "job-1", ev1.ID)
	ev2 := <-wch
	require.Equal(t, "job-1", ev2.ID)

	r.Unregister("job-1", ch)
	r.Unregister("", wch)
	unreg()
	wunreg()

	require.NoError(t, r.Wake(context.Background(), &LifecycleEvent{ID: "job-2"}))

	r.Close()
	r.Close()
	require.Equal(t, 0, r.Dispatch(&LifecycleEvent{ID: "job-1", EventType: EventTypeSchedulerCallback}))

	closedCh, closedUnreg := r.Register("job-after-close")
	closedUnreg()
	_, ok := <-closedCh
	require.False(t, ok)

	closedWCh, closedWUnreg := r.RegisterWildcard()
	closedWUnreg()
	_, ok = <-closedWCh
	require.False(t, ok)
}

func TestCycle142_Listener_Elevation(t *testing.T) {
	fwNil := &FuncCallbackWaker{}
	require.NoError(t, fwNil.Wake(context.Background(), nil))

	fwErr := &FuncCallbackWaker{fn: func(ctx context.Context, ev *LifecycleEvent) error {
		return errors.New("boom")
	}}

	dir := t.TempDir()
	wal := createTestLifecycleWAL(t, dir)

	trCh := make(chan TransitionRequest, 10)
	mockStore := newMockStorageProvider()
	l := NewListener(dir, wal, DefaultTransitionRules(), trCh, func(root string) (storage.ObjectStorageProvider, bool) {
		return mockStore, true
	})

	l.RegisterWaker(nil)
	l.RegisterWaker(fwErr)
	errs := l.DispatchCallbackNow(context.Background(), &LifecycleEvent{EventType: EventTypeSchedulerCallback})
	require.Len(t, errs, 1)

	l.UnregisterWaker(fwErr.Name())
	require.Empty(t, l.Wakers())
}

func TestCycle142_Listener_DedupAndRules(t *testing.T) {
	m := make(map[string]struct{})
	for i := 0; i < 20; i++ {
		m[fmt.Sprintf("k%d", i)] = struct{}{}
	}
	pruneDedupMap(m, 10)
	require.Equal(t, 10, len(m))

	fullCh := make(chan TransitionRequest, 1)
	fullCh <- TransitionRequest{Kind: "dummy"}
	l := &Listener{
		transitionCh: fullCh,
		rules:        DefaultTransitionRules(),
		fired:        make(map[string]struct{}),
	}
	l.tryFireRulesLocked(criterionAllBacklogComplete, "plan_id=PRI-1", map[string]string{scopePlanID: "PRI-1"})
	require.Empty(t, l.fired)
}

func TestCycle142_Listener_Checkpoint_Elevation(t *testing.T) {
	dir := t.TempDir()
	wal := createTestLifecycleWAL(t, dir)

	l := NewListener(dir, wal, nil, nil, nil)
	require.NoError(t, l.loadCheckpoint())
	require.NoError(t, l.saveCheckpoint())
	require.NoError(t, l.loadCheckpoint())

	require.NoError(t, fileutil.WriteFile(l.checkpointPath, []byte("invalid-checkpoint"), paths.FilePerm600))
	require.Error(t, l.loadCheckpoint())
}

func TestCycle142_Updater_Elevation(t *testing.T) {
	_, ok1 := resolveStorageProvider(nil, "root")
	require.False(t, ok1)

	_, ok2 := resolveStorageProvider(func(r string) (storage.ObjectStorageProvider, bool) {
		return nil, false
	}, "root")
	require.False(t, ok2)

	mockStore := newMockStorageProvider()
	p, ok3 := resolveStorageProvider(func(r string) (storage.ObjectStorageProvider, bool) {
		return mockStore, true
	}, "root")
	require.True(t, ok3)
	require.NotNil(t, p)

	dir := t.TempDir()
	wal := createTestLifecycleWAL(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := RunListenerAndUpdater(ctx, dir, wal, func(r string) (storage.ObjectStorageProvider, bool) {
		return mockStore, true
	})
	time.Sleep(50 * time.Millisecond)
	stop()
}

func TestCycle142_Criterion_Evaluation(t *testing.T) {
	dir := t.TempDir()
	appendAndSyncCriterionSatisfied(dir, "crit-1", map[string]string{"a": "b"})
	require.NoError(t, CloseLifecycleWAL(dir))
	appendAndSyncCriterionSatisfied("", "crit-1", nil)

	mockStore := newMockStorageProvider()
	ctx := context.Background()
	require.False(t, areAllLinkedCriteriaSatisfied(ctx, mockStore, nil))
	require.False(t, areAllLinkedCriteriaSatisfied(ctx, mockStore, []string{"CRIT-NOT-FOUND"}))

	mockStore.add(map[string]any{
		objects.FieldKeyID:     "CRIT-UNSAT",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: "draft",
	})
	require.False(t, areAllLinkedCriteriaSatisfied(ctx, mockStore, []string{"CRIT-UNSAT"}))

	mockStore.add(map[string]any{
		objects.FieldKeyID:     "CRIT-SAT",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: "validated",
	})
	require.True(t, areAllLinkedCriteriaSatisfied(ctx, mockStore, []string{"CRIT-SAT"}))
}

func TestCycle142_Criterion_EmitBacklogAndRequirement(t *testing.T) {
	dir, mockStore, ctx := newTestCriterionEnvironment(t)
	storeFn := func(r string) (storage.ObjectStorageProvider, bool) { return mockStore, true }

	mockStore.add(map[string]any{
		objects.FieldKeyID:     "REQ-1",
		objects.FieldKeyKind:   objects.KindRequirement,
		objects.FieldKeyStatus: "in_progress",
	})
	TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, dir, "REQ-1", storeFn)

	mockStore.add(map[string]any{
		objects.FieldKeyID:     "BLI-1",
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: "draft",
	})
	TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, dir, "BLI-1", storeFn)

	mockStore.add(map[string]any{
		objects.FieldKeyID:     "CRIT-1",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: "validated",
	})
	mockStore.add(map[string]any{
		objects.FieldKeyID:           "BLI-2",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       "in_progress",
		objects.FieldKeyCriteriaRefs: []string{"CRIT-1"},
	})
	TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, dir, "BLI-2", storeFn)

	TryEmitAllCriteriaCompleteForRequirement(ctx, dir, "BLI-2", storeFn)
	mockStore.add(map[string]any{
		objects.FieldKeyID:     "REQ-DONE",
		objects.FieldKeyKind:   objects.KindRequirement,
		objects.FieldKeyStatus: "complete",
	})
	TryEmitAllCriteriaCompleteForRequirement(ctx, dir, "REQ-DONE", storeFn)

	mockStore.add(map[string]any{
		objects.FieldKeyID:           "REQ-2",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       "in_progress",
		objects.FieldKeyCriteriaRefs: []string{"CRIT-1"},
	})
	mockStore.add(map[string]any{
		objects.FieldKeyID:              "TST-1",
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyStatus:          "in_progress",
		objects.FieldKeyRequirementRefs: []string{"REQ-2"},
	})
	TryEmitAllCriteriaCompleteForRequirement(ctx, dir, "REQ-2", storeFn)

	mockStore.add(map[string]any{
		objects.FieldKeyID:              "TST-1",
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyStatus:          "complete",
		objects.FieldKeyRequirementRefs: []string{"REQ-2"},
	})
	TryEmitAllCriteriaCompleteForRequirement(ctx, dir, "REQ-2", storeFn)
}

func TestCycle142_RemainingOpenCount_Elevation(t *testing.T) {
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newMockStorageProvider()
	ctx := context.Background()

	_, err := casAdjustRemainingOpenCount(ctx, nil, secCtx, "P1", 1)
	require.Error(t, err)
	_, err = casAdjustRemainingOpenCount(ctx, mockStore, secCtx, "", 1)
	require.Error(t, err)
	_, err = casAdjustRemainingOpenCount(ctx, mockStore, secCtx, "NOTFOUND", 1)
	require.Error(t, err)

	mockStore.add(map[string]any{objects.FieldKeyID: "P-UNSET"})
	_, err = casAdjustRemainingOpenCount(ctx, mockStore, secCtx, "P-UNSET", 1)
	require.Error(t, err)

	mockStore.add(map[string]any{objects.FieldKeyID: "P-ZERO", objects.FieldKeyRemainingOpenCount: 0})
	n, err := casAdjustRemainingOpenCount(ctx, mockStore, secCtx, "P-ZERO", -1)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	mockStore.add(map[string]any{objects.FieldKeyID: "P-TWO", objects.FieldKeyRemainingOpenCount: 2})
	n, err = casAdjustRemainingOpenCount(ctx, mockStore, secCtx, "P-TWO", -1)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	NoteOpenCountableMemberEntered(ctx, nil, "P")
	NoteOpenCountableMemberEntered(ctx, mockStore, "")
	NoteOpenCountableMemberEntered(ctx, mockStore, "P-TWO")

	dir := t.TempDir()
	t.Cleanup(func() { require.NoError(t, CloseLifecycleWAL(dir)) })
	EmitTrustedRemainingOpenDrained("", "P")
	EmitTrustedRemainingOpenDrained(dir, "")
	EmitTrustedRemainingOpenDrained(dir, "MIL-1")
	EmitTrustedRemainingOpenDrained(dir, "PRI-1")
}

func TestCycle142_DependencyPropagation_Elevation(t *testing.T) {
	require.True(t, planStatusCanOccupancyLock("active"))
	require.True(t, planStatusCanOccupancyLock("grooming"))
	require.False(t, planStatusCanOccupancyLock("ready"))
	require.False(t, planStatusCanOccupancyLock("cancelled"))

	up, ok := autoCompleteUpdates("backlog_item", "in_progress")
	require.True(t, ok)
	require.Equal(t, "complete", up[objects.FieldKeyStatus])

	_, ok = autoCompleteUpdates("unknown_kind", "in_progress")
	require.False(t, ok)

	opt := &MembershipRemovedOptions{}
	WithPark(true)(opt)
	require.True(t, opt.Park)
	WithPark(false)(opt)
	require.False(t, opt.Park)

	planUp := make(map[string]any)
	applyRoleTransitionSideEffects(objects.KindPriorityPlan, "draft", "in_progress", planUp)
	require.Equal(t, storage.FieldUnset, planUp[objects.FieldKeyActiveOrder])

	bliUp := make(map[string]any)
	applyRoleTransitionSideEffects(objects.KindBacklogItem, "draft", "in_progress", bliUp)
	require.Empty(t, bliUp)
}

func TestCycle142_DependencyPropagation_Commit(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewEventLogger(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()
	mockStore := newMockStorageProvider()

	mockStore.add(map[string]any{
		objects.FieldKeyID:     "PRI-COMM",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: "draft",
	})

	ev := DependencyRefEvent{
		EventID:   "EV-DEP-1",
		TargetID:  "PRI-COMM",
		TriggerID: "BLI-1",
		FromState: "draft",
		ToState:   "in_progress",
	}
	commitMembershipTransition(ctx, logger, mockStore, secCtx, ev, objects.KindPriorityPlan, "draft", "in_progress", map[string]any{
		objects.FieldKeyStatus: "in_progress",
	})
	commitMembershipTransition(ctx, logger, mockStore, secCtx, ev, objects.KindPriorityPlan, "draft", "in_progress", map[string]any{
		objects.FieldKeyStatus: "in_progress",
	})

	evShock := DependencyRefEvent{
		EventID:   "EV-SHOCK-1",
		TargetID:  "PRI-COMM",
		TriggerID: "BLI-1",
		FromState: "draft",
		ToState:   "in_progress",
	}
	commitPriorityPlanShockwave(ctx, logger, mockStore, secCtx, evShock, "draft", map[string]any{
		objects.FieldKeyStatus: "in_progress",
	})
	commitPriorityPlanShockwave(ctx, logger, mockStore, secCtx, evShock, "draft", map[string]any{
		objects.FieldKeyStatus: "in_progress",
	})
}

func TestCycle142_ShockwaveResolve_Elevation(t *testing.T) {
	require.Equal(t, 100, lineageRank("unknown_kind"))

	p := objects.ShockwavePolicy{}
	require.True(t, pruneAcceptsKind("goal", "milestone", p))

	p.LineageKinds = []string{"goal", "milestone"}
	require.True(t, pruneAcceptsKind("goal", "milestone", p))
	require.False(t, pruneAcceptsKind("milestone", "goal", p))

	pNil := ResolveShockwavePolicy(nil, "draft", "complete")
	require.Empty(t, pNil.Mode)

	lc := &objects.Lifecycle{
		Statuses: []objects.Status{
			{Value: "archived", Shockwave: objects.ShockwavePolicy{Mode: objects.ShockwaveModePrune}},
		},
	}
	pArch := ResolveShockwavePolicy(lc, "active", "archived")
	require.Equal(t, objects.ShockwaveModePrune, pArch.Mode)

	pPark := ResolveShockwavePolicy(nil, "active", "archived")
	require.Equal(t, objects.ShockwaveModeCluster, pPark.Mode)
}

func TestCycle142_StageMembrane_Elevation(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	blocked := func(id, reason string) error { return fmt.Errorf("blocked %s: %s", id, reason) }

	err := rejectArchivedBacklogUnderLivePlan(ctx, secCtx, nil, nil, nil, "complete", blocked)
	require.NoError(t, err)

	members := []MembraneMember{{ID: "BLI-1", Kind: objects.KindBacklogItem}}
	loaded := map[string]map[string]any{"BLI-1": {objects.FieldKeyID: "BLI-1", objects.FieldKeyPriorityPlanRef: ""}}
	err = rejectArchivedBacklogUnderLivePlan(ctx, secCtx, nil, loaded, members, "archived", blocked)
	require.NoError(t, err)

	mockStore := newMockStorageProvider()
	err = rejectArchivedBacklogUnderLivePlan(ctx, secCtx, mockStore, nil, members, "archived", blocked)
	require.Error(t, err)

	mockStore.add(map[string]any{objects.FieldKeyID: "BLI-1", objects.FieldKeyPriorityPlanRef: "PRI-1"})
	err = rejectArchivedBacklogUnderLivePlan(ctx, secCtx, mockStore, nil, members, "archived", blocked)
	require.Error(t, err)

	mockStore.add(map[string]any{objects.FieldKeyID: "PRI-1", objects.FieldKeyStatus: ""})
	err = rejectArchivedBacklogUnderLivePlan(ctx, secCtx, mockStore, nil, members, "archived", blocked)
	require.Error(t, err)

	mockStore.add(map[string]any{objects.FieldKeyID: "PRI-1", objects.FieldKeyStatus: "in_progress"})
	err = rejectArchivedBacklogUnderLivePlan(ctx, secCtx, mockStore, nil, members, "archived", blocked)
	require.Error(t, err)

	mockStore.add(map[string]any{objects.FieldKeyID: "PRI-1", objects.FieldKeyStatus: "archived"})
	err = rejectArchivedBacklogUnderLivePlan(ctx, secCtx, mockStore, nil, members, "archived", blocked)
	require.NoError(t, err)
}

func TestCycle142_RemainingOpenCount_More(t *testing.T) {
	mockStore := newMockStorageProvider()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	mockStore.add(map[string]any{
		objects.FieldKeyID:        "PLAN-UP",
		objects.FieldKeyUpdatedAt: "2026-01-01T00:00:00Z",
	})
	n, err := casWriteRemainingOpenCount(ctx, mockStore, secCtx, "PLAN-UP", 5)
	require.NoError(t, err)
	require.Equal(t, 5, n)

	_, err = casWriteRemainingOpenCount(ctx, mockStore, secCtx, "NOT-EXIST", 5)
	require.Error(t, err)

	rem, handled, err := consumeOpenChild(ctx, nil, secCtx, DependencyRefEvent{})
	require.NoError(t, err)
	require.False(t, handled)
	require.Equal(t, 0, rem)

	ev := DependencyRefEvent{TargetID: "PLAN-UP"}
	rem, handled, err = consumeOpenChild(ctx, mockStore, secCtx, ev)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, 4, rem)

	evUnset := DependencyRefEvent{TargetID: "PLAN-UNSET"}
	mockStore.add(map[string]any{objects.FieldKeyID: "PLAN-UNSET"})
	rem, handled, err = consumeOpenChild(ctx, mockStore, secCtx, evUnset)
	require.NoError(t, err)
	require.False(t, handled)

	noteOpenCountableReentered(ctx, nil, "")
	noteOpenCountableReentered(ctx, mockStore, "PLAN-UP")
}

func TestCycle142_Criterion_More(t *testing.T) {
	refs, ok := criteriaRefsFromObject(map[string]any{})
	require.False(t, ok)
	require.Nil(t, refs)

	refs, ok = criteriaRefsFromObject(map[string]any{objects.FieldKeyCriteriaRefs: nil})
	require.False(t, ok)

	refs, ok = criteriaRefsFromObject(map[string]any{
		objects.FieldKeyCriteriaRefs: []string{"CRIT-A", "CRIT-B"},
	})
	require.True(t, ok)
	require.Len(t, refs, 2)

	dir, mockStore, ctx := newTestCriterionEnvironment(t)

	mockStore.add(map[string]any{
		objects.FieldKeyID:                 "P-DRAIN",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyRemainingOpenCount: 0,
	})
	TryEmitRemainingOpenDrained(ctx, dir, "P-DRAIN", func(r string) (storage.ObjectStorageProvider, bool) {
		return mockStore, true
	})

	mockStore.add(map[string]any{
		objects.FieldKeyID:                 "P-NONZERO",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyRemainingOpenCount: 2,
	})
	TryEmitRemainingOpenDrained(ctx, dir, "P-NONZERO", func(r string) (storage.ObjectStorageProvider, bool) {
		return mockStore, true
	})
}

func TestCycle142_DependencyPropagation_More(t *testing.T) {
	require.Equal(t, "backlog_item", resolveObjectKind(nil, "backlog_item"))
	require.Equal(t, "backlog_item", resolveObjectKind(map[string]any{}, "backlog_item"))
	require.Equal(t, "custom_kind", resolveObjectKind(map[string]any{
		objects.FieldKeyKind: "custom_kind",
	}, "default_kind"))

	lc := loadGlobalLifecycle("unknown_dummy_kind")
	require.NotNil(t, lc)

	lcBli := loadGlobalLifecycle(objects.KindBacklogItem)
	require.NotNil(t, lcBli)
}

