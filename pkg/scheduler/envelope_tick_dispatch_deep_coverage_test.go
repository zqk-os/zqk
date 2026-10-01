package scheduler

import (
	"testing"
)

func TestExtended_EnvelopeTickDispatch_DeepCoverage(t *testing.T) {
	// 1. envelopeTickDispatchJobTypeAllowed
	extraAllow := map[string]struct{}{"custom_job": {}}
	if !envelopeTickDispatchJobTypeAllowed("cap_orchestrator", false, nil) {
		t.Errorf("expected core job_type cap_orchestrator to be allowed")
	}
	if !envelopeTickDispatchJobTypeAllowed("custom_job", false, extraAllow) {
		t.Errorf("expected extra allowlisted custom_job to be allowed")
	}
	if envelopeTickDispatchJobTypeAllowed("object_validation", false, nil) {
		t.Errorf("expected expanded job_type to not be allowed when expand is false")
	}
	if !envelopeTickDispatchJobTypeAllowed("object_validation", true, nil) {
		t.Errorf("expected expanded job_type to be allowed when expand is true")
	}
	if envelopeTickDispatchJobTypeAllowed("", true, nil) {
		t.Errorf("expected empty job_type to not be allowed")
	}

	// 2. envelopeTickDispatchCommaSeparatedSet
	set := envelopeTickDispatchCommaSeparatedSet("a, b , c, ,")
	if len(set) != 3 {
		t.Errorf("expected 3 items in set, got %d", len(set))
	}
	if envelopeTickDispatchCommaSeparatedSet("") != nil {
		t.Errorf("expected nil for empty string")
	}

	// 3. Env helpers
	env := map[string]string{
		EnvKeyEnvelopeTickDispatchAllowlistExtra: "job_a,job_b",
		EnvKeyEnvelopeTickDispatchDenyExtra:      "deny_a",
		EnvKeyEnvelopeTickDispatchMaxTriggers:    "5",
		EnvKeyEnvelopeTickDispatchMode:           "shadow",
		EnvKeyEnvelopeTickDispatchExpand:         "true",
	}
	allowExtra := envelopeTickDispatchAllowlistExtraFromEnv(env)
	if len(allowExtra) != 2 {
		t.Errorf("expected 2 extra allow items, got %d", len(allowExtra))
	}
	denyExtra := envelopeTickDispatchDenyExtraFromEnv(env)
	if len(denyExtra) != 1 {
		t.Errorf("expected 1 deny extra item, got %d", len(denyExtra))
	}
	maxTriggers, invalid := envelopeTickDispatchMaxTriggersFromEnv(env)
	if invalid || maxTriggers != 5 {
		t.Errorf("expected 5 max triggers, got %d invalid=%v", maxTriggers, invalid)
	}

	// Invalid max triggers
	envInvalid := map[string]string{EnvKeyEnvelopeTickDispatchMaxTriggers: "invalid"}
	_, invalid = envelopeTickDispatchMaxTriggersFromEnv(envInvalid)
	if !invalid {
		t.Errorf("expected invalid true for non-integer")
	}

	// Nil env
	if envelopeTickDispatchAllowlistExtraFromEnv(nil) != nil {
		t.Errorf("expected nil for nil env")
	}
	if envelopeTickDispatchDenyExtraFromEnv(nil) != nil {
		t.Errorf("expected nil for nil env")
	}
	maxT, inv := envelopeTickDispatchMaxTriggersFromEnv(nil)
	if inv || maxT != 0 {
		t.Errorf("expected 0, false for nil env")
	}

	// 4. envelopeTickDispatchHardDeny
	if !envelopeTickDispatchHardDeny(JobTypeDataCellEnvelopeTick) {
		t.Errorf("expected hard deny for envelope tick")
	}
	if !envelopeTickDispatchHardDeny(JobTypeCleanup) {
		t.Errorf("expected hard deny for cleanup")
	}
	if envelopeTickDispatchHardDeny("cap_orchestrator") {
		t.Errorf("expected false for cap_orchestrator")
	}

	// 5. dedupeEnvelopeTickResolvedJobTypes
	deduped, dropped := dedupeEnvelopeTickResolvedJobTypes([]string{"b", "a", "b", "c", "", " "})
	if dropped != 1 {
		t.Errorf("expected 1 duplicate dropped, got %d", dropped)
	}
	if len(deduped) != 3 || deduped[0] != "a" || deduped[1] != "b" || deduped[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", deduped)
	}

	// 6. envelopeTickDispatchParseTruthy
	if !envelopeTickDispatchParseTruthy("1") || !envelopeTickDispatchParseTruthy("true") || !envelopeTickDispatchParseTruthy("yes") || !envelopeTickDispatchParseTruthy("on") {
		t.Errorf("expected true for truthy values")
	}
	if envelopeTickDispatchParseTruthy("0") || envelopeTickDispatchParseTruthy("false") || envelopeTickDispatchParseTruthy("no") {
		t.Errorf("expected false for falsy values")
	}

	// 7. envelopeTickDispatchModeFromJob & envelopeTickDispatchExpandFromJob
	if envelopeTickDispatchModeFromJob(env) != "shadow" {
		t.Errorf("expected shadow mode, got %s", envelopeTickDispatchModeFromJob(env))
	}
	if envelopeTickDispatchModeFromJob(nil) != "" {
		t.Errorf("expected empty string for nil env")
	}
	if !envelopeTickDispatchExpandFromJob(env) {
		t.Errorf("expected expand true")
	}
	if envelopeTickDispatchExpandFromJob(nil) {
		t.Errorf("expected expand false for nil env")
	}
}
