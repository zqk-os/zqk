// Package lifecycle: one-hop status-event listeners.
// A catalyst status save publishes one event to each outbound ref (listener stubs).
// status_reactive kinds interpret the event from their own lifecycle (and any local
// ledger). The listener updates only itself; a self-update is a new catalyst.
// TRACK: REDACTED
package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
)

var dependencySubscriberOnce sync.Once

const (
	dependencyEventProjectRootKey = "project_root"
	dependencyEventTriggerIDKey   = "trigger_id"
	dependencyEventTriggerKindKey = "trigger_kind"
	dependencyEventFromStateKey   = "from_state"
	dependencyEventToStateKey     = "to_state"
	dependencyEventVersionKey     = "event_version"
	dependencyEventFieldKey       = "field"
)

type dependencyEventStamp struct {
	eventID string
	version time.Time
}

var dependencyEventStamps = struct {
	sync.Mutex
	latest map[string]dependencyEventStamp
}{latest: make(map[string]dependencyEventStamp)}

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
	EventID     string
	Version     string
	TargetID    string
	TriggerID   string
	TriggerKind string
	FromState   string
	ToState     string
	Field       string
	EdgeRole    objects.EdgeRole
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
	projectRoot, _ := event.Metadata[dependencyEventProjectRootKey].(string)
	targetID, _ := event.Metadata[objects.FieldKeyTargetID].(string)
	triggerID, _ := event.Metadata[dependencyEventTriggerIDKey].(string)
	triggerKind, _ := event.Metadata[dependencyEventTriggerKindKey].(string)
	fromState, _ := event.Metadata[dependencyEventFromStateKey].(string)
	toState, _ := event.Metadata[dependencyEventToStateKey].(string)
	version, _ := event.Metadata[dependencyEventVersionKey].(string)
	if projectRoot == emptyValue || targetID == emptyValue {
		return nil
	}
	if version == emptyValue {
		timestamp := event.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now()
		}
		version = zqktime.FormatRFC3339NanoUTC(timestamp)
	}
	eventID := event.OperationID
	if eventID == emptyValue {
		eventID = dependencyEventID(targetID, triggerID, fromState, toState, version)
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
		EventID:     eventID,
		Version:     version,
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
//
// priority_plan shockwave (priority_plan_lifecycle.yaml):
//   - linked backlog_item → in_progress: plan active|grooming|… → in_progress
//     (locks scope; clears active_order so other shovel-ready plans can take numeric order 1+)
//   - linked backlog_item → in_progress: shovel-ready criteria awaiting_verification → in_progress
//     (class lock; destination is never validated)
//   - linked backlog_item reopened to exploring/validated: plan active|in_progress|paused|blocked → grooming
//   - last linked backlog_item → terminal: plan in_progress|active → complete (trusted open-children
//     ledger; no List on this hot path — TRACK: BLI-CEF-ARCH-EVENTS-GLOBALS)
//
// planning/prioritizing collapsed into grooming (REDACTED).
//
// Semantics: plan status in_progress ≡ execution-locked "top of stack". Numeric active_order
// is unset (FieldUnset) so other shovel-ready plans can occupy 1+. Ranking (whats-next) must
// treat in_progress as order-0, not as "unset = lowest".
//
// Do NOT send the plan to grooming merely because a child is planned — that inverted the
// shovel-ready contract (observed: Community PRI forced to grooming while a BLI was in_progress).
// DO skip →in_progress when any linked child is still exploring/validated (ready-or-later
// precondition); otherwise membership warnings fire on every system check.
func applyDependencyRefEvent(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, ev DependencyRefEvent) {
	secCtx := pkgctx.NewSystemSecurityContext()
	target, err := provider.Read(ctx, secCtx, ev.TargetID)
	if err != nil || target == nil {
		logging.FluentEvent(logger).Warn("dependency propagation: read target failed").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			WithError(err).
			Log()
		return
	}
	kind, _ := target[objects.FieldKeyKind].(string)
	currentStatus, _ := target[objects.FieldKeyStatus].(string)

	if kind == emptyValue || isTerminalStatus(currentStatus) {
		return
	}
	if !kindIsStatusReactive(kind) {
		return
	}
	if kindIsOpenCountable(kind) {
		if isTerminalStatus(ev.ToState) && !isTerminalStatus(ev.FromState) {
			if applyOpenCountableLastChild(ctx, logger, provider, secCtx, ev, kind, currentStatus) {
				return
			}
		}
		if isTerminalStatus(ev.FromState) && !isTerminalStatus(ev.ToState) {
			noteOpenCountableReentered(ctx, provider, ev.TargetID)
		}
	}
	if kind == objects.KindPriorityPlan {
		applyPriorityPlanDependencyRef(ctx, logger, provider, secCtx, ev, currentStatus)
		return
	}
	updates, ok := statusReactiveUpdates(kind, currentStatus, ev.TriggerKind, ev.FromState, ev.ToState)
	if !ok || len(updates) == 0 {
		return
	}
	commitStatusReactive(ctx, logger, provider, secCtx, ev, kind, currentStatus, updates)
}

func kindIsStatusReactive(kind string) bool {
	has, err := objects.KindHasTrait(kind, objects.TraitStatusReactive)
	return err == nil && has
}

func kindIsOpenCountable(kind string) bool {
	has, err := objects.KindHasTrait(kind, objects.TraitOpenCountable)
	return err == nil && has
}

func applyPriorityPlanDependencyRef(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, ev DependencyRefEvent, currentStatus string) {
	triggerKind := ev.TriggerKind
	if triggerKind == emptyValue {
		triggerKind = objects.KindBacklogItem
	}

	updates, ok := priorityPlanShockwaveUpdates(currentStatus, triggerKind, ev.FromState, ev.ToState)
	if !ok || len(updates) == 0 {
		// Child hop was not the lock catalyst (e.g. validated→planned). Re-evaluate occupancy:
		// if work is already in flight and membership is now ready-or-later, lock.
		// TRACK: REDACTED
		applyOccupancyExecutionLock(ctx, logger, provider, secCtx, ev, currentStatus)
		return
	}
	// Airtight ready-or-later (priority_plan_lifecycle.yaml precondition + REDACTED):
	// YAML on_dependent_status alone must not lock execution while exploring/validated siblings remain —
	// that is exactly execution_facing_membership. Shockwave used to ignore the precondition.
	// TRACK: REDACTED — PRI active→in_progress shockwave miss
	if newStatus, _ := updates[objects.FieldKeyStatus].(string); newStatus == statusInProgress {
		ready, checkErr := planLinkedBacklogReadyOrLater(ctx, provider, ev.TargetID)
		if checkErr != nil {
			logging.FluentEvent(logger).Debug("dependency propagation: ready-or-later check failed; skip lock").
				String("target_id", ev.TargetID).
				String("trigger_id", ev.TriggerID).
				WithError(checkErr).
				Log()
			return
		}
		if !ready {
			logging.FluentEvent(logger).Info("dependency propagation: skip in_progress lock — linked children not ready-or-later").
				String("target_id", ev.TargetID).
				String("trigger_id", ev.TriggerID).
				String("plan_status", currentStatus).
				Log()
			return
		}
	}
	commitPriorityPlanShockwave(ctx, logger, provider, secCtx, ev, currentStatus, updates)
}

func statusReactiveUpdates(kind, currentStatus, triggerKind, fromState, toState string) (map[string]any, bool) {
	_ = fromState
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return nil, false
	}
	lc, err := loader.LoadLifecycle(kind)
	if err != nil || lc == nil {
		return nil, false
	}
	return shockwaveUpdatesFromTransitions(lc.Transitions, currentStatus, triggerKind, toState)
}

