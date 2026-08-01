// Package lifecycle: one-level dependency propagation on status toggle.
// When a dependent object's status toggles and saves, we emit dependency ref events via the
// coordinator (operational channel) for one level up (refs the object points to) and one level
// down (dependents that point to it). A subscriber applies v1 rules (e.g. priority_plan → grooming
// when linked backlog work becomes non-terminal); that Update triggers the lifecycle hook again,
// so propagation continues one hop at a time until no further updates apply.

package lifecycle

import (
	"context"
	"strings"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

var dependencySubscriberOnce sync.Once

// RegisterDependencyPropagationWithCoordinator subscribes the dependency propagation handler to the
// global coordinator so lifecycle.dependency_ref events are processed. Call once at startup (e.g. from root).
func RegisterDependencyPropagationWithCoordinator(getStorage StorageProvider) {
	dependencySubscriberOnce.Do(func() {
		coord := coordination.GetCoordinator()
		if coord == nil {
			return
		}
		coord.Subscribe(NewDependencyPropagationSubscriber(getStorage))
	})
}

// DependencyRefEvent is one event: target object should re-evaluate status after trigger's transition.
type DependencyRefEvent struct {
	ProjectRoot string
	TargetID    string
	TriggerID   string
	TriggerKind string
	FromState   string
	ToState     string
}

// DependencyPropagationSubscriber implements coordination.OperationalEventSubscriber and
// handles lifecycle.dependency_ref events by applying one-level status propagation rules.
type DependencyPropagationSubscriber struct {
	getStorage StorageProvider
	id         string
	active     bool
}

// NewDependencyPropagationSubscriber creates a subscriber that processes dependency ref events.
// Register with coordination.GetCoordinator().Subscribe(subscriber) when the scheduler or CLI is ready.
func NewDependencyPropagationSubscriber(getStorage StorageProvider) *DependencyPropagationSubscriber {
	return &DependencyPropagationSubscriber{
		getStorage: getStorage,
		id:         "lifecycle_dependency_propagation",
		active:     true,
	}
}

// ID implements OperationalEventSubscriber.
func (s *DependencyPropagationSubscriber) ID() string { return s.id }

// EventTypes implements OperationalEventSubscriber.
func (s *DependencyPropagationSubscriber) EventTypes() []string {
	return []string{coordination.EventTypeLifecycleDependencyRef}
}

// IsActive implements OperationalEventSubscriber.
func (s *DependencyPropagationSubscriber) IsActive() bool { return s.active }

// HandleEvent implements OperationalEventSubscriber; applies dependency propagation rules.
func (s *DependencyPropagationSubscriber) HandleEvent(event *coordination.OperationalEvent) error {
	if event == nil || event.Metadata == nil {
		return nil
	}
	projectRoot, _ := event.Metadata["project_root"].(string)
	targetID, _ := event.Metadata[objects.FieldKeyTargetID].(string)
	triggerID, _ := event.Metadata["trigger_id"].(string)
	triggerKind, _ := event.Metadata["trigger_kind"].(string)
	fromState, _ := event.Metadata["from_state"].(string)
	toState, _ := event.Metadata["to_state"].(string)
	if projectRoot == emptyValue || targetID == emptyValue {
		return nil
	}
	provider, ok := s.getStorage(projectRoot)
	if !ok {
		return nil
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
		return nil
	}
	ev := DependencyRefEvent{
		ProjectRoot: projectRoot,
		TargetID:    targetID,
		TriggerID:   triggerID,
		TriggerKind: triggerKind,
		FromState:   fromState,
		ToState:     toState,
	}
	ctx := pkgctx.NewSystemContext()
	logger := logging.NewEventLogger(ctx)
	applyDependencyRefEvent(ctx, logger, provider, ev)
	return nil
}

// applyDependencyRefEvent loads the target and applies one-level propagation rules.
// Rule: if trigger (child) transitioned to a non-terminal status, move certain parent kinds to a
// lifecycle-valid "re-align" status. v1: priority_plan → grooming (see priority_plan_lifecycle.yaml
// dependency-propagation transitions). Skips non-configured kinds and terminal parents.
func applyDependencyRefEvent(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, ev DependencyRefEvent) {
	secCtx := pkgctx.NewSystemSecurityContext()
	target, err := provider.Read(ctx, secCtx, ev.TargetID)
	if err != nil || target == nil {
		return
	}
	kind, _ := target[objects.FieldKeyKind].(string)
	currentStatus, _ := target[objects.FieldKeyStatus].(string)

	if kind == emptyValue || isTerminalStatus(currentStatus) {
		return
	}
	if !isTerminalStatus(ev.ToState) {
		want, ok := dependencyPropagationTargetStatus(kind)
		if !ok || want == emptyValue {
			return
		}
		if currentStatus == want {
			return
		}
		updates := map[string]any{objects.FieldKeyStatus: want}
		if err := provider.Update(ctx, secCtx, ev.TargetID, updates); err != nil {
			logging.FluentEvent(logger).Debug("dependency propagation: update target failed").
				String("target_id", ev.TargetID).
				String("trigger_id", ev.TriggerID).
				WithError(err).
				Log()
			return
		}
		logging.FluentEvent(logger).Debug("dependency propagation: set target status for linked non-terminal trigger").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			String("trigger_to_state", ev.ToState).
			String("target_kind", kind).
			String("target_to_status", want).
			Log()
	}
}

// dependencyPropagationTargetStatus returns the status to apply for v1 propagation, or ok=false if none.
func dependencyPropagationTargetStatus(kind string) (string, bool) {
	switch kind {
	case objects.KindPriorityPlan:
		return statusGrooming, true
	default:
		return "", false
	}
}

func isTerminalStatus(s string) bool {
	switch s {
	case statusComplete, objects.ObjectStatusArchived, statusCancelled, objects.ObjectStatusError:
		return true
	default:
		return false
	}
}

// EmitDependencyRefEvents emits one-level dependency ref events via the coordinator after a status save.
// Collects (1) refs the object points to (one level up) and (2) dependents that point to it (one level down).
// Does not walk the full chain; each subsequent save triggers the next level via the lifecycle hook.
func EmitDependencyRefEvents(projectRoot, kind, id, fromState, toState string, objectData map[string]any) {
	if projectRoot == emptyValue || id == emptyValue {
		return
	}
	coord := coordination.GetCoordinator()
	if coord == nil {
		return
	}

	// One level up: refs this object points to (parents)
	refs := storage.GetReferencedObjectIDs(objectData)
	// One level down: objects that reference this ID (dependents)
	index := storage.GetGlobalReverseReferenceIndex()
	dependents := index.GetDependents(id)

	seen := make(map[string]bool)
	ctx := pkgctx.NewSystemContext()
	for _, targetID := range refs {
		targetID = strings.TrimSpace(targetID)
		if targetID == emptyValue || seen[targetID] {
			continue
		}
		seen[targetID] = true
		emitDependencyRefEvent(coord, ctx, projectRoot, targetID, id, kind, fromState, toState)
	}
	for _, targetID := range dependents {
		if targetID == emptyValue || seen[targetID] {
			continue
		}
		seen[targetID] = true
		emitDependencyRefEvent(coord, ctx, projectRoot, targetID, id, kind, fromState, toState)
	}
}

func emitDependencyRefEvent(coord coordination.EventCoordinator, ctx context.Context, projectRoot, targetID, triggerID, triggerKind, fromState, toState string) {
	eventCtx := coordination.NewEventContext(targetID, "lifecycle_dependency_ref", "dependency_ref").
		WithContext(ctx).
		WithChannels(false, false, false, true).
		WithEventData(&coordination.EventData{
			MetricsData: map[string]any{
				"project_root":           projectRoot,
				objects.FieldKeyTargetID: targetID,
				"trigger_id":             triggerID,
				"trigger_kind":           triggerKind,
				"from_state":             fromState,
				"to_state":               toState,
			},
		})
	_ = coord.Emit(ctx, eventCtx) //nolint:errcheck // best-effort
}
