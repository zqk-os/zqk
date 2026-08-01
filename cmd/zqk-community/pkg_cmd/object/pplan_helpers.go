package object

import (
	"context"
	"fmt"
	"sort"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// backlogItemTerminalStatuses are statuses that should be excluded from priority plan views
// (matches backlog_item lifecycle: terminal: true = complete, archived, rejected)
var backlogItemTerminalStatuses = []any{pplanStatusComplete, pplanStatusArchived, pplanStatusRejected}

// statusFilterExcludingTerminal returns a filter value for backlog_item status that excludes terminal statuses
func statusFilterExcludingTerminal() map[string]any {
	return map[string]any{"$nin": backlogItemTerminalStatuses}
}

// PriorityPlanInfo represents a priority plan with ordering information
type PriorityPlanInfo struct {
	ID          string
	Status      string
	ActiveOrder *int
	PlanDate    string
}

// findCurrentPriorityPlan finds the current priority plan
// Priority: in_progress > active (lowest active_order) > active (no active_order, by plan_date desc)
func findCurrentPriorityPlan(cmdCtx context.Context, storageProvider storage.ObjectStorageProvider) (*PriorityPlanInfo, error) {
	logger := logging.GetLoggerFromContext(cmdCtx)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// First, try to find in_progress plan
	filter := storage.ListFilter{
		Kind:    pplanKindPriorityPlan,
		Filters: map[string]any{pplanFieldStatus: pplanStatusInProgress},
	}

	result, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query priority plans").Wrap(err)
	}

	if len(result.Objects) > 0 {
		// Return first in_progress plan (should only be one)
		plan := result.Objects[0]
		planID, _ := plan[objects.FieldKeyID].(string)
		status, _ := plan[pplanFieldStatus].(string)
		planDate, _ := plan[pplanFieldPlanDate].(string)
		var activeOrder *int
		switch ao := plan[pplanFieldActiveOrder].(type) {
		case int:
			activeOrder = &ao
		case float64:
			aoInt := int(ao)
			activeOrder = &aoInt
		}

		logging.FluentEvent(logger).Debug(fmt.Sprintf("Found in_progress plan: %s", planID)).Log()
		return &PriorityPlanInfo{
			ID:          planID,
			Status:      status,
			ActiveOrder: activeOrder,
			PlanDate:    planDate,
		}, nil
	}

	// If no in_progress, find active plans
	filter = storage.ListFilter{
		Kind:    pplanKindPriorityPlan,
		Filters: map[string]any{pplanFieldStatus: pplanStatusActive},
	}

	result, err = storageProvider.List(cmdCtx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to query active priority plans").Wrap(err)
	}

	if len(result.Objects) == 0 {
		return nil, errfmt.Errorf("no active or in_progress priority plans found")
	}

	// Sort active plans: lowest active_order first, then by plan_date desc
	plans := make([]PriorityPlanInfo, 0, len(result.Objects))
	for _, obj := range result.Objects {
		planID, _ := obj[objects.FieldKeyID].(string)
		status, _ := obj[pplanFieldStatus].(string)
		planDate, _ := obj[pplanFieldPlanDate].(string)
		var activeOrder *int
		switch ao := obj[pplanFieldActiveOrder].(type) {
		case int:
			activeOrder = &ao
		case float64:
			aoInt := int(ao)
			activeOrder = &aoInt
		}

		plans = append(plans, PriorityPlanInfo{
			ID:          planID,
			Status:      status,
			ActiveOrder: activeOrder,
			PlanDate:    planDate,
		})
	}

	// Sort: active_order (lower first), then plan_date (desc)
	sort.Slice(plans, func(i, j int) bool {
		// Plans with active_order come first
		if plans[i].ActiveOrder != nil && plans[j].ActiveOrder != nil {
			if *plans[i].ActiveOrder != *plans[j].ActiveOrder {
				return *plans[i].ActiveOrder < *plans[j].ActiveOrder
			}
		} else if plans[i].ActiveOrder != nil {
			return true // i has order, j doesn't - i comes first
		} else if plans[j].ActiveOrder != nil {
			return false // j has order, i doesn't - j comes first
		}

		// Both have no active_order or same active_order, sort by plan_date desc
		return plans[i].PlanDate > plans[j].PlanDate
	})

	logging.FluentEvent(logger).Debug(fmt.Sprintf("Found active plan: %s (active_order: %v)", plans[0].ID, plans[0].ActiveOrder)).Log()
	return &plans[0], nil
}

// getAllActivePriorityPlans gets all active and in_progress priority plans, sorted
func getAllActivePriorityPlans(cmdCtx context.Context, storageProvider storage.ObjectStorageProvider) ([]PriorityPlanInfo, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Get in_progress plans
	filterInProgress := storage.ListFilter{
		Kind:    pplanKindPriorityPlan,
		Filters: map[string]any{pplanFieldStatus: pplanStatusInProgress},
	}

	resultInProgress, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filterInProgress)
	if err != nil {
		return nil, errfmt.Newf("failed to query in_progress priority plans").Wrap(err)
	}

	// Get active plans
	filterActive := storage.ListFilter{
		Kind:    pplanKindPriorityPlan,
		Filters: map[string]any{pplanFieldStatus: pplanStatusActive},
	}

	resultActive, err := storageProvider.List(cmdCtx, secCtx, storageCtx, filterActive)
	if err != nil {
		return nil, errfmt.Newf("failed to query active priority plans").Wrap(err)
	}

	// Combine results
	allObjects := make([]map[string]any, 0, len(resultInProgress.Objects)+len(resultActive.Objects))
	allObjects = append(allObjects, resultInProgress.Objects...)
	allObjects = append(allObjects, resultActive.Objects...)

	plans := make([]PriorityPlanInfo, 0, len(allObjects))
	for _, obj := range allObjects {
		planID, _ := obj[objects.FieldKeyID].(string)
		status, _ := obj[pplanFieldStatus].(string)
		planDate, _ := obj[pplanFieldPlanDate].(string)
		var activeOrder *int
		switch ao := obj[pplanFieldActiveOrder].(type) {
		case int:
			activeOrder = &ao
		case float64:
			aoInt := int(ao)
			activeOrder = &aoInt
		}

		plans = append(plans, PriorityPlanInfo{
			ID:          planID,
			Status:      status,
			ActiveOrder: activeOrder,
			PlanDate:    planDate,
		})
	}

	// Sort: in_progress first, then active by active_order (lower first), then by plan_date desc
	sort.Slice(plans, func(i, j int) bool {
		// in_progress always comes before active
		if plans[i].Status == pplanStatusInProgress && plans[j].Status != pplanStatusInProgress {
			return true
		}
		if plans[i].Status != pplanStatusInProgress && plans[j].Status == pplanStatusInProgress {
			return false
		}

		// Both same status, sort by active_order
		if plans[i].ActiveOrder != nil && plans[j].ActiveOrder != nil {
			if *plans[i].ActiveOrder != *plans[j].ActiveOrder {
				return *plans[i].ActiveOrder < *plans[j].ActiveOrder
			}
		} else if plans[i].ActiveOrder != nil {
			return true
		} else if plans[j].ActiveOrder != nil {
			return false
		}

		// Both have no active_order or same active_order, sort by plan_date desc
		return plans[i].PlanDate > plans[j].PlanDate
	})

	return plans, nil
}
