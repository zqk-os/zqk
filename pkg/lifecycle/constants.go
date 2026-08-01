package lifecycle

import "github.com/lanceman/zqk/pkg/objects"

const (
	criterionAllBacklogComplete                     = "all_backlog_items_complete_for_plan"
	criterionAllBacklogCompleteForMilestone         = "all_backlog_items_complete_for_milestone"
	criterionAllCriteriaCompleteForMilestone        = "all_criteria_complete_for_milestone"
	criterionAllAcceptanceCriteriaMetForBacklogItem = "all_acceptance_criteria_met_for_backlog_item"

	kindPriorityPlan = objects.KindPriorityPlan
	kindMilestone    = objects.KindMilestone
	kindBacklogItem  = objects.KindBacklogItem
	kindCriteria     = objects.KindCriteria

	scopePlanID        = "plan_id"
	scopeMilestoneID   = "milestone_id"
	scopeBacklogItemID = "backlog_item_id"

	statusComplete   = "complete"
	statusArchived   = "archived"
	statusRejected   = "rejected"
	statusDeferred   = "deferred"
	statusInProgress = "in_progress"
	statusCancelled  = "cancelled"
	statusGrooming   = "grooming"

	prefixCrit = "CRIT-"

	emptyValue = ""
)
