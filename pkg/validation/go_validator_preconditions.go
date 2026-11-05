package validation

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
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
	} else if (strings.Contains(normPre, "git mutation evidence") || strings.Contains(normPre, "git_mutation_evidence")) &&
		!strings.Contains(normPre, "ensure at least one commit touches code_location") {
		bliID, _ := obj[objects.FieldKeyID].(string)
		bliID = strings.TrimSpace(bliID)
		if bliID == "" {
			bliID = "BLI-xxx"
		}
		return fmt.Sprintf("%s (%s)", precondition, fmt.Sprintf(ExplanationGitMutationEvidenceFormat, bliID))
	}
	return precondition
}
