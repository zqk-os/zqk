package validation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// TRACK: BLI-1786686769839541000-f5a3260f — draft→active fail-closed on the real lifecycle YAML.
func TestGoValidator_CVSDraftToActiveAdmission(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	gv := NewGoValidatorWithLoaders(nil, objects.NewLifecycleLoader(lifecyclesDir))
	ctx := context.Background()
	kind := objects.KindConvergenceSession

	hollow := map[string]any{
		objects.FieldKeyKind:   kind,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	errs, _ := gv.validateLifecycleState(ctx, kind, objects.ObjectStatusActive, objects.ObjectStatusDraft, hollow, nil)
	if len(errs) == 0 {
		t.Fatal("hollow draft→active must fail closed")
	}

	phaseOnly := map[string]any{
		objects.FieldKeyKind:         kind,
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyCurrentPhase: "c1_scope",
	}
	errs, _ = gv.validateLifecycleState(ctx, kind, objects.ObjectStatusActive, objects.ObjectStatusDraft, phaseOnly, nil)
	if len(errs) == 0 {
		t.Fatal("draft→active with only current_phase must fail closed")
	}

	admitted := map[string]any{
		objects.FieldKeyKind:             kind,
		objects.FieldKeyStatus:           objects.ObjectStatusActive,
		objects.FieldKeyHypothesis:       "hollow sessions must not enter CAP",
		objects.FieldKeyDesiredEndState:  "draft→active requires hypothesis, desired_end_state, current_phase",
		objects.FieldKeyCurrentPhase:     "c1_scope",
		objects.FieldKeyOutcomeCharacter: "pending",
	}
	errs, warnings := gv.validateLifecycleState(ctx, kind, objects.ObjectStatusActive, objects.ObjectStatusDraft, admitted, nil)
	if len(errs) != 0 {
		t.Fatalf("minimal contract should admit: errors=%v warnings=%v", errs, warnings)
	}
}
