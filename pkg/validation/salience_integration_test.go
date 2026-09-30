package validation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/predicate"
)

func TestEvalOverlayDSLStage_TitleBodyCohesion(t *testing.T) {
	gv := NewGoValidator()
	options := &ValidationOptions{}

	// 1. Cohesive title and description passes
	objCohesive := map[string]any{
		objects.FieldKeyID:          "REQ-TEST-001",
		objects.FieldKeyTitle:       "StateLocker distributed fencing token validation",
		objects.FieldKeyDescription: "Ensure StateLocker validates monotonic fencing tokens to prevent concurrent split-brain execution.",
	}
	handled, met := evalOverlayDSLStage(gv, "title_body_cohesion", objCohesive, options)
	if !handled || !met {
		t.Errorf("expected cohesive object to pass title_body_cohesion, handled=%v, met=%v", handled, met)
	}

	// 2. Cohesive with custom min_stems argument
	handled, met = evalOverlayDSLStage(gv, "title_body_cohesion:2", objCohesive, options)
	if !handled || !met {
		t.Errorf("expected cohesive object with min_stems=2 to pass, handled=%v, met=%v", handled, met)
	}

	// 3. Vacuous disconnected title fails
	objVacuous := map[string]any{
		objects.FieldKeyID:          "REQ-TEST-002",
		objects.FieldKeyTitle:       "Update general system configuration",
		objects.FieldKeyDescription: "Ensure StateLocker validates monotonic fencing tokens to prevent concurrent split-brain execution.",
	}
	handled, met = evalOverlayDSLStage(gv, "title_body_cohesion", objVacuous, options)
	if !handled || met {
		t.Errorf("expected vacuous object to fail title_body_cohesion, handled=%v, met=%v", handled, met)
	}
}

func TestPredicate_TitleBodySemanticProof(t *testing.T) {
	title := "ReverseReferenceIndex lock contention"
	body := "Under high load, concurrent object deletions trigger a linear map scan and slice reallocation while holding write lock."

	ok, shared := predicate.VerifyTitleBodyCohesion(title, body, 1)
	if !ok {
		t.Fatalf("expected cohesion, got false, shared: %v", shared)
	}
	if len(shared) < 1 {
		t.Fatalf("expected at least 1 shared stem, got %v", shared)
	}
}
