package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

type synchronousRecordingCoordinator struct {
	asyncCalls int
	syncCalls  int
}

func (c *synchronousRecordingCoordinator) Emit(context.Context, *coordination.EventContext) error {
	c.asyncCalls++
	return nil
}

func (c *synchronousRecordingCoordinator) EmitOperationalSync(context.Context, *coordination.EventContext) error {
	c.syncCalls++
	return nil
}

func (*synchronousRecordingCoordinator) Subscribe(coordination.OperationalEventSubscriber) string {
	return ""
}

func (*synchronousRecordingCoordinator) Unsubscribe(string) {}

func TestEmitDependencyRefEventUsesSynchronousOperationalDelivery(t *testing.T) {
	t.Parallel()
	coord := &synchronousRecordingCoordinator{}
	version := "2030-01-01T00:00:00.000000001Z"

	emitDependencyRefEvent(coord, context.Background(), DependencyRefEvent{
		ProjectRoot: "/project",
		EventID:     dependencyEventID("PRI-1", "BLI-1", "planned", "in_progress", version),
		Version:     version,
		TargetID:    "PRI-1",
		TriggerID:   "BLI-1",
		TriggerKind: objects.KindBacklogItem,
		FromState:   "planned",
		ToState:     "in_progress",
	})

	if coord.syncCalls != 1 {
		t.Fatalf("synchronous calls = %d, want 1", coord.syncCalls)
	}
	if coord.asyncCalls != 0 {
		t.Fatalf("asynchronous calls = %d, want 0", coord.asyncCalls)
	}
}

func TestReserveDependencyEventRejectsDuplicateAndStale(t *testing.T) {
	t.Parallel()
	targetID := fixtureID(t, "PRI")
	triggerID := fixtureID(t, "BLI")
	newer := DependencyRefEvent{
		EventID:   "newer",
		Version:   "2030-01-01T00:00:00.000000002Z",
		TargetID:  targetID,
		TriggerID: triggerID,
	}
	if !reserveDependencyEvent(newer) {
		t.Fatal("new event must be accepted")
	}
	if reserveDependencyEvent(newer) {
		t.Fatal("duplicate event must be rejected")
	}
	stale := newer
	stale.EventID = "stale"
	stale.Version = "2030-01-01T00:00:00.000000001Z"
	if reserveDependencyEvent(stale) {
		t.Fatal("stale event must be rejected")
	}
}

func TestDependencyRefTargetIDsIncludesPriorityPlanParent(t *testing.T) {
	t.Parallel()

	got := dependencyRefTargetIDs("BLI-1", map[string]any{
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	})

	if len(got) != 1 || got[0] != "PRI-1" {
		t.Fatalf("target IDs = %#v, want [PRI-1]", got)
	}
}

func TestDependencyHopMetaTagsMembership(t *testing.T) {
	t.Parallel()
	field, role := dependencyHopMeta(objects.KindBacklogItem, "PRI-1", map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	})
	if field != objects.FieldKeyPriorityPlanRef {
		t.Fatalf("field = %q, want priority_plan_ref", field)
	}
	if role != objects.EdgeRoleMembership {
		t.Fatalf("role = %s, want membership", role)
	}
}

