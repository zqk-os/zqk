package object

import "github.com/lanceman/zqk/pkg/objects"

const (
	pplanKindBacklogItem  = objects.KindBacklogItem
	pplanKindPriorityPlan = objects.KindPriorityPlan
	pplanStatusInProgress = "in_progress"
	pplanStatusActive     = "active"
	pplanSortPriorityTier = "priority_tier"
	pplanSortStatus       = "status"
	pplanFieldStatus      = "status"
	pplanFieldPlanDate    = "plan_date"
	pplanFieldActiveOrder = "active_order"
	pplanFieldPriorityRef = objects.FieldKeyPriorityPlanRef
)
