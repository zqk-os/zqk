package validation

import (
	"strings"
)

// evalPrecondDecideTable is the DECIDE first-match walk. Unrecognized strings
// fail-open (handled=false, met=true). Overlay DSL remains the fail-closed path.
func (gv *GoValidator) evalPrecondDecideTable(normalized string, obj map[string]any, options *ValidationOptions) (ruleName string, handled, met bool) {
	for _, rule := range precondDecideRules {
		matched, ruleMet := rule.eval(gv, normalized, obj, options)
		if matched {
			return rule.name, true, ruleMet
		}
	}
	return "", false, true
}

// dispatchPrecondition runs validation.lifecycle_precondition rules sequentially.
func (gv *GoValidator) dispatchPrecondition(precondition string, obj map[string]any, options *ValidationOptions, recognized *bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(precondition))
	_, handled, met := gv.evalPrecondDecideTable(normalized, obj, options)
	if recognized != nil {
		*recognized = handled
	}
	return met
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