func commitStatusReactive(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, ev DependencyRefEvent, kind, currentStatus string, updates map[string]any) {
	if !reserveDependencyEvent(ev) {
		return
	}
	trustedCtx := pkgctx.WithTrustedLifecycleEvent(ctx, pkgctx.TrustedLifecycleEvent{
		EventID:   ev.EventID,
		Version:   ev.Version,
		TargetID:  ev.TargetID,
		TriggerID: ev.TriggerID,
		FromState: ev.FromState,
		ToState:   ev.ToState,
	})
	if err := provider.Update(trustedCtx, secCtx, ev.TargetID, updates); err != nil {
		releaseDependencyEvent(ev)
		logging.FluentEvent(logger).Debug("dependency propagation: status_reactive update failed").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			String("kind", kind).
			WithError(err).
			Log()
		return
	}
	if ev.ProjectRoot != emptyValue {
		if flushErr := storage.FlushListingIndexForProjectRoot(ev.ProjectRoot, kind); flushErr != nil {
			logging.FluentEvent(logger).Debug("dependency propagation: flush status_reactive CAS index failed").
				String("target_id", ev.TargetID).
				String("kind", kind).
				WithError(flushErr).
				Log()
		}
	}
	logging.FluentEvent(logger).Debug("dependency propagation: status_reactive applied").
		String("event_id", ev.EventID).
		String("target_id", ev.TargetID).
		String("trigger_id", ev.TriggerID).
		String("kind", kind).
		String("from", currentStatus).
		String("to", fmt.Sprint(updates[objects.FieldKeyStatus])).
		Log()
}

