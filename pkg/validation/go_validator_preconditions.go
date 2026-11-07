package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/predicate"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// evalPrecondDecideTable is the DECIDE first-match walk. Unrecognized strings
// fail closed (handled=false, met=false) unless ZQK_PRECONDITIONS_FAIL_OPEN=1
// is explicitly set for backwards compatibility. Overlay DSL provides the fail-closed path.
func (gv *GoValidator) evalPrecondDecideTable(normalized string, obj map[string]any, options *ValidationOptions) (ruleName string, handled, met bool) {
	for _, rule := range precondDecideRules {
		matched, ruleMet := rule.eval(gv, normalized, obj, options)
		if matched {
			return rule.name, true, ruleMet
		}
	}
	if zqkenv.PreconditionsFailOpen().Get() == "1" {
		return "", false, true
	}
	return "", false, false
}

// dispatchPrecondition runs validation.lifecycle_precondition rules sequentially.
func (gv *GoValidator) dispatchPrecondition(precondition string, obj map[string]any, options *ValidationOptions, recognized *bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(precondition))
	if normalized == "" {
		if recognized != nil {
			*recognized = true
		}
		return true
	}
	_, handled, met := gv.evalPrecondDecideTable(normalized, obj, options)
	if recognized != nil {
		*recognized = handled
	}
	return met
}

// EvaluatePrecondition evaluates a single precondition string against obj and returns whether it was met and recognized.
func (gv *GoValidator) EvaluatePrecondition(precondition string, obj map[string]any, options *ValidationOptions) (met bool, recognized bool) {
	return gv.evaluatePrecondition(precondition, obj, options)
}

func (gv *GoValidator) evaluatePrecondition(precondition string, obj map[string]any, options *ValidationOptions) (met bool, recognized bool) {
	if precondition == "" {
		return true, false
	}
	met = gv.dispatchPrecondition(precondition, obj, options, &recognized)
	return met, recognized
}

func (gv *GoValidator) checkPrecondition(precondition string, obj map[string]any, options *ValidationOptions) bool {
	return gv.dispatchPrecondition(precondition, obj, options, nil)
}

// FormatUnmetPreconditionDetail enriches precondition failure messages with actionable details
// for specific invariants like priority plan validation or git mutation evidence.
func FormatUnmetPreconditionDetail(precondition string, obj map[string]any) string {
	return formatUnmetPreconditionDetail(precondition, obj)
}

func formatUnmetPreconditionDetail(precondition string, obj map[string]any) string {
	return formatUnmetPreconditionDetailWithReason(precondition, obj, nil)
}

// errGitEvidenceNoProjectRoot is reported when no git project root can be resolved for evidence checks.
var errGitEvidenceNoProjectRoot = errors.New("no git project root resolved for commit_hashes evidence")

// escapeDoubleQuotedLiteral safely escapes user-provided content before embedding into
// text that may be consumed in double-quoted contexts.
func escapeDoubleQuotedLiteral(raw string) string {
	escaped := strings.ReplaceAll(raw, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return escaped
}

// formatUnmetPreconditionDetailForOptions enriches an unmet precondition and, for the git
// mutation evidence gate, re-derives the concrete per-commit rejection reason so operators
// see which SHA failed and why (SHA not found, merge commit, missing id mention, process-only paths).
func (gv *GoValidator) formatUnmetPreconditionDetailForOptions(precondition string, obj map[string]any, options *ValidationOptions) string {
	var reason error
	if gv != nil && isGitMutationEvidencePrecondition(precondition) {
		reason = gv.commitHashesGitMutationEvidenceErr(obj, options)
	}
	return formatUnmetPreconditionDetailWithReason(precondition, obj, reason)
}

// isGitMutationEvidencePrecondition reports whether precondition is the (un-enriched) git mutation evidence gate.
func isGitMutationEvidencePrecondition(precondition string) bool {
	normPre := strings.ToLower(strings.TrimSpace(precondition))
	return (strings.Contains(normPre, "git mutation evidence") || strings.Contains(normPre, predicate.GateGitMutationEvidencePresent)) &&
		!strings.Contains(normPre, ExplanationGitMutationEvidencePrefix)
}

// formatGitMutationEvidenceExplanation is the single formatter for the git mutation
// evidence hint: accepted id mentions, canonical [BLI-<id>] tag, and (when known) the
// concrete per-commit rejection reason from pkg/gitevidence.
func formatGitMutationEvidenceExplanation(obj map[string]any, reason error) string {
	bliID, _ := obj[objects.FieldKeyID].(string)
	bliID = strings.TrimSpace(bliID)
	if bliID == "" {
		bliID = ExplanationGitMutationEvidencePlaceholderID
	}
	safeBliID := escapeDoubleQuotedLiteral(bliID)
	accepted := safeBliID
	if planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string); strings.TrimSpace(planRef) != "" {
		safePlanRef := escapeDoubleQuotedLiteral(strings.TrimSpace(planRef))
		accepted = fmt.Sprintf(ExplanationGitMutationEvidencePlanAltFormat, safeBliID, safePlanRef)
	}
	tag := fmt.Sprintf(ExplanationGitMutationEvidenceTagFormat, safeBliID)
	explanation := fmt.Sprintf(ExplanationGitMutationEvidenceFormat, accepted, tag)
	if reason != nil {
		explanation = fmt.Sprintf(ExplanationGitMutationEvidenceRejectedFormat, explanation, reason)
	}
	return explanation
}

func formatUnmetPreconditionDetailWithReason(precondition string, obj map[string]any, gitEvidenceReason error) string {
	normPre := strings.ToLower(strings.TrimSpace(precondition))
	if (normPre == strings.ToLower(PrecondPriorityPlanValidated) || strings.Contains(normPre, "priority plan validated") || strings.Contains(normPre, "priority_plan_validated")) &&
		!strings.Contains(normPre, ReasonPriorityPlanWrittenIdentity) && !strings.Contains(normPre, ReasonPriorityPlanWorkstreamLane) {
		var reasons []string
		if !objects.PlanHasWrittenIdentity(obj) {
			reasons = append(reasons, ReasonPriorityPlanWrittenIdentity)
		}
		if !objects.PlanHasWorkstreamLane(obj) {
			reasons = append(reasons, ReasonPriorityPlanWorkstreamLane)
		}
		if len(reasons) > 0 {
			return fmt.Sprintf("%s: %s", precondition, strings.Join(reasons, "; "))
		}
	} else if isGitMutationEvidencePrecondition(precondition) {
		return fmt.Sprintf("%s (%s)", precondition, formatGitMutationEvidenceExplanation(obj, gitEvidenceReason))
	}
	return precondition
}
