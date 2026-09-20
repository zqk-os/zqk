package lifecycle

import "github.com/zqk-os/zqk/pkg/objects"

const (
	criterionAllBacklogComplete                     = "all_backlog_items_complete_for_plan"
	criterionAllBacklogCompleteForMilestone         = "all_backlog_items_complete_for_milestone"
	criterionAllCriteriaCompleteForMilestone        = "all_criteria_complete_for_milestone"
	criterionAllAcceptanceCriteriaMetForBacklogItem = "all_acceptance_criteria_met_for_backlog_item"

	kindPriorityPlan  = objects.KindPriorityPlan
	kindMilestone     = objects.KindMilestone
	kindBacklogItem   = objects.KindBacklogItem
	kindCriteria      = objects.KindCriteria
	kindRequirement   = objects.KindRequirement
	kindGoal          = objects.KindGoal
	kindVision        = objects.KindVision
	kindMission       = objects.KindMission
	kindWorkstream    = objects.KindWorkstream
	kindRoadmap       = objects.KindRoadmap
	kindStrategicPlan = objects.KindStrategicPlan

	scopePlanID        = "plan_id"
	scopeMilestoneID   = "milestone_id"
	scopeBacklogItemID = "backlog_item_id"

	statusComplete     = "complete"
	statusArchived     = "archived"
	statusDeferred     = "deferred"
	statusInProgress   = "in_progress"
	statusCancelled    = "cancelled"
	statusGrooming     = "grooming"
	statusActive       = "active"
	statusExploring    = "exploring"
	statusValidated    = "validated"
	statusPaused       = "paused"
	statusBlocked      = "blocked"
	statusPlanning     = "planning"
	statusPrioritizing = "prioritizing"

	emptyValue = ""
)
