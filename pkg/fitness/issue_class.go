// Package fitness separates lifecycle process position from contextual fitness findings.
//
// demote→error only for IssueClassProcessFailure; surfaces filter the rest.
package fitness

import (
	"strings"
)

// IssueClass classifies why an object is unsafe or incomplete for a use.
// It is orthogonal to lifecycle status (process position).
type IssueClass string

const (
	IssueClassProcessFailure       IssueClass = "process_failure"
	IssueClassDataCompleteness     IssueClass = "data_completeness"
	IssueClassEmploymentFitness    IssueClass = "employment_fitness"
	IssueClassReferentialIntegrity IssueClass = "referential_integrity"
	IssueClassPolicyGate           IssueClass = "policy_gate"
	IssueClassUnknown              IssueClass = "unknown"
)

// IssueInput is the minimal check/validation signal needed to classify a finding.
type IssueInput struct {
	Tier        int
	Category    string
	Message     string
	AutoFixable bool
}

// ClassifyIssue maps a check issue to an IssueClass.
// process_failure is reserved for illegal lifecycle tokens and status preconditions —
// the only class that may drive autofix demote→error (when the kind supports error).
func ClassifyIssue(in IssueInput) IssueClass {
	msg := strings.TrimSpace(in.Message)
	cat := strings.ToLower(strings.TrimSpace(in.Category))

	if cat == "integrity" {
		// Disk/CAS integrity is not a lifecycle demote signal.
		return IssueClassReferentialIntegrity
	}

	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "Invalid lifecycle status"),
		strings.Contains(msg, "Precondition not met for status"),
		strings.Contains(msg, "Precondition not met for transition"):
		return IssueClassProcessFailure
	case strings.Contains(lower, "rbac"),
		strings.Contains(lower, "employ"),
		strings.Contains(lower, "agent-usable"),
		strings.Contains(lower, "persona_ref"),
		strings.Contains(lower, "api_key"),
		strings.Contains(lower, "credential"):
		return IssueClassEmploymentFitness
	case strings.Contains(lower, "orphan"),
		strings.Contains(lower, "dangling"),
		strings.Contains(lower, "broken reference"),
		strings.Contains(lower, "does not exist"),
		strings.Contains(lower, "missing reference"):
		return IssueClassReferentialIntegrity
	case strings.Contains(lower, "policy"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "not allowed"):
		return IssueClassPolicyGate
	case strings.Contains(lower, "required"),
		strings.Contains(lower, "missing"),
		strings.Contains(lower, "empty"),
		strings.Contains(lower, "must have"),
		strings.Contains(lower, "incomplete"):
		return IssueClassDataCompleteness
	case cat == "lifecycle":
		// Other lifecycle messages without illegal/precondition phrasing: treat as process.
		return IssueClassProcessFailure
	case cat == "policy":
		return IssueClassPolicyGate
	case cat == "reference":
		return IssueClassReferentialIntegrity
	default:
		// Fail closed for demote: unknown field/shape issues are completeness, not process failure.
		return IssueClassDataCompleteness
	}
}

// MayDemoteLifecycleToError reports whether this class may mutate status→error.
func MayDemoteLifecycleToError(c IssueClass) bool {
	return c == IssueClassProcessFailure
}
