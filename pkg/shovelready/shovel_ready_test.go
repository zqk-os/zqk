package shovelready

import (
	"slices"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func readyBLI() map[string]any {
	return map[string]any{
		objects.FieldKeyID:              "BLI-test-ready",
		objects.FieldKeyRequirementRefs: []string{"REQ-1"},
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-1"},
		objects.FieldKeyEstimatedEffort: "2h",
		objects.FieldKeyPersonaRefs:     []string{"PER-coder"},
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	}
}

func TestEvaluate_holes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(map[string]any)
		ready   bool
		missing []string
	}{
		{
			name:   "complete",
			mutate: func(map[string]any) {},
			ready:  true,
		},
		{
			name: "missing requirement and tspec",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyRequirementRefs)
			},
			ready:   false,
			missing: []string{"requirement_refs|technical_spec_refs"},
		},
		{
			name: "tspec substitutes for requirement",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyRequirementRefs)
				bli[objects.FieldKeyTechnicalSpecRefs] = []string{"TDE-1"}
			},
			ready: true,
		},
		{
			name: "missing criteria",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyCriteriaRefs)
			},
			ready:   false,
			missing: []string{objects.FieldKeyCriteriaRefs},
		},
		{
			name: "missing effort and scope",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyEstimatedEffort)
			},
			ready:   false,
			missing: []string{objects.FieldKeyEstimatedEffort + "|scope_signal"},
		},
		{
			name: "P0 substitutes for estimated_effort",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyEstimatedEffort)
				bli[objects.FieldKeyPriorityTier] = "P0"
			},
			ready: true,
		},
		{
			name: "missing persona",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyPersonaRefs)
			},
			ready:   false,
			missing: []string{"persona|stakeholders"},
		},
		{
			name: "stakeholders substitute for persona",
			mutate: func(bli map[string]any) {
				delete(bli, objects.FieldKeyPersonaRefs)
				bli[objects.FieldKeyStakeholders] = "TPM"
			},
			ready: true,
		},
		{
			name: "blocked status",
			mutate: func(bli map[string]any) {
				bli[objects.FieldKeyStatus] = objects.ObjectStatusBlocked
			},
			ready:   false,
			missing: []string{"unresolved_blocker"},
		},
		{
			name: "AUTO-BLI bypass",
			mutate: func(bli map[string]any) {
				bli[objects.FieldKeyID] = "AUTO-BLI-PRI-1"
				delete(bli, objects.FieldKeyRequirementRefs)
				delete(bli, objects.FieldKeyCriteriaRefs)
				delete(bli, objects.FieldKeyEstimatedEffort)
				delete(bli, objects.FieldKeyPersonaRefs)
			},
			ready: true,
		},
		{
			name:    "nil object",
			mutate:  nil,
			ready:   false,
			missing: []string{"object"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bli map[string]any
			if tt.mutate != nil {
				bli = readyBLI()
				tt.mutate(bli)
			}
			got := Evaluate(bli)
			if got.Ready != tt.ready {
				t.Fatalf("ready=%v want %v missing=%v", got.Ready, tt.ready, got.Missing)
			}
			for _, want := range tt.missing {
				if !slices.Contains(got.Missing, want) {
					t.Fatalf("missing=%v want contains %q", got.Missing, want)
				}
			}
		})
	}
}

func TestRequiresPromoteGate(t *testing.T) {
	t.Parallel()
	if !RequiresPromoteGate(objects.ObjectStatusExploring, objects.ObjectStatusPlanned) {
		t.Fatal("exploring→planned must gate")
	}
	if !RequiresPromoteGate(objects.ObjectStatusPlanned, objects.ObjectStatusInProgress) {
		t.Fatal("planned→in_progress must gate")
	}
	if RequiresPromoteGate(objects.ObjectStatusInProgress, objects.ObjectStatusPlanned) {
		t.Fatal("in_progress→planned must not gate (demote recovery)")
	}
	if RequiresPromoteGate(objects.ObjectStatusExploring, objects.ObjectStatusValidated) {
		t.Fatal("exploring→validated must not gate")
	}
}
