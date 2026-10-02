package validation

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Plan/backlog membership helpers remain here for lifecycle precondition checks
// (PrecondReadyBacklogReferencesPlan, airtight lock). Child-owned link refuse for
// execution-facing plans is composed: OpRefuseExecutionFacingMembership in
// pkg/kernelcas/compose (no Go customRuleValidators wrap).
// TRACK: deleted membership init wrap.

func statusRole(kind, status string) string {
	return objects.GetGlobalStatusChecker().Role(kind, status)
}

// planStatusRequiresReadyChildren reports whether linking a non-ready BLI is forbidden.
func planStatusRequiresReadyChildren(planStatus string) bool {
	role := statusRole(objects.KindPriorityPlan, planStatus)
	if role != "" {
		return objects.RolePlanRequiresReadyChildren(role)
	}
	// Fallback until all plan statuses carry role annotations.
	switch strings.ToLower(strings.TrimSpace(planStatus)) {
	case objects.ObjectStatusActive, objects.ObjectStatusInProgress:
		return true
	default:
		return false
	}
}

// BacklogItemStatusReadyOrLater is shovel_ready / execution_locked / terminal (not halted).
// Used for airtight lock (→in_progress): halted/error does NOT count — lock stays blocked until repaired.
func BacklogItemStatusReadyOrLater(status string) bool {
	role := statusRole(objects.KindBacklogItem, status)
	if role != "" {
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

// backlogItemAllowedOnExecutionFacingPlan is membership (not lock): realign roles are scope creep;
// halted is allowed so D11 recovery (error→planned) is not deadlocked.
// TRACK: remove status=error snowflake once roles land (this file).
func backlogItemAllowedOnExecutionFacingPlan(status string) bool {
	role := statusRole(objects.KindBacklogItem, status)
	if role != "" {
		return objects.RoleAllowedOnExecutionFacingPlan(role)
	}
	if BacklogItemStatusReadyOrLater(status) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(status), objects.ObjectStatusError)
}

// BacklogItemStatusTerminalForPlanCompletion is complete/archived/rejected (role=terminal).
// Used so priority_plan cannot sit in complete while shovel-ready work is still linked.
func BacklogItemStatusTerminalForPlanCompletion(status string) bool {
	role := statusRole(objects.KindBacklogItem, status)
	if role == objects.LifecycleRoleTerminal {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case objects.ObjectStatusComplete, objects.ObjectStatusArchived, objects.ObjectStatusRejected:
		return true
	default:
		return false
	}
}

// backlogDependentStillReferencesPlan reports whether depID still points at planID.
// When ObjectLookup is set, stale reverse-index entries (unlinked BLIs, CHA-nested
// snapshots) are ignored. When ObjectLookup is nil, trust DependentsLookup.
// TRACK: pair with reverse-ref persist rebuild.
func backlogDependentStillReferencesPlan(planID, depID string, options *ValidationOptions) bool {
	if options == nil || options.ObjectLookup == nil {
		return true
	}
	obj, err := options.ObjectLookup(depID)
	if err != nil || obj == nil {
		return false
	}
	ref, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
	return strings.TrimSpace(ref) == planID
}

// iterateLinkedBacklogItems walks all validated backlog items referencing planID and calls checkStatus.
// Returns false immediately if planID is invalid, lookups are unavailable, or checkStatus returns false.
func iterateLinkedBacklogItems(planID string, options *ValidationOptions, inferKind func(string) string, skipLookupErr bool, checkStatus func(status string) bool) bool {
	planID = strings.TrimSpace(planID)
	if planID == emptyValue {
		return false
	}
	if options == nil || options.DependentsLookup == nil || options.ObjectStatusLookup == nil {
		return false
	}
	if inferKind == nil {
		inferKind = GetIDValidator().InferKindFromID
	}
	deps := options.DependentsLookup(planID)
	if deps == nil {
		// nil slice indicates unready or unverified reverse index (fail-closed).
		return false
	}
	for _, depID := range deps {
		if depID == emptyValue {
			continue
		}
		kind := inferKind(depID)
		if kind != emptyValue && kind != objects.KindBacklogItem {
			continue
		}
		if !backlogDependentStillReferencesPlan(planID, depID, options) {
			continue
		}
		st, err := options.ObjectStatusLookup(depID)
		if err != nil {
			if skipLookupErr {
				// Ghost reverse-index entries (deleted BLIs) must not block plan-complete/plan-promote.
				continue
			}
			return false
		}
		if !checkStatus(st) {
			return false
		}
	}
	return true
}

// LinkedBacklogItemsAllTerminal evaluates PrecondLinkedBacklogAllTerminal.
// Fail-closed when DependentsLookup/ObjectStatusLookup are unavailable.
// Vacuous true when no backlog_item dependents are found.
func LinkedBacklogItemsAllTerminal(planID string, options *ValidationOptions, inferKind func(string) string) bool {
	return iterateLinkedBacklogItems(planID, options, inferKind, true, func(st string) bool {
		return BacklogItemStatusTerminalForPlanCompletion(st)
	})
}

// LinkedBacklogItemsAllReadyOrLater evaluates PrecondAllLinkedBacklogReadyOrLater.
// Fail-closed when DependentsLookup/ObjectStatusLookup are unavailable or dependents are unverified (nil).
// Vacuous true only when verified that zero backlog_item dependents exist (empty non-nil slice).
func LinkedBacklogItemsAllReadyOrLater(planID string, options *ValidationOptions, inferKind func(string) string) bool {
	return iterateLinkedBacklogItems(planID, options, inferKind, false, func(st string) bool {
		return BacklogItemStatusReadyOrLater(st)
	})
}

// BacklogItemStatusInProgressOrComplete checks if status represents active work or completion
func BacklogItemStatusInProgressOrComplete(status string) bool {
	role := statusRole(objects.KindBacklogItem, status)
	if role == objects.LifecycleRoleExecutionLocked {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case objects.ObjectStatusInProgress, objects.ObjectStatusComplete:
		return true
	default:
		return false
	}
}

// LinkedBacklogItemsNoneInProgressOrComplete evaluates PrecondNoLinkedBacklogInProgressOrComplete.
// Fail-closed when DependentsLookup/ObjectStatusLookup are unavailable.
func LinkedBacklogItemsNoneInProgressOrComplete(planID string, options *ValidationOptions, inferKind func(string) string) bool {
	return iterateLinkedBacklogItems(planID, options, inferKind, true, func(st string) bool {
		return !BacklogItemStatusInProgressOrComplete(st)
	})
}