// MaybeExecutionLockPlan locks an active/grooming/halted plan when all linked
// backlog_items are ready-or-later and at least one is already in_progress.
// Used after promoting a plan to shovel-ready when children started during grooming.
// TRACK: REDACTED
func MaybeExecutionLockPlan(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, projectRoot, planID string) bool {
	if provider == nil || strings.TrimSpace(planID) == emptyValue {
		return false
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	target, err := provider.Read(ctx, secCtx, planID)
	if err != nil || target == nil {
		return false
	}
	currentStatus, _ := target[objects.FieldKeyStatus].(string)
	if currentStatus == statusInProgress {
		return false
	}
	version := zqktime.NowRFC3339NanoUTC()
	ev := DependencyRefEvent{
		ProjectRoot: projectRoot,
		EventID:     occupancyLockEventID(planID, version),
		Version:     version,
		TargetID:    planID,
		TriggerID:   planID,
		TriggerKind: objects.KindPriorityPlan,
		FromState:   currentStatus,
		ToState:     statusInProgress,
	}
	applyOccupancyExecutionLock(ctx, logger, provider, secCtx, ev, currentStatus)
	after, err := provider.Read(ctx, secCtx, planID)
	if err != nil || after == nil {
		return false
	}
	st, _ := after[objects.FieldKeyStatus].(string)
	return st == statusInProgress && currentStatus != statusInProgress
}

func occupancyLockEventID(planID, version string) string {
	return "occupancy-lock:" + planID + ":" + version
}

func planStatusCanOccupancyLock(status string) bool {
	switch status {
	case statusActive, statusGrooming, statusPaused, statusBlocked:
		return true
	default:
		return false
	}
}

func applyOccupancyExecutionLock(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, ev DependencyRefEvent, currentStatus string) {
	if !planStatusCanOccupancyLock(currentStatus) {
		return
	}
	allReady, anyInProgress, err := planLinkedBacklogLockScan(ctx, provider, ev.TargetID)
	if err != nil {
		logging.FluentEvent(logger).Debug("dependency propagation: occupancy lock scan failed").
			String("target_id", ev.TargetID).
			WithError(err).
			Log()
		return
	}
	if !allReady || !anyInProgress {
		return
	}
	updates, ok := priorityPlanShockwaveUpdates(currentStatus, objects.KindBacklogItem, ev.FromState, statusInProgress)
	if !ok || len(updates) == 0 {
		return
	}
	occ := ev
	occ.ToState = statusInProgress
	if occ.Version == emptyValue {
		occ.Version = zqktime.NowRFC3339NanoUTC()
	}
	if occ.EventID == emptyValue {
		occ.EventID = occupancyLockEventID(ev.TargetID, occ.Version)
	}
	commitPriorityPlanShockwave(ctx, logger, provider, secCtx, occ, currentStatus, updates)
}

func commitPriorityPlanShockwave(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, ev DependencyRefEvent, currentStatus string, updates map[string]any) {
	if !reserveDependencyEvent(ev) {
		return
	}
	trustedCtx := pkgctx.WithTrustedLifecycleEvent(ctx, pkgctx.TrustedLifecycleEvent{
		EventID:   ev.EventID,
		Version:   ev.Version,
		TargetID:  ev.TargetID,
		TriggerID: ev.TriggerID,
		FromState: ev.FromState,
		ToState:   ev.ToState,
	})
	if err := provider.Update(trustedCtx, secCtx, ev.TargetID, updates); err != nil {
		releaseDependencyEvent(ev)
		logging.FluentEvent(logger).Debug("dependency propagation: update target failed").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			WithError(err).
			Log()
		return
	}
	// Cold path: seed remaining_open_count once when execution-locking (member List, no sidecar).
	if newStatus, _ := updates[objects.FieldKeyStatus].(string); newStatus == statusInProgress {
		if seedErr := SeedRemainingOpenCountFromMembers(ctx, provider, ev.TargetID, false); seedErr != nil {
			logging.FluentEvent(logger).Debug("dependency propagation: seed remaining_open_count failed").
				String("target_id", ev.TargetID).
				WithError(seedErr).
				Log()
		}
	}
	// Shockwave is in-process side effect of BLI promote; flush plan CAS index so the next
	// CLI process can Read the plan. TRACK: REDACTED
	if ev.ProjectRoot != emptyValue {
		if flushErr := storage.FlushListingIndexForProjectRoot(ev.ProjectRoot, objects.KindPriorityPlan); flushErr != nil {
			logging.FluentEvent(logger).Debug("dependency propagation: flush priority_plan CAS index failed").
				String("target_id", ev.TargetID).
				WithError(flushErr).
				Log()
		}
	}
	logging.FluentEvent(logger).Debug("dependency propagation: priority_plan shockwave applied").
		String("event_id", ev.EventID).
		String("event_version", ev.Version).
		String("target_id", ev.TargetID).
		String("trigger_id", ev.TriggerID).
		String("trigger_from", ev.FromState).
		String("trigger_to", ev.ToState).
		String("plan_from", currentStatus).
		String("plan_to", fmt.Sprint(updates[objects.FieldKeyStatus])).
		Log()
}

// applyOpenCountableLastChild CAS-decrements remaining_open_count and, when zero,
// applies the YAML auto complete hop (side_effects included). Returns true when the
// terminal-child path handled the event (including non-last decrements).
func applyOpenCountableLastChild(
	ctx context.Context,
	logger *logging.EventLogger,
	provider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	ev DependencyRefEvent,
	kind, currentStatus string,
) bool {
	if ev.ProjectRoot == emptyValue || ev.TriggerID == emptyValue {
		return false
	}
	remaining, handled, err := consumeOpenChild(ctx, provider, secCtx, ev)
	if err != nil {
		logging.FluentEvent(logger).Debug("dependency propagation: remaining_open_count decrement failed").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			WithError(err).
			Log()
		return false
	}
	if !handled {
		return false
	}
	if remaining > 0 {
		logging.FluentEvent(logger).Debug("dependency propagation: remaining_open_count decremented").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			Int("remaining", remaining).
			Log()
		return true
	}

	updates, ok := autoCompleteUpdates(kind, currentStatus)
	if !ok || len(updates) == 0 {
		return true
	}
	updates[objects.FieldKeyRemainingOpenCount] = 0

	if !reserveDependencyEvent(ev) {
		return true
	}
	EmitTrustedRemainingOpenDrained(ev.ProjectRoot, ev.TargetID)
	trustedCtx := pkgctx.WithTrustedLifecycleEvent(ctx, pkgctx.TrustedLifecycleEvent{
		EventID:   ev.EventID,
		Version:   ev.Version,
		TargetID:  ev.TargetID,
		TriggerID: ev.TriggerID,
		FromState: ev.FromState,
		ToState:   ev.ToState,
	})
	trustedCtx = pkgctx.WithLifecycleBreakGlass(trustedCtx, "lifecycle shockwave last-child complete")
	if err := provider.Update(trustedCtx, secCtx, ev.TargetID, updates); err != nil {
		releaseDependencyEvent(ev)
		logging.FluentEvent(logger).Debug("dependency propagation: last-child complete update failed").
			String("target_id", ev.TargetID).
			String("trigger_id", ev.TriggerID).
			String("kind", kind).
			String("from", currentStatus).
			WithError(err).
			Log()
		return true
	}
	if flushErr := storage.FlushListingIndexForProjectRoot(ev.ProjectRoot, kind); flushErr != nil {
		logging.FluentEvent(logger).Debug("dependency propagation: flush last-child CAS index failed").
			String("target_id", ev.TargetID).
			String("kind", kind).
			WithError(flushErr).
			Log()
	}
	logging.FluentEvent(logger).Debug("dependency propagation: open_countable last-child complete applied").
		String("event_id", ev.EventID).
		String("target_id", ev.TargetID).
		String("trigger_id", ev.TriggerID).
		String("kind", kind).
		String("from", currentStatus).
		String("to", fmt.Sprint(updates[objects.FieldKeyStatus])).
		Log()
	return true
}

// autoCompleteUpdates finds an auto lifecycle hop from currentStatus to a terminal
// status that is not an on_dependent_status listener (those run via status_reactive).
func autoCompleteUpdates(kind, currentStatus string) (map[string]any, bool) {
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return nil, false
	}
	lc, err := loader.LoadLifecycle(kind)
	if err != nil || lc == nil {
		return nil, false
	}
	currentStatus = strings.TrimSpace(currentStatus)
	for i := range lc.Transitions {
		tr := &lc.Transitions[i]
		if !tr.Auto || tr.OnDependentStatus != nil {
			continue
		}
		if tr.From != currentStatus && tr.From != "*" {
			continue
		}
		if !isTerminalStatus(tr.To) {
			continue
		}
		updates := map[string]any{objects.FieldKeyStatus: tr.To}
		for _, se := range tr.SideEffects {
			field := strings.TrimSpace(se.Clear)
			if field == emptyValue {
				continue
			}
			updates[field] = storage.FieldUnset
		}
		return updates, true
	}
	return nil, false
}

