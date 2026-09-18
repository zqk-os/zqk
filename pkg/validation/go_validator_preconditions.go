package validation

import (
	"strings"

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