func TestDependencyPropagationSubscriber_IDAndEventTypes(t *testing.T) {
	t.Parallel()
	sub := NewDependencyPropagationSubscriber(func(string) (storage.ObjectStorageProvider, bool) { return nil, false })
	if sub.ID() != "lifecycle_dependency_propagation" {
		t.Errorf("ID() = %q, want lifecycle_dependency_propagation", sub.ID())
	}
	types := sub.EventTypes()
	if len(types) != 1 || types[0] != coordination.EventTypeLifecycleDependencyRef {
		t.Errorf("EventTypes() = %v", types)
	}
	if !sub.IsActive() {
		t.Error("IsActive() = false, want true")
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_NoUpdateWhenTerminal(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyActiveOrder: 1,
	})
	// The lifecycle event may arrive before the triggering child's new CAS/index row is
	// visible to this provider. The event's to_state is authoritative in that window.
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliID, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned, objects.FieldKeyPriorityPlanRef: planID,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	event := &coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliID,
			"trigger_kind":           "backlog_item",
			"from_state":             "complete",
			"to_state":               "complete", // terminal -> no propagation
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "active" {
		t.Error("HandleEvent with terminal to_state should not update target")
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_SkipLockWhenSiblingNotReadyOrLater(t *testing.T) {
	// TRACK: REDACTED — airtight lock; REDACTED — no t.Parallel.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliWorking := fixtureID(t, "BLI-w")
	bliCreep := fixtureID(t, "BLI-c")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyActiveOrder: 1,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliWorking, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyPriorityPlanRef: planID,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliCreep, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeyPriorityPlanRef: planID,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliWorking,
			"trigger_kind":           "backlog_item",
			"from_state":             "planned",
			"to_state":               "in_progress",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "active" {
		t.Errorf("ready-or-later gate must skip →in_progress while exploring sibling linked, got %v", obj[objects.FieldKeyStatus])
	}
	if obj[objects.FieldKeyActiveOrder] == nil {
		t.Errorf("active_order must remain when lock is skipped, got %v", obj[objects.FieldKeyActiveOrder])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_OccupancyLockWhenLastDraftSiblingClears(t *testing.T) {
	// TRACK: REDACTED — lock is skipped while a sibling is
	// validated; promoting that sibling to planned must re-evaluate occupancy.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliWorking := fixtureID(t, "BLI-w")
	bliDraft := fixtureID(t, "BLI-d")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyActiveOrder: 2,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliWorking, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyPriorityPlanRef: planID,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliDraft, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned, objects.FieldKeyPriorityPlanRef: planID,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliDraft,
			"trigger_kind":           "backlog_item",
			"from_state":             "validated",
			"to_state":               "planned",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("occupancy lock after last draft sibling → planned, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestMaybeExecutionLockPlan_LocksActiveWhenChildrenAlreadyInFlight(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliWorking := fixtureID(t, "BLI-w")
	bliReady := fixtureID(t, "BLI-r")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyActiveOrder: 1,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliWorking, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyPriorityPlanRef: planID,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliReady, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned, objects.FieldKeyPriorityPlanRef: planID,
	})

	if !MaybeExecutionLockPlan(ctx, logging.NewEventLogger(ctx), realStorage, t.TempDir(), planID) {
		t.Fatal("expected occupancy lock after seal-with-children-in-flight")
	}
	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("MaybeExecutionLockPlan status = %v, want in_progress", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_LocksWhenAllSiblingsReadyOrLater(t *testing.T) {
	// TRACK: REDACTED
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliWorking := fixtureID(t, "BLI-w")
	bliReady := fixtureID(t, "BLI-r")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		objects.FieldKeyActiveOrder: 1,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliWorking, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress, objects.FieldKeyPriorityPlanRef: planID,
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: bliReady, objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyStatus: objects.ObjectStatusPlanned, objects.FieldKeyPriorityPlanRef: planID,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliWorking,
			"trigger_kind":           "backlog_item",
			"from_state":             "planned",
			"to_state":               "in_progress",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("all ready-or-later siblings should allow lock, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_BLIInProgressLocksActivePlan(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyActiveOrder: 1,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	event := &coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliID,
			"trigger_kind":           "backlog_item",
			"from_state":             "planned",
			"to_state":               "in_progress",
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("expected plan in_progress after child starts work, got %v", obj[objects.FieldKeyStatus])
	}
	if obj[objects.FieldKeyActiveOrder] != nil {
		t.Errorf("expected active_order cleared, got %v", obj[objects.FieldKeyActiveOrder])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_BLIInProgressRepairsGroomingPlan(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		objects.FieldKeyActiveOrder: 1,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliID,
			"trigger_kind":           "backlog_item",
			"from_state":             "planned",
			"to_state":               "in_progress",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("grooming+in_progress child should repair to in_progress, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_ReopenedExploringReturnsToGrooming(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliID,
			"trigger_kind":           "backlog_item",
			"from_state":             "planned",
			"to_state":               "exploring",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "grooming" {
		t.Errorf("reopened exploring should realign plan to grooming, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_PlannedChildDoesNotGroom(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyActiveOrder: 1,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliID,
			"trigger_kind":           "backlog_item",
			"from_state":             "exploring",
			"to_state":               "planned",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "active" {
		t.Errorf("planned child must not demote shovel-ready active plan, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_SkipsWhenAlreadyInProgress(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             "BLI-2",
			"trigger_kind":           "backlog_item",
			"to_state":               "in_progress",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "in_progress" {
		t.Errorf("already in_progress: expected no demotion, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_SkipsTerminalPriorityPlan(t *testing.T) {
	// TRACK: REDACTED — no t.Parallel: shared Memgraph + fixed fixture IDs.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := fixtureID(t, "PRI")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: planID, objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: objects.ObjectStatusComplete,
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: planID,
			"trigger_id":             bliID,
			"trigger_kind":           "backlog_item",
			"to_state":               "in_progress",
		},
	})

	obj, _ := realStorage.Read(ctx, secCtx, planID)
	if obj[objects.FieldKeyStatus] != "complete" {
		t.Errorf("terminal plan must not change, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_LocksMilestoneWhenBLIStarts(t *testing.T) {
	// Plane D must interpret status_reactive YAML the same as applyDependencyRefEvent.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	milID := fixtureID(t, "MIL")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              milID,
		objects.FieldKeyKind:            objects.KindMilestone,
		objects.FieldKeyStatus:          objects.ObjectStatusNotStarted,
		objects.FieldKeyEstimatedEffort: "1w",
	})

	getStorage := func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	}
	sub := NewDependencyPropagationSubscriber(getStorage)
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: milID,
			"trigger_id":             bliID,
			"trigger_kind":           objects.KindBacklogItem,
			"from_state":             objects.ObjectStatusPlanned,
			"to_state":               statusInProgress,
		},
	})

	obj, err := realStorage.Read(ctx, secCtx, milID)
	if err != nil {
		t.Fatalf("read milestone: %v", err)
	}
	if obj[objects.FieldKeyStatus] != objects.ObjectStatusInProgress {
		t.Errorf("HandleEvent must lock status_reactive milestone, got %v", obj[objects.FieldKeyStatus])
	}
}

func loadPriorityPlanShockwaveTransitions(t *testing.T) []objects.Transition {
	t.Helper()
	dir := filepath.Join("..", "..", "docs", "process", "_internal", "lifecycles")
	loader := objects.NewLifecycleLoader(dir)
	lc, err := loader.LoadLifecycle(objects.KindPriorityPlan)
	if err != nil || lc == nil {
		t.Fatalf("load priority_plan lifecycle: %v", err)
	}
	return lc.Transitions
}

func TestPriorityPlanShockwaveUpdates_Table(t *testing.T) {
	t.Parallel()
	transitions := loadPriorityPlanShockwaveTransitions(t)
	cases := []struct {
		name        string
		plan, to    string
		triggerKind string
		wantStatus  string
		wantClear   bool
		wantOK      bool
	}{
		// Lock to in_progress + clear active_order (≡ top-of-stack; frees numeric order slots).
		{"active+in_progress", statusActive, statusInProgress, objects.KindBacklogItem, statusInProgress, true, true},
		// grooming → in_progress is pruned skip-seal (rubric remaining candidate). Plane F occupancy lock is not this matcher.
		{"grooming+in_progress", statusGrooming, statusInProgress, objects.KindBacklogItem, "", false, false},
		{"paused+in_progress", statusPaused, statusInProgress, objects.KindBacklogItem, statusInProgress, true, true},
		{"blocked+in_progress", statusBlocked, statusInProgress, objects.KindBacklogItem, statusInProgress, true, true},
		// Idempotent / no demote.
		{"in_progress+in_progress", statusInProgress, statusInProgress, objects.KindBacklogItem, "", false, false},
		{"active+planned", statusActive, "planned", objects.KindBacklogItem, "", false, false},
		{"active+complete", statusActive, statusComplete, objects.KindBacklogItem, "", false, false},
		{"grooming+planned", statusGrooming, "planned", objects.KindBacklogItem, "", false, false},
		// Reopen below shovel-ready → grooming (YAML from active|paused|blocked, not from in_progress).
		{"in_progress+exploring", statusInProgress, statusExploring, objects.KindBacklogItem, "", false, false},
		{"active+validated", statusActive, statusValidated, objects.KindBacklogItem, statusGrooming, true, true},
		{"active+exploring", statusActive, statusExploring, objects.KindBacklogItem, statusGrooming, true, true},
		{"grooming+exploring", statusGrooming, statusExploring, objects.KindBacklogItem, "", false, false},
		// Wrong trigger kind: no shockwave.
		{"non_bli_trigger", statusActive, statusInProgress, "milestone", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, ok := shockwaveUpdatesFromTransitions(transitions, tc.plan, tc.triggerKind, tc.to)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v updates=%v", ok, tc.wantOK, u)
			}
			if !tc.wantOK {
				return
			}
			if u[objects.FieldKeyStatus] != tc.wantStatus {
				t.Fatalf("status=%v want %v", u[objects.FieldKeyStatus], tc.wantStatus)
			}
			if tc.wantClear {
				v, has := u[objects.FieldKeyActiveOrder]
				if !has {
					t.Fatal("expected active_order key in updates")
				}
				if !storage.IsFieldUnset(v) {
					t.Fatalf("expected active_order FieldUnset, got %v", v)
				}
			} else if _, has := u[objects.FieldKeyActiveOrder]; has {
				t.Fatalf("did not expect active_order in updates: %v", u)
			}
		})
	}
}

func TestPriorityPlanShockwaveUpdates_GlobalLoader(t *testing.T) {
	t.Parallel()
	updates, ok := priorityPlanShockwaveUpdates(
		objects.ObjectStatusActive,
		objects.KindBacklogItem,
		objects.ObjectStatusPlanned,
		objects.ObjectStatusInProgress,
	)
	if !ok {
		t.Fatal("global lifecycle loader did not resolve active priority-plan shockwave")
	}
	if updates[objects.FieldKeyStatus] != objects.ObjectStatusInProgress {
		t.Fatalf("status = %v, want in_progress", updates[objects.FieldKeyStatus])
	}
}

func loadKindShockwaveTransitions(t *testing.T, kind string) []objects.Transition {
	t.Helper()
	dir := filepath.Join("..", "..", "docs", "process", "_internal", "lifecycles")
	loader := objects.NewLifecycleLoader(dir)
	lc, err := loader.LoadLifecycle(kind)
	if err != nil || lc == nil {
		t.Fatalf("load %s lifecycle: %v", kind, err)
	}
	return lc.Transitions
}

func TestMilestoneShockwaveUpdates_LinkedBLIInProgress(t *testing.T) {
	t.Parallel()
	transitions := loadKindShockwaveTransitions(t, objects.KindMilestone)
	u, ok := shockwaveUpdatesFromTransitions(transitions, objects.ObjectStatusNotStarted, objects.KindBacklogItem, objects.ObjectStatusInProgress)
	if !ok {
		t.Fatal("expected milestone not_started → in_progress when BLI is in_progress")
	}
	if u[objects.FieldKeyStatus] != objects.ObjectStatusInProgress {
		t.Fatalf("status=%v want in_progress", u[objects.FieldKeyStatus])
	}
}

func TestGoalShockwaveUpdates_LinkedBLIInProgress(t *testing.T) {
	t.Parallel()
	transitions := loadKindShockwaveTransitions(t, objects.KindGoal)
	u, ok := shockwaveUpdatesFromTransitions(transitions, objects.ObjectStatusProposed, objects.KindBacklogItem, objects.ObjectStatusInProgress)
	if !ok {
		t.Fatal("expected goal proposed → active when BLI is in_progress")
	}
	if u[objects.FieldKeyStatus] != objects.ObjectStatusActive {
		t.Fatalf("status=%v want active", u[objects.FieldKeyStatus])
	}
}

func TestApplyDependencyRefEvent_LocksMilestoneWhenLinkedBLIStarts(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	milID := fixtureID(t, "MIL")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              milID,
		objects.FieldKeyKind:            objects.KindMilestone,
		objects.FieldKeyStatus:          objects.ObjectStatusNotStarted,
		objects.FieldKeyEstimatedEffort: "1w",
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyMilestoneRefs: []any{milID},
	})

	applyDependencyRefEvent(ctx, logging.NewEventLogger(ctx), realStorage, DependencyRefEvent{
		ProjectRoot: t.TempDir(),
		EventID:     "test-mil-lock-" + milID,
		Version:     "2030-01-01T00:00:00.000000001Z",
		TargetID:    milID,
		TriggerID:   bliID,
		TriggerKind: objects.KindBacklogItem,
		FromState:   objects.ObjectStatusPlanned,
		ToState:     objects.ObjectStatusInProgress,
	})

	obj, err := realStorage.Read(ctx, secCtx, milID)
	if err != nil {
		t.Fatalf("read milestone: %v", err)
	}
	if obj[objects.FieldKeyStatus] != objects.ObjectStatusInProgress {
		t.Errorf("milestone status = %v, want in_progress", obj[objects.FieldKeyStatus])
	}
}

func TestApplyDependencyRefEvent_LockedBLIDoesNotWalkMilestone(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	milID := fixtureID(t, "MIL")
	bliID := fixtureID(t, "BLI")
	atkID := fixtureID(t, "ATK")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              milID,
		objects.FieldKeyKind:            objects.KindMilestone,
		objects.FieldKeyStatus:          objects.ObjectStatusNotStarted,
		objects.FieldKeyEstimatedEffort: "1w",
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:            bliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyMilestoneRefs: []any{milID},
	})

	applyDependencyRefEvent(ctx, logging.NewEventLogger(ctx), realStorage, DependencyRefEvent{
		ProjectRoot: t.TempDir(),
		EventID:     "test-no-walk-" + bliID,
		Version:     "2030-01-01T00:00:00.000000002Z",
		TargetID:    bliID,
		TriggerID:   atkID,
		TriggerKind: objects.KindAgentTask,
		FromState:   objects.ObjectStatusApproved,
		ToState:     objects.ObjectStatusInProgress,
	})

	obj, err := realStorage.Read(ctx, secCtx, milID)
	if err != nil {
		t.Fatalf("read milestone: %v", err)
	}
	if obj[objects.FieldKeyStatus] != objects.ObjectStatusNotStarted {
		t.Errorf("listener must not walk: milestone status = %v, want not_started", obj[objects.FieldKeyStatus])
	}
}