// ApplyPlanChildMembershipRemoved handles override pplan remove / clearing priority_plan_ref.
// Membership left without a status transition to terminal — still must shrink the open set and
// may complete the plan when the removed child was the last open item.
func ApplyPlanChildMembershipRemoved(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, projectRoot, planID, childID string) {
	if provider == nil || projectRoot == emptyValue || planID == emptyValue || childID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	plan, err := provider.Read(ctx, secCtx, planID)
	if err != nil || plan == nil {
		return
	}
	kind, _ := plan[objects.FieldKeyKind].(string)
	status, _ := plan[objects.FieldKeyStatus].(string)
	if !kindIsOpenCountable(kind) || isTerminalStatus(status) {
		return
	}
	if child, childErr := provider.Read(ctx, secCtx, childID); childErr == nil && child != nil {
		if st, _ := child[objects.FieldKeyStatus].(string); isBacklogItemTerminalStatus(st) {
			// Status hop already consumed this child. Do not decrement twice.
			return
		}
	}
	if status != statusInProgress && status != statusActive {
		ev := DependencyRefEvent{ProjectRoot: projectRoot, TargetID: planID, TriggerID: childID}
		_, _, _ = consumeOpenChild(ctx, provider, secCtx, ev)
		return
	}
	version := zqktime.NowRFC3339NanoUTC()
	ev := DependencyRefEvent{
		ProjectRoot: projectRoot,
		EventID:     dependencyEventID(planID, childID, "membership", statusComplete, version),
		Version:     version,
		TargetID:    planID,
		TriggerID:   childID,
		TriggerKind: objects.KindBacklogItem,
		FromState:   "membership",
		ToState:     statusComplete, // treat remove as leaving the open set
	}
	_ = applyOpenCountableLastChild(ctx, logger, provider, secCtx, ev, kind, status)
}

