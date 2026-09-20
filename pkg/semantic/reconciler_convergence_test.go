package semantic

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSemanticReconciler_CheckConvergenceDrift(t *testing.T) {
	store := &mockStorage{}
	reconciler := NewSemanticReconciler(store)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Setup a failing convergence session
	failingSession := map[string]any{
		objects.FieldKeyID:              "CVS-FAIL-001",
		objects.FieldKeyKind:            objects.KindConvergenceSession,
		objects.FieldKeyTitle:           "Failing Test Bundle",
		objects.FieldKeyStatus:          "active",
		objects.FieldKeyDeltaAssessment: "trending_away",
	}
	_ = store.Create(ctx, secCtx, failingSession)

	// 2. Setup a passing convergence session
	passingSession := map[string]any{
		objects.FieldKeyID:              "CVS-PASS-001",
		objects.FieldKeyKind:            objects.KindConvergenceSession,
		objects.FieldKeyTitle:           "Passing Test Bundle",
		objects.FieldKeyStatus:          "active",
		objects.FieldKeyDeltaAssessment: "trending_toward",
	}
	_ = store.Create(ctx, secCtx, passingSession)

	// 3. Run reconciliation
	results, err := reconciler.CheckConvergenceDrift(ctx, secCtx)
	if err != nil {
		t.Fatalf("CheckConvergenceDrift failed: %v", err)
	}

	// 4. Verify
	foundFailing := false
	for _, res := range results {
		if res.SessionID == "CVS-FAIL-001" {
			if !res.TrendingAway {
				t.Errorf("expected CVS-FAIL-001 to be trending away")
			}
			foundFailing = true
		}
	}
	if !foundFailing {
		t.Errorf("did not find failing session CVS-FAIL-001")
	}

	// 5. Test Inference
	engine := NewInferenceEngine(store)
	inferences := engine.InferFromConvergence(results)

	foundInference := false
	for _, inf := range inferences {
		if inf[objects.FieldKeyTitle] == "Remediate Failing Test Bundle" {
			foundInference = true
			if inf[objects.FieldKeyPriorityTier] != "P1" {
				t.Errorf("expected P1 priority for failing session inference")
			}
		}
	}
	if !foundInference {
		t.Errorf("did not generate inference for failing session")
	}
}
