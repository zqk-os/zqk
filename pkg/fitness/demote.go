package fitness

import "strings"

// DemoteDecision is the autofix demote→error policy outcome.
type DemoteDecision struct {
	Demote bool
	Reason string
	Class  IssueClass
}

// ShouldDemoteToError decides whether unresolved issues justify status→error.
// Only IssueClassProcessFailure may demote, and only when AutofixMayDemoteKind(kind).
// Soft field/completeness issues must not yank process position.
func ShouldDemoteToError(kind, currentStatus string, issues []IssueInput) DemoteDecision {
	switch currentStatus {
	case "", "error":
		return DemoteDecision{}
	}
	if !AutofixMayDemoteKind(kind) {
		return DemoteDecision{}
	}

	for _, in := range issues {
		if strings.EqualFold(strings.TrimSpace(in.Category), "integrity") {
			continue
		}
		c := ClassifyIssue(in)
		if !MayDemoteLifecycleToError(c) {
			continue
		}
		// True terminals: only yank when the status token itself is illegal for the kind.
		switch currentStatus {
		case "archived", "rejected", "deferred", "deprecated", "superseded", "answered", "resolved":
			if !strings.Contains(in.Message, "Invalid lifecycle status") {
				continue
			}
		}
		reason := in.Message
		if reason == "" {
			reason = "process_failure"
		}
		return DemoteDecision{Demote: true, Reason: reason, Class: c}
	}
	return DemoteDecision{}
}

// DemoteReasonFromIssues picks the highest-signal process_failure message for notes.
func DemoteReasonFromIssues(issues []IssueInput) string {
	var fallback string
	for _, in := range issues {
		if strings.EqualFold(strings.TrimSpace(in.Category), "integrity") {
			continue
		}
		if ClassifyIssue(in) != IssueClassProcessFailure {
			continue
		}
		msg := in.Message
		if msg == "" {
			continue
		}
		if strings.Contains(msg, "Invalid lifecycle status") || strings.Contains(msg, "Precondition not met for status") {
			return msg
		}
		if fallback == "" {
			fallback = msg
		}
	}
	if fallback != "" {
		return fallback
	}
	return "process_failure"
}
