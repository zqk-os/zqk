package validation

import (
	"os"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// TestFailClosedPreconditions_FunctionalAcceptance covers CRIT-1789669261063570000-7f136b3c:
// Functional acceptance of Overlay DSL compilation and deterministic fail-closed evaluation.
func TestFailClosedPreconditions_FunctionalAcceptance(t *testing.T) {
	gv := NewGoValidator()

	t.Run("StandardChecksPass_ValidID", func(t *testing.T) {
		obj := map[string]any{objects.FieldKeyID: "BLI-TEST-001"}
		met, recognized := gv.evaluatePrecondition("Standard checks pass", obj, nil)
		if !recognized {
			t.Fatal("expected 'Standard checks pass' to be recognized by Overlay DSL")
		}
		if !met {
			t.Fatal("expected 'Standard checks pass' to be met for object with valid ID")
		}
	})

	t.Run("StandardChecksPass_EmptyID", func(t *testing.T) {
		obj := map[string]any{objects.FieldKeyID: ""}
		met, recognized := gv.evaluatePrecondition("Standard checks pass", obj, nil)
		if !recognized {
			t.Fatal("expected 'Standard checks pass' to be recognized by Overlay DSL")
		}
		if met {
			t.Fatal("expected 'Standard checks pass' to fail when ID is empty")
		}
	})

	t.Run("DocEntryMetadata_Populated", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyTitle:   "Architecture Overview",
			objects.FieldKeySummary: "High-level design document",
			objects.FieldKeyPath:    "docs/architecture/overview.md",
		}
		met, recognized := gv.evaluatePrecondition("Title, summary, and path are populated", obj, nil)
		if !recognized {
			t.Fatal("expected 'Title, summary, and path are populated' to be recognized")
		}
		if !met {
			t.Fatal("expected metadata to be met when all three fields are populated")
		}
	})

	t.Run("DocEntryMetadata_MissingField", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyTitle:   "Architecture Overview",
			objects.FieldKeySummary: "",
			objects.FieldKeyPath:    "docs/architecture/overview.md",
		}
		met, recognized := gv.evaluatePrecondition("Title, summary, and path are populated", obj, nil)
		if !recognized {
			t.Fatal("expected 'Title, summary, and path are populated' to be recognized")
		}
		if met {
			t.Fatal("expected metadata check to fail when summary is empty")
		}
	})

	t.Run("ContentHash_Sealed", func(t *testing.T) {
		obj := map[string]any{"content_hash": "a1b2c3d4e5f6"}
		met, recognized := gv.evaluatePrecondition("Cryptographic content_hash computed and sealed", obj, nil)
		if !recognized {
			t.Fatal("expected content_hash check to be recognized")
		}
		if !met {
			t.Fatal("expected content_hash to be met when non-empty")
		}
	})

	t.Run("ContentSize_Measured", func(t *testing.T) {
		obj := map[string]any{"content_size": 1024}
		met, recognized := gv.evaluatePrecondition("Document content_size measured", obj, nil)
		if !recognized {
			t.Fatal("expected content_size check to be recognized")
		}
		if !met {
			t.Fatal("expected content_size to be met when > 0")
		}

		zeroObj := map[string]any{"content_size": 0}
		met, _ = gv.evaluatePrecondition("Document content_size measured", zeroObj, nil)
		if met {
			t.Fatal("expected content_size=0 to fail")
		}
	})

	t.Run("UnrecognizedPrecondition_FailsClosedByDefault", func(t *testing.T) {
		met, recognized := gv.evaluatePrecondition("some arbitrary unknown check", map[string]any{}, nil)
		if recognized {
			t.Fatal("unknown string must not be recognized")
		}
		if met {
			t.Fatal("unknown string must fail closed (met=false)")
		}
	})
}

