// Package object lifecycle_updater: when a priority plan transitions to complete,
// activate the next plan (by order) or trigger the planning prioritization workflow (REQ-014).
package object

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Priority plan lifecycle: terminal statuses excluded from "next plan" candidates.
var priorityPlanTerminalStatuses = []any{objectStatusComplete, objectStatusArchived, objectStatusCancelled}

// Event type emitted when no next plan exists so workflows can establish the next block of work.
const EventTypePlanningPrioritizationRequired = "planning_prioritization_required"

func resolveCachedPriorityPlanStorage(projectRoot string) (storage.ObjectStorageProvider, *pkgctx.SecurityContext, *pkgctx.StorageContext, bool) {
	if projectRoot == emptyValue {
		return nil, nil, nil, false
	}
	raw, ok := cli.GetObjectStorageForProjectRoot(projectRoot)
	if !ok {
		return nil, nil, nil, false
	}
	provider, ok := nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](raw)
	if !ok {
		return nil, nil, nil, false
	}
	return provider, pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), true
}

// RunPriorityPlanCompleteUpdater runs asynchronously when a priority plan transitions to complete.
// It (1) finds the next plan by active_order/plan_date, (2) sets that plan to active_order=1 and
// status=in_progress, or (3) if no next plan exists, emits a coordinator event so the planning
// prioritization workflow can establish the next block of work.
// Uses storage from process cache (GetObjectStorageForProjectRoot); no-op if storage not cached.
func RunPriorityPlanCompleteUpdater(ctx context.Context, projectRoot string) {
	provider, secCtx, storageCtx, ok := resolveCachedPriorityPlanStorage(projectRoot)
	if !ok {
		return
	}
	eventLogger := logging.NewEventLogger(ctx)

	// List non-terminal priority plans (planning, grooming, prioritizing, active, in_progress, paused, blocked)
	filter := storage.ListFilter{
		Kind: objectKindPriorityPlan,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{"$nin": priorityPlanTerminalStatuses},
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		logging.FluentEvent(eventLogger).Debug("Lifecycle updater: list priority plans failed").
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
		return
	}
	if len(result.Objects) == 0 {
		// No next plan: trigger planning prioritization workflow (event for agents/scheduler)
		emitPlanningPrioritizationRequired(ctx, eventLogger)
		return
	}

	// Sort by active_order (lower first, nil last), then plan_date desc
	plans := result.Objects
	sort.Slice(plans, func(i, j int) bool {
		aoI, _ := toInt(plans[i][objects.FieldKeyActiveOrder])
		aoJ, _ := toInt(plans[j][objects.FieldKeyActiveOrder])
		hasI := plans[i][objects.FieldKeyActiveOrder] != nil
		hasJ := plans[j][objects.FieldKeyActiveOrder] != nil
		if hasI && hasJ {
			if aoI != aoJ {
				return aoI < aoJ
			}
		} else if hasI {
			return true
		} else if hasJ {
			return false
		}
		dateI, _ := plans[i][objects.FieldKeyPlanDate].(string)
		dateJ, _ := plans[j][objects.FieldKeyPlanDate].(string)
		return dateI > dateJ
	})

	next := plans[0]
	nextID, _ := next[objects.FieldKeyID].(string)
	if nextID == emptyValue {
		return
	}

	// Promote next plan: active_order=1, status=in_progress (REQ-014)
	updates := map[string]any{
		objects.FieldKeyActiveOrder: 1,
		objects.FieldKeyStatus:      objectStatusInProgress,
	}
	if err := provider.Update(ctx, secCtx, nextID, updates); err != nil {
		logging.FluentEvent(eventLogger).Debug("Lifecycle updater: update next plan failed").
			String("plan_id", nextID).
			ProjectRoot(projectRoot).
			WithError(err).
			Log()
		return
	}
	logging.FluentEvent(eventLogger).Info("Lifecycle updater: promoted next priority plan").
		String("plan_id", nextID).
		ProjectRoot(projectRoot).
		Log()
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}

func emitPlanningPrioritizationRequired(ctx context.Context, eventLogger *logging.EventLogger) {
	coord := coordination.GetCoordinator()
	if coord == nil {
		logging.FluentEvent(eventLogger).Debug("Lifecycle updater: no coordinator; cannot emit planning_prioritization_required").Log()
		return
	}
	eventCtx := coordination.NewEventContext(
		fmt.Sprintf("plan-pri-%d", time.Now().UnixNano()),
		EventTypePlanningPrioritizationRequired,
		"complete",
	).WithChannels(false, false, false, true).WithEventData(&coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: "reason", Value: "no_next_priority_plan"},
			{Key: "message", Value: "All priority plans complete; trigger planning prioritization to establish next block of work"},
		},
	})
	if err := coord.Emit(ctx, eventCtx); err != nil {
		logging.FluentEvent(eventLogger).Debug("Lifecycle updater: emit planning_prioritization_required failed").
			WithError(err).
			Log()
		return
	}
	logging.FluentEvent(eventLogger).Info("Lifecycle updater: emitted planning_prioritization_required (no next plan)").Log()
}

func RunPriorityPlanActivationUpdater(ctx context.Context, projectRoot string, planID string, toState string) {
	provider, secCtx, storageCtx, ok := resolveCachedPriorityPlanStorage(projectRoot)
	if !ok {
		return
	}

	filter := storage.ListFilter{
		Kind: objectKindPriorityPlan,
		Filters: map[string]any{
			objects.FieldKeyStatus: "active",
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return
	}
	plans := result.Objects

	if toState == "active" {
		max := 0
		for _, p := range plans {
			if ao, ok := toInt(p[objects.FieldKeyActiveOrder]); ok {
				if ao > max {
					max = ao
				}
			}
		}

		_ = provider.Update(ctx, secCtx, planID, map[string]any{
			objects.FieldKeyActiveOrder: max + 1,
		})
	} else if toState == "in_progress" || toState == "ready" {
		_ = provider.Update(ctx, secCtx, planID, map[string]any{
			objects.FieldKeyActiveOrder: nil,
		})

		sort.Slice(plans, func(i, j int) bool {
			aoI, _ := toInt(plans[i][objects.FieldKeyActiveOrder])
			aoJ, _ := toInt(plans[j][objects.FieldKeyActiveOrder])
			return aoI < aoJ
		})

		currentOrder := 1
		for _, p := range plans {
			id, _ := p[objects.FieldKeyID].(string)
			if id == planID {
				continue
			}
			ao, ok := toInt(p[objects.FieldKeyActiveOrder])
			if !ok || ao != currentOrder {
				_ = provider.Update(ctx, secCtx, id, map[string]any{
					objects.FieldKeyActiveOrder: currentOrder,
				})
			}
			currentOrder++
		}
	}
}
