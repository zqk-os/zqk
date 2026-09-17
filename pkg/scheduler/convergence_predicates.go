package scheduler

import (
	"strings"

	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/when"
)

// EvaluateConvergencePredicateReadiness evaluates optional machine-readable hints in thresholds
// against the latest test-bundle snapshot. This does not perform lifecycle transitions (those remain
// manual or future automation); it answers whether documented start/completion gates would pass.
//
// Conventions (thresholds object on convergence_session):
//   - start_gate: object with optional keys:
//   - min_lines_in_window (int, default 1): require at least N health lines in the measure window.
//   - completion_gate: object with optional keys:
//   - require_ready_for_session_completion (bool, default true): require snap.ReadyForSessionCompletion.
//     When false, completion is treated as open for predicate/rollup purposes even if health.jsonl is empty
//     or ready_for_session_completion is false (product-delivery / docs sessions that are not bundle-gated).
func EvaluateConvergencePredicateReadiness(status string, thresholds map[string]any, snap *TestBundleConvergenceSnapshot) (startGateOpen, completionGateOpen bool) {
	bundleCompletionBypass := false
	if cg := mapFromThresholds(thresholds, "completion_gate"); cg != nil && !completionGateRequiresReadyForSessionCompletion(cg) {
		bundleCompletionBypass = true
	}
	completionGateOpen = when.Result[bool]().
		When(func() bool { return bundleCompletionBypass }).Then(func() bool { return true }).
		OrElseWhen(func() bool { return snap == nil }).Then(func() bool { return false }).
		OrElse(func() bool { return snap.ReadyForSessionCompletion }).
		Run()

	if strings.TrimSpace(status) != "draft" {
		startGateOpen = true
		return startGateOpen, completionGateOpen
	}
	sg := mapFromThresholds(thresholds, "start_gate")
	if sg == nil {
		// No structured start gate: operator uses text start_condition + manual activation.
		startGateOpen = false
		return startGateOpen, completionGateOpen
	}
	if snap == nil {
		startGateOpen = false
		return startGateOpen, completionGateOpen
	}
	minLines := 1
	if v, ok := sg["min_lines_in_window"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				minLines = n
			}
		case int64:
			if n > 0 {
				minLines = int(n)
			}
		case float64:
			if n > 0 {
				minLines = int(n)
			}
		}
	}
	startGateOpen = snap.LinesInWindow >= minLines
	return startGateOpen, completionGateOpen
}

// completionGateRequiresReadyForSessionCompletion returns whether rollup/predicates should require
// snap.ReadyForSessionCompletion. Default true when the key is absent. Coerces string/float
// shapes from YAML/JSON decoders.
func completionGateRequiresReadyForSessionCompletion(cg map[string]any) bool {
	if cg == nil {
		return true
	}
	raw, ok := cg["require_ready_for_session_completion"]
	if !ok {
		return true
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return true
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		switch s {
		case "false", "0", "no", "off":
			return false
		case "true", "1", "yes", "on":
			return true
		default:
			return true
		}
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return true
	}
}

func mapFromThresholds(thresholds map[string]any, key string) map[string]any {
	if thresholds == nil {
		return nil
	}
	raw, ok := thresholds[key]
	if !ok {
		return nil
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return m
}

// ThresholdsMapFromObject returns the session thresholds map from a convergence_session object map, or nil.
func ThresholdsMapFromObject(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	raw, ok := obj[objects.FieldKeyThresholds]
	if !ok {
		return nil
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return m
}

// ThresholdsCompletionGateRequiresReadyForSessionCompletion reports whether thresholds.completion_gate
// requires snap.ReadyForSessionCompletion for completion-gate semantics (rollup bundle leg).
// Defaults to true when thresholds or completion_gate is absent — same rule as EvaluateConvergencePredicateReadiness.
func ThresholdsCompletionGateRequiresReadyForSessionCompletion(thresholds map[string]any) bool {
	cg := mapFromThresholds(thresholds, "completion_gate")
	return completionGateRequiresReadyForSessionCompletion(cg)
}

// CompletionGateObservabilityNote returns one line for phase_router.notes when thresholds.completion_gate
// is present, describing how rollup interprets bundle readiness. Empty when there is no completion_gate block.
func CompletionGateObservabilityNote(thresholds map[string]any) string {
	cg := mapFromThresholds(thresholds, "completion_gate")
	if cg == nil {
		return ""
	}
	if completionGateRequiresReadyForSessionCompletion(cg) {
		return "Observed: thresholds.completion_gate.require_ready_for_session_completion is true (or default): rollup_status_core uses test-bundle readiness (ReadyForSessionCompletion) for completion-gate semantics."
	}
	return "Observed: thresholds.completion_gate.require_ready_for_session_completion is false: rollup_status_core does not use test-bundle health as the completion gate (health.jsonl remains informative)."
}