// TestFailClosedPreconditions_BoundaryAndErrorHandling covers CRIT-1789669261063571000-464b2255:
// Boundary condition validation, negative testing, invalid input rejection, and fail-open opt-out.
func TestFailClosedPreconditions_BoundaryAndErrorHandling(t *testing.T) {
	gv := NewGoValidator()

	t.Run("TypoedPrecondition_FailsClosed", func(t *testing.T) {
		typos := []string{
			"at lest one ready backlog_item references this plan",
			"all linked critria_refs are validated or complete",
			"comit_hashes have git mutation evidence for this backlog_item",
			"standard checks pas",
			"priority plan validatd",
		}
		for _, typo := range typos {
			met, recognized := gv.evaluatePrecondition(typo, map[string]any{}, nil)
			if recognized {
				t.Fatalf("typo %q must not be recognized", typo)
			}
			if met {
				t.Fatalf("typo %q must fail closed (met=false)", typo)
			}
		}
	})

	t.Run("EmptyPrecondition_SafeHandling", func(t *testing.T) {
		met, recognized := gv.evaluatePrecondition("", map[string]any{}, nil)
		if !met {
			t.Fatal("empty precondition must return met=true")
		}
		if recognized {
			t.Fatal("empty precondition must return recognized=false")
		}

		whitespaceMet, _ := gv.evaluatePrecondition("   ", map[string]any{}, nil)
		if !whitespaceMet {
			t.Fatal("whitespace precondition must return met=true")
		}
	})

	t.Run("NilObjectAndOptions_NoPanic", func(t *testing.T) {
		met, recognized := gv.evaluatePrecondition("Standard checks pass", nil, nil)
		if !recognized {
			t.Fatal("expected 'Standard checks pass' to be recognized")
		}
		if met {
			t.Fatal("expected nil object to not satisfy 'Standard checks pass'")
		}
	})

	t.Run("BreakGlass_FailOpenEnvVar", func(t *testing.T) {
		orig := os.Getenv("ZQK_PRECONDITIONS_FAIL_OPEN")
		defer os.Setenv("ZQK_PRECONDITIONS_FAIL_OPEN", orig)

		os.Setenv("ZQK_PRECONDITIONS_FAIL_OPEN", "1")
		met, recognized := gv.evaluatePrecondition("some unknown legacy precondition", map[string]any{}, nil)
		if recognized {
			t.Fatal("unknown string must not be recognized even with fail-open enabled")
		}
		if !met {
			t.Fatal("expected fail-open when ZQK_PRECONDITIONS_FAIL_OPEN=1 is set")
		}
	})
}

// TestFailClosedPreconditions_IntegrationAndConformance covers CRIT-1789669261063572000-7e56df0f:
// Integration with GoValidator lifecycle state validation and pipeline execution contexts.
func TestFailClosedPreconditions_IntegrationAndConformance(t *testing.T) {
	gv := NewGoValidator()

	t.Run("PipelineContextOutcomeKeys_ReflectsFailClosed", func(t *testing.T) {
		pctx, met := gv.runLifecyclePreconditionPipeline("unknown lifecycle gate phrase", map[string]any{}, nil, nil)
		if met {
			t.Fatal("expected met=false for unknown precondition pipeline execution")
		}
		if pctx.Outcome[pipeline.OutcomeKeyLifecycleOk] != false {
			t.Fatalf("expected OutcomeKeyLifecycleOk=false, got %v", pctx.Outcome[pipeline.OutcomeKeyLifecycleOk])
		}
		if pctx.Outcome[pipeline.OutcomeKeyValidationSuccess] != false {
			t.Fatalf("expected OutcomeKeyValidationSuccess=false, got %v", pctx.Outcome[pipeline.OutcomeKeyValidationSuccess])
		}
	})

	t.Run("PipelineContextOutcomeKeys_ReflectsSuccess", func(t *testing.T) {
		obj := map[string]any{objects.FieldKeyID: "BLI-12345"}
		pctx, met := gv.runLifecyclePreconditionPipeline("Standard checks pass", obj, nil, nil)
		if !met {
			t.Fatal("expected met=true for valid standard checks pass")
		}
		if pctx.Outcome[pipeline.OutcomeKeyLifecycleOk] != true {
			t.Fatalf("expected OutcomeKeyLifecycleOk=true, got %v", pctx.Outcome[pipeline.OutcomeKeyLifecycleOk])
		}
		if pctx.Outcome[pipeline.OutcomeKeyValidationSuccess] != true {
			t.Fatalf("expected OutcomeKeyValidationSuccess=true, got %v", pctx.Outcome[pipeline.OutcomeKeyValidationSuccess])
		}
	})
}
