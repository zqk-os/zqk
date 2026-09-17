package validation

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestEvaluateShovelReady_complete(t *testing.T) {
	bli := map[string]any{
		objects.FieldKeyID:              "BLI-test-ready",
		objects.FieldKeyRequirementRefs: []string{"REQ-1"},
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-1"},
		objects.FieldKeyEstimatedEffort: "2h",
		objects.FieldKeyPersonaRefs:     []string{"PER-coder"},
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	}
	got := EvaluateShovelReady(bli)
	if !got.Ready {
		t.Fatalf("expected ready, missing=%v", got.Missing)
	}
}

func TestEvaluateShovelReady_missingTraceAndScope(t *testing.T) {
	bli := map[string]any{
		objects.FieldKeyID:           "BLI-test-thin",
		objects.FieldKeyCriteriaRefs: []string{"CRIT-1"},
		objects.FieldKeyPersonaRef:   "PER-x",
		objects.FieldKeyStatus:       objects.ObjectStatusPlanned,
	}
	got := EvaluateShovelReady(bli)
	if got.Ready {
		t.Fatal("expected not ready")
	}
	if len(got.Missing) < 2 {
		t.Fatalf("expected multiple missing, got %v", got.Missing)
	}
}

func TestEvaluateShovelReady_blocked(t *testing.T) {
	bli := map[string]any{
		objects.FieldKeyID:                "BLI-test-blocked",
		objects.FieldKeyTechnicalSpecRefs: []string{"TDE-1"},
		objects.FieldKeyCriteriaRefs:      []string{"CRIT-1"},
		objects.FieldKeyEstimatedEffort:   "1h",
		objects.FieldKeyStakeholderRefs:   []string{"ACC-1"},
		objects.FieldKeyStatus:            objects.ObjectStatusBlocked,
	}
	got := EvaluateShovelReady(bli)
	if got.Ready {
		t.Fatal("blocked must not be shovel-ready")
	}
}

func TestEvaluateShovelReady_autoGroomingBypass(t *testing.T) {
	bli := map[string]any{
		objects.FieldKeyID:    "AUTO-BLI-PRI-1",
		objects.FieldKeyTitle: "Organize backlog",
	}
	if !IsShovelReady(bli) {
		t.Fatal("AUTO-BLI grooming must bypass gate")
	}
}
