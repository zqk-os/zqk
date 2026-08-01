package lifecycle

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

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
	t.Parallel()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "active",
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
			objects.FieldKeyTargetID: "PLAN-1",
			"trigger_id":             "ITEM-1",
			"trigger_kind":           "backlog_item",
			"from_state":             "complete",
			"to_state":               "complete", // terminal -> no propagation
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, "PLAN-1")
	if obj[objects.FieldKeyStatus] != "active" {
		t.Error("HandleEvent with terminal to_state should not update target")
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_UpdatesWhenNonTerminal(t *testing.T) {
	t.Parallel()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "active",
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
			objects.FieldKeyTargetID: "PLAN-1",
			"trigger_id":             "ITEM-1",
			"trigger_kind":           "backlog_item",
			"from_state":             "complete",
			"to_state":               "in_progress", // non-terminal -> priority_plan parent to grooming
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, "PLAN-1")
	if obj[objects.FieldKeyStatus] != "grooming" {
		t.Errorf("HandleEvent: expected Update(PLAN-1, status=grooming), got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_SkipsWhenAlreadyGrooming(t *testing.T) {
	t.Parallel()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "grooming",
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
			objects.FieldKeyTargetID: "PLAN-1",
			"trigger_id":             "ITEM-1",
			"to_state":               "in_progress",
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, "PLAN-1")
	if obj[objects.FieldKeyStatus] != "grooming" {
		t.Errorf("target already grooming: expected no Update, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_SkipsTerminalPriorityPlan(t *testing.T) {
	t.Parallel()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "PLAN-1", objects.FieldKeyKind: "priority_plan", objects.FieldKeyStatus: "complete",
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
			objects.FieldKeyTargetID: "PLAN-1",
			"trigger_id":             "ITEM-1",
			"trigger_kind":           "backlog_item",
			"from_state":             "complete",
			"to_state":               "in_progress",
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, "PLAN-1")
	if obj[objects.FieldKeyStatus] != "complete" {
		t.Fatalf("terminal parent: expected no Update, got %v", obj[objects.FieldKeyStatus])
	}
}

func TestDependencyPropagationSubscriber_HandleEvent_SkipsNonPriorityPlanTarget(t *testing.T) {
	t.Parallel()
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	_ = realStorage.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID: "MIL-1", objects.FieldKeyKind: objects.KindMilestone, objects.FieldKeyStatus: "active",
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
			objects.FieldKeyTargetID: "MIL-1",
			"trigger_id":             "ITEM-1",
			"trigger_kind":           "backlog_item",
			"from_state":             "complete",
			"to_state":               "in_progress",
		},
	}
	_ = sub.HandleEvent(event)

	obj, _ := realStorage.Read(ctx, secCtx, "MIL-1")
	if obj[objects.FieldKeyStatus] != "active" {
		t.Fatalf("v1 propagation is priority_plan-only: expected no Update, got %v", obj[objects.FieldKeyStatus])
	}
}