func reserveDependencyEvent(ev DependencyRefEvent) bool {
	if ev.EventID == emptyValue || ev.Version == emptyValue {
		return true
	}
	version, err := time.Parse(time.RFC3339Nano, ev.Version)
	if err != nil {
		return false
	}
	key := ev.TargetID + "\x00" + ev.TriggerID
	dependencyEventStamps.Lock()
	defer dependencyEventStamps.Unlock()
	if previous, ok := dependencyEventStamps.latest[key]; ok {
		if previous.eventID == ev.EventID || !version.After(previous.version) {
			return false
		}
	}
	dependencyEventStamps.latest[key] = dependencyEventStamp{eventID: ev.EventID, version: version}
	return true
}

func releaseDependencyEvent(ev DependencyRefEvent) {
	if ev.EventID == emptyValue || ev.Version == emptyValue {
		return
	}
	key := ev.TargetID + "\x00" + ev.TriggerID
	dependencyEventStamps.Lock()
	defer dependencyEventStamps.Unlock()
	if current, ok := dependencyEventStamps.latest[key]; ok && current.eventID == ev.EventID {
		delete(dependencyEventStamps.latest, key)
	}
}

// planLinkedBacklogReadyOrLater mirrors validation.PrecondAllLinkedBacklogReadyOrLater using
// a storage List (same cold path as SeedRemainingOpenCountFromMembers). Vacuous true when no
// linked backlog_items exist. Fail-closed (false, err) on List failure.
// TRACK: REDACTED
func planLinkedBacklogReadyOrLater(ctx context.Context, provider storage.ObjectStorageProvider, planID string) (bool, error) {
	allReady, _, err := planLinkedBacklogLockScan(ctx, provider, planID)
	return allReady, err
}

