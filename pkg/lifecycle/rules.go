// Package lifecycle: transition rules define when a criterion (with scope) triggers a status transition.
//
// Process traceability: formal requirement [REDACTED-ID] (goal [REDACTED-ID])
// requires extending this set with milestone-scoped CriterionSatisfied → milestone complete when the
// verification WAL emits events (see docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md).
// Today only all_backlog_items_complete_for_plan → priority_plan complete is wired (REQ-014).

package lifecycle

// TransitionRule defines: when criterion C with scope S is satisfied, trigger transition to (Kind, ID, ToStatus).
// ID can be taken from scope (e.g. scope["plan_id"]).
type TransitionRule struct {
	CriterionID string // e.g. "all_backlog_items_complete_for_plan"
	ScopeKey    string // e.g. "plan_id=PRI-219" or "" for any scope
	Kind        string // object kind to update
	IDFromScope string // key in scope that provides the object ID, e.g. "plan_id"
	ToStatus    string // target status
}

// DefaultTransitionRules returns the in-code transition rules (REQ-014 priority plan completion;
// [REDACTED-ID] milestone completion when linked criteria are satisfied).
func DefaultTransitionRules() []TransitionRule {
	return []TransitionRule{
		{
			CriterionID: criterionAllBacklogComplete,
			Kind:        kindPriorityPlan,
			IDFromScope: scopePlanID,
			ToStatus:    statusComplete,
		},
		{
			CriterionID: criterionAllBacklogCompleteForMilestone,
			Kind:        kindMilestone,
			IDFromScope: scopeMilestoneID,
			ToStatus:    statusComplete,
		},
		{
			CriterionID: criterionAllCriteriaCompleteForMilestone,
			Kind:        kindMilestone,
			IDFromScope: scopeMilestoneID,
			ToStatus:    statusComplete,
		},
		{
			CriterionID: criterionAllAcceptanceCriteriaMetForBacklogItem,
			Kind:        kindBacklogItem,
			IDFromScope: scopeBacklogItemID,
			ToStatus:    statusComplete,
		},
	}
}

// RuleMatch returns true if the rule matches the given criterion and scope.
func (r *TransitionRule) RuleMatch(criterionID, scopeKey string, scope map[string]string) bool {
	if r.CriterionID != criterionID {
		return false
	}
	if r.ScopeKey != emptyValue && r.ScopeKey != scopeKey {
		return false
	}
	if r.IDFromScope != emptyValue {
		if _, ok := scope[r.IDFromScope]; !ok {
			return false
		}
	}
	return true
}

// ObjectID returns the target object ID from scope.
func (r *TransitionRule) ObjectID(scope map[string]string) string {
	if r.IDFromScope == emptyValue {
		return emptyValue
	}
	return scope[r.IDFromScope]
}
