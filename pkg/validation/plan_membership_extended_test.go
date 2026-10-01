package validation

import (
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPlanMembershipHelpers(t *testing.T) {
	// PlanStatusAllowsBacklogInProgress
	if !PlanStatusAllowsBacklogInProgress(objects.ObjectStatusActive) {
		t.Errorf("expected active to allow backlog in progress")
	}
	if !PlanStatusAllowsBacklogInProgress(objects.ObjectStatusInProgress) {
		t.Errorf("expected in_progress to allow backlog in progress")
	}
	if PlanStatusAllowsBacklogInProgress(objects.ObjectStatusPlanned) {
		t.Errorf("expected planned to not allow backlog in progress")
	}

	// ErrBacklogInProgressRequiresExecutionFacingPlan
	err := ErrBacklogInProgressRequiresExecutionFacingPlan("BLI-1", "PLAN-1", "draft")
	if err == nil || err.Error() == "" {
		t.Errorf("expected non-empty error message")
	}

	// planStatusRequiresReadyChildren
	if !planStatusRequiresReadyChildren(objects.ObjectStatusActive) {
		t.Errorf("expected active to require ready children")
	}
	if planStatusRequiresReadyChildren("custom_unknown_status") {
		t.Errorf("expected unknown status to not require ready children")
	}

	// BacklogItemStatusReadyOrLater
	if !BacklogItemStatusReadyOrLater(objects.ObjectStatusPlanned) {
		t.Errorf("expected planned to be ready or later")
	}
	if !BacklogItemStatusReadyOrLater(objects.ObjectStatusInProgress) {
		t.Errorf("expected in_progress to be ready or later")
	}
	if !BacklogItemStatusReadyOrLater(objects.ObjectStatusComplete) {
		t.Errorf("expected complete to be ready or later")
	}
	if !BacklogItemStatusReadyOrLater(objects.ObjectStatusArchived) {
		t.Errorf("expected archived to be ready or later")
	}
	if BacklogItemStatusReadyOrLater("nonexistent_status") {
		t.Errorf("expected nonexistent to not be ready or later")
	}

	// backlogItemAllowedOnExecutionFacingPlan
	if !backlogItemAllowedOnExecutionFacingPlan(objects.ObjectStatusPlanned) {
		t.Errorf("expected planned to be allowed on execution facing plan")
	}
	if !backlogItemAllowedOnExecutionFacingPlan(objects.ObjectStatusError) {
		t.Errorf("expected error to be allowed on execution facing plan for recovery")
	}

	// BacklogItemStatusTerminalForPlanCompletion
	if !BacklogItemStatusTerminalForPlanCompletion(objects.ObjectStatusComplete) {
		t.Errorf("expected complete to be terminal")
	}
	if !BacklogItemStatusTerminalForPlanCompletion(objects.ObjectStatusArchived) {
		t.Errorf("expected archived to be terminal")
	}
	if !BacklogItemStatusTerminalForPlanCompletion(objects.ObjectStatusRejected) {
		t.Errorf("expected rejected to be terminal")
	}
	if BacklogItemStatusTerminalForPlanCompletion(objects.ObjectStatusInProgress) {
		t.Errorf("expected in_progress not to be terminal")
	}

	// BacklogItemStatusInProgressOrComplete
	if !BacklogItemStatusInProgressOrComplete(objects.ObjectStatusInProgress) {
		t.Errorf("expected in_progress to be in progress or complete")
	}
	if !BacklogItemStatusInProgressOrComplete(objects.ObjectStatusComplete) {
		t.Errorf("expected complete to be in progress or complete")
	}
	if BacklogItemStatusInProgressOrComplete(objects.ObjectStatusPlanned) {
		t.Errorf("expected planned not to be in progress or complete")
	}
}

func TestLinkedBacklogItemsEvaluation(t *testing.T) {
	// Empty planID
	if LinkedBacklogItemsAllTerminal("", nil, nil) {
		t.Errorf("expected false on empty planID")
	}
	if LinkedBacklogItemsAllReadyOrLater("", nil, nil) {
		t.Errorf("expected false on empty planID")
	}
	if LinkedBacklogItemsNoneInProgressOrComplete("", nil, nil) {
		t.Errorf("expected false on empty planID")
	}

	// Nil lookups
	if LinkedBacklogItemsAllTerminal("PLAN-1", nil, nil) {
		t.Errorf("expected false on nil options")
	}
	if LinkedBacklogItemsAllReadyOrLater("PLAN-1", nil, nil) {
		t.Errorf("expected false on nil options")
	}
	if LinkedBacklogItemsNoneInProgressOrComplete("PLAN-1", nil, nil) {
		t.Errorf("expected false on nil options")
	}

	// DependentsLookup returns nil (unready/unverified)
	opts := &ValidationOptions{
		DependentsLookup: func(id string) []string {
			return nil
		},
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusComplete, nil
		},
	}
	if LinkedBacklogItemsAllTerminal("PLAN-1", opts, nil) {
		t.Errorf("expected false when DependentsLookup returns nil")
	}
	if LinkedBacklogItemsAllReadyOrLater("PLAN-1", opts, nil) {
		t.Errorf("expected false when DependentsLookup returns nil")
	}
	if LinkedBacklogItemsNoneInProgressOrComplete("PLAN-1", opts, nil) {
		t.Errorf("expected false when DependentsLookup returns nil")
	}

	// Valid empty dependents
	optsEmpty := &ValidationOptions{
		DependentsLookup: func(id string) []string {
			return []string{}
		},
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusComplete, nil
		},
	}
	if !LinkedBacklogItemsAllTerminal("PLAN-1", optsEmpty, nil) {
		t.Errorf("expected true when empty dependents")
	}
	if !LinkedBacklogItemsAllReadyOrLater("PLAN-1", optsEmpty, nil) {
		t.Errorf("expected true when empty dependents")
	}
	if !LinkedBacklogItemsNoneInProgressOrComplete("PLAN-1", optsEmpty, nil) {
		t.Errorf("expected true when empty dependents")
	}

	// Dependents with different statuses
	objectsDB := map[string]map[string]any{
		"BLI-1": {
			"id":                            "BLI-1",
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
		},
		"BLI-2": {
			"id":                            "BLI-2",
			objects.FieldKeyPriorityPlanRef: "PLAN-OTHER", // unlinked
		},
		"BLI-3": {
			"id":                            "BLI-3",
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
		},
	}
	statuses := map[string]string{
		"BLI-1": objects.ObjectStatusComplete,
		"BLI-3": objects.ObjectStatusInProgress,
	}

	inferKind := func(id string) string {
		if id == "OTHER-1" {
			return "other_kind"
		}
		return objects.KindBacklogItem
	}

	optsWithDeps := &ValidationOptions{
		DependentsLookup: func(id string) []string {
			return []string{"", "OTHER-1", "BLI-1", "BLI-2", "BLI-GHOST", "BLI-3"}
		},
		ObjectLookup: func(id string) (map[string]any, error) {
			if obj, ok := objectsDB[id]; ok {
				return obj, nil
			}
			return nil, errors.New("not found")
		},
		ObjectStatusLookup: func(id string) (string, error) {
			if st, ok := statuses[id]; ok {
				return st, nil
			}
			return "", errors.New("not found")
		},
	}

	// BLI-3 is in_progress, so not all terminal
	if LinkedBacklogItemsAllTerminal("PLAN-1", optsWithDeps, inferKind) {
		t.Errorf("expected false for LinkedBacklogItemsAllTerminal because BLI-3 is in_progress")
	}
	// BLI-3 is in_progress, so LinkedBacklogItemsNoneInProgressOrComplete should be false
	if LinkedBacklogItemsNoneInProgressOrComplete("PLAN-1", optsWithDeps, inferKind) {
		t.Errorf("expected false for LinkedBacklogItemsNoneInProgressOrComplete because BLI-3 is in_progress")
	}

	// Now set BLI-3 to complete
	statuses["BLI-3"] = objects.ObjectStatusComplete
	if !LinkedBacklogItemsAllTerminal("PLAN-1", optsWithDeps, inferKind) {
		t.Errorf("expected true for LinkedBacklogItemsAllTerminal now that all are complete")
	}
	if LinkedBacklogItemsNoneInProgressOrComplete("PLAN-1", optsWithDeps, inferKind) {
		t.Errorf("expected false because complete counts as in_progress or complete")
	}

	// Now set BLI-1 and BLI-3 to planned
	statuses["BLI-1"] = objects.ObjectStatusPlanned
	statuses["BLI-3"] = objects.ObjectStatusPlanned
	if !LinkedBacklogItemsNoneInProgressOrComplete("PLAN-1", optsWithDeps, inferKind) {
		t.Errorf("expected true when all are planned")
	}
	if !LinkedBacklogItemsAllReadyOrLater("PLAN-1", optsWithDeps, inferKind) {
		t.Errorf("expected true for LinkedBacklogItemsAllReadyOrLater when all are planned")
	}
}