func planLinkedBacklogLockScan(ctx context.Context, provider storage.ObjectStorageProvider, planID string) (allReady bool, anyInProgress bool, err error) {
	planID = strings.TrimSpace(planID)
	if provider == nil || planID == emptyValue {
		return false, false, nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	result, err := provider.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: planID},
	})
	if err != nil {
		return false, false, err
	}
	allReady = true
	for _, obj := range result.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		if !backlogItemStatusReadyOrLaterForLock(st) {
			allReady = false
		}
		if strings.EqualFold(strings.TrimSpace(st), statusInProgress) {
			anyInProgress = true
		}
	}
	return allReady, anyInProgress, nil
}

func backlogItemStatusReadyOrLaterForLock(status string) bool {
	role := objects.GetGlobalStatusChecker().Role(objects.KindBacklogItem, status)
	if role != emptyValue {
		return objects.RoleReadyOrLaterForLock(role)
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case objects.ObjectStatusPlanned, objects.ObjectStatusInProgress,
		objects.ObjectStatusComplete, objects.ObjectStatusArchived:
		return true
	default:
		return false
	}
}

// priorityPlanShockwaveUpdates returns status (and optional side-effect) updates for a
// priority_plan after a linked child's status change. Rules come from lifecycle YAML
// transitions with on_dependent_status (TRACK: REDACTED).
func priorityPlanShockwaveUpdates(planStatus, triggerKind, fromState, toState string) (map[string]any, bool) {
	return statusReactiveUpdates(objects.KindPriorityPlan, planStatus, triggerKind, fromState, toState)
}

func priorityPlanShockwaveTransitions() []objects.Transition {
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return nil
	}
	lc, err := loader.LoadLifecycle(objects.KindPriorityPlan)
	if err != nil || lc == nil {
		return nil
	}
	return lc.Transitions
}

// shockwaveUpdatesFromTransitions matches auto transitions annotated with
// on_dependent_status and applies side_effects. Go stays a thin executor over YAML.
func shockwaveUpdatesFromTransitions(transitions []objects.Transition, planStatus, triggerKind, toState string) (map[string]any, bool) {
	if triggerKind == emptyValue {
		triggerKind = objects.KindBacklogItem
	}
	planStatus = strings.TrimSpace(planStatus)
	toState = strings.TrimSpace(toState)
	for i := range transitions {
		tr := &transitions[i]
		if !tr.Auto || tr.OnDependentStatus == nil {
			continue
		}
		if tr.From != planStatus && tr.From != "*" {
			continue
		}
		dep := tr.OnDependentStatus
		if dep.Kind != emptyValue && dep.Kind != triggerKind {
			continue
		}
		if !dependentStatusMatches(dep.To, toState) {
			continue
		}
		updates := map[string]any{objects.FieldKeyStatus: tr.To}
		for _, se := range tr.SideEffects {
			field := strings.TrimSpace(se.Clear)
			if field == emptyValue {
				continue
			}
			updates[field] = storage.FieldUnset
		}
		return updates, true
	}
	return nil, false
}

func dependentStatusMatches(candidates objects.StringOrSlice, toState string) bool {
	for _, c := range candidates {
		if strings.EqualFold(strings.TrimSpace(c), toState) {
			return true
		}
	}
	return false
}

func isTerminalStatus(s string) bool {
	switch s {
	case statusComplete, objects.ObjectStatusArchived, statusCancelled, objects.ObjectStatusError:
		return true
	default:
		return false
	}
}

// EmitDependencyRefEvents publishes one status event to each outbound ref (listener stubs).
func EmitDependencyRefEvents(projectRoot, kind, id, fromState, toState string, objectData map[string]any) {
	if projectRoot == emptyValue || id == emptyValue {
		return
	}
	coord := coordination.GetCoordinator()
	if coord == nil {
		return
	}

	kindFromObj, _ := objectData[objects.FieldKeyKind].(string)
	if kindFromObj == emptyValue {
		kindFromObj = kind
	}
	targetIDs := dependencyRefTargetIDs(id, objectData)
	ctx := pkgctx.NewSystemContext()
	version := zqktime.NowRFC3339NanoUTC()
	for _, targetID := range targetIDs {
		field, role := dependencyHopMeta(kindFromObj, targetID, objectData)
		emitDependencyRefEvent(coord, ctx, DependencyRefEvent{
			ProjectRoot: projectRoot,
			EventID:     dependencyEventID(targetID, id, fromState, toState, version),
			Version:     version,
			TargetID:    targetID,
			TriggerID:   id,
			TriggerKind: kindFromObj,
			FromState:   fromState,
			ToState:     toState,
			Field:       field,
			EdgeRole:    role,
		})
	}
}

// ApplyDependencyRefEvents applies one-level propagation synchronously with the storage provider
// already participating in the transition. This is the deterministic path for short-lived CLI
// mutations; EmitDependencyRefEvents remains the event-bus fallback.
func ApplyDependencyRefEvents(ctx context.Context, logger *logging.EventLogger, provider storage.ObjectStorageProvider, projectRoot, kind, id, fromState, toState string, objectData map[string]any) {
	if provider == nil || projectRoot == emptyValue || id == emptyValue {
		return
	}
	version := zqktime.NowRFC3339NanoUTC()
	kindFromObj, _ := objectData[objects.FieldKeyKind].(string)
	if kindFromObj == emptyValue {
		kindFromObj = kind
	}
	targetIDs := dependencyRefTargetIDs(id, objectData)
	for _, targetID := range targetIDs {
		field, role := dependencyHopMeta(kindFromObj, targetID, objectData)
		applyDependencyRefEvent(ctx, logger, provider, DependencyRefEvent{
			ProjectRoot: projectRoot,
			EventID:     dependencyEventID(targetID, id, fromState, toState, version),
			Version:     version,
			TargetID:    targetID,
			TriggerID:   id,
			TriggerKind: kindFromObj,
			FromState:   fromState,
			ToState:     toState,
			Field:       field,
			EdgeRole:    role,
		})
	}
}

func dependencyHopMeta(kind, targetID string, objectData map[string]any) (string, objects.EdgeRole) {
	for _, hop := range storage.OutboundGraphHops(kind, objectData) {
		if hop.NeighborID == targetID {
			return hop.Field, hop.Role
		}
	}
	return emptyValue, emptyValue
}

func dependencyRefTargetIDs(id string, objectData map[string]any) []string {
	// Catalyst outbound refs are listener stubs. Reverse-index dependents are not
	// a walk of the graph in this event. TRACK: REDACTED
	refs := storage.GetReferencedObjectIDs(objectData)
	seen := make(map[string]bool)
	targetIDs := make([]string, 0, len(refs))
	for _, targetID := range refs {
		targetID = strings.TrimSpace(targetID)
		if targetID == emptyValue || targetID == id || seen[targetID] {
			continue
		}
		seen[targetID] = true
		targetIDs = append(targetIDs, targetID)
	}
	return targetIDs
}

func dependencyEventID(targetID, triggerID, fromState, toState, version string) string {
	return strings.Join([]string{triggerID, targetID, fromState, toState, version}, "|")
}

func emitDependencyRefEvent(coord coordination.EventCoordinator, ctx context.Context, ev DependencyRefEvent) {
	eventCtx := coordination.NewEventContext(ev.EventID, "lifecycle_dependency_ref", "dependency_ref").
		WithContext(ctx).
		WithChannels(false, false, false, true).
		WithEventData(&coordination.EventData{
			MetricsData: map[string]any{
				dependencyEventProjectRootKey: ev.ProjectRoot,
				objects.FieldKeyTargetID:      ev.TargetID,
				dependencyEventTriggerIDKey:   ev.TriggerID,
				dependencyEventTriggerKindKey: ev.TriggerKind,
				dependencyEventFromStateKey:   ev.FromState,
				dependencyEventToStateKey:     ev.ToState,
				dependencyEventVersionKey:     ev.Version,
				dependencyEventFieldKey:       ev.Field,
				objects.SpecKeyEdgeRole:       string(ev.EdgeRole),
			},
		})
	// Lifecycle propagation is part of the transition's consistency boundary. A short-lived
	// CLI may exit before DefaultOperationalRouter's goroutines run, leaving the child and
	// parent in contradictory lifecycle states.
	if syncCoord, ok := coord.(interface {
		EmitOperationalSync(context.Context, *coordination.EventContext) error
	}); ok {
		_ = syncCoord.EmitOperationalSync(ctx, eventCtx) //nolint:errcheck // best-effort lifecycle hook
		return
	}
	_ = coord.Emit(ctx, eventCtx) //nolint:errcheck // best-effort
}
