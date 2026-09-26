package supervision

import (
	"strings"
	"testing"
)

func TestSupervisionTypesAndDefaults(t *testing.T) {
	policy := DefaultSupervisionPolicy()
	if policy.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts=3, got %d", policy.MaxAttempts)
	}
	if !policy.EnableHypothesisPrune {
		t.Errorf("expected EnableHypothesisPrune=true")
	}
	if !policy.EnableExemplarInject {
		t.Errorf("expected EnableExemplarInject=true")
	}

	diag := BuildFailureDiagnostic(1, 1, PhaseCompile, "main.go:42:10: undefined: Foo", "", "Undefined symbol")
	if diag.Attempt != 1 {
		t.Errorf("expected Attempt=1, got %d", diag.Attempt)
	}
	if diag.ExitCode != 1 {
		t.Errorf("expected ExitCode=1, got %d", diag.ExitCode)
	}
	if diag.Phase != PhaseCompile {
		t.Errorf("expected Phase=compile, got %s", diag.Phase)
	}
	if diag.FailureAnchor != "main.go:42:10" {
		t.Errorf("expected FailureAnchor=main.go:42:10, got %s", diag.FailureAnchor)
	}
	if !strings.Contains(diag.FocusedDiagnostic, "undefined: Foo") {
		t.Errorf("focused diagnostic should contain undefined: Foo, got %s", diag.FocusedDiagnostic)
	}
}

func TestNegativeHypothesisSynthesis(t *testing.T) {
	diag := &FailureDiagnostic{
		Attempt:       1,
		ExitCode:      2,
		Phase:         PhaseVerification,
		FailureAnchor: "pkg/service/auth.go:88",
	}

	hyp := SynthesizeNegativeHypothesis(1, diag, "Attempted to use insecure token without validation")
	if hyp.Attempt != 1 {
		t.Errorf("expected attempt=1, got %d", hyp.Attempt)
	}
	if !strings.Contains(hyp.InvalidatedApproach, "insecure token") {
		t.Errorf("expected invalidated approach to be preserved, got %s", hyp.InvalidatedApproach)
	}
	if len(hyp.ForbiddenActions) == 0 {
		t.Errorf("expected non-empty forbidden actions")
	}

	// Auto-synthesized hypothesis when approach is empty
	autoHyp := SynthesizeNegativeHypothesis(2, diag, "")
	if !strings.Contains(autoHyp.InvalidatedApproach, "pkg/service/auth.go:88") {
		t.Errorf("expected auto hypothesis to mention failure anchor, got %s", autoHyp.InvalidatedApproach)
	}
}

func TestExemplarRegistryResolution(t *testing.T) {
	reg := DefaultExemplarRegistry()
	exemplars := reg.Resolve("testing", "error_handling")

	if len(exemplars) < 2 {
		t.Fatalf("expected at least 2 exemplars for testing and error_handling, got %d", len(exemplars))
	}

	foundTest := false
	foundErr := false
	for _, ex := range exemplars {
		if ex.Category == "testing" {
			foundTest = true
		}
		if ex.Category == "error_handling" {
			foundErr = true
		}
	}

	if !foundTest || !foundErr {
		t.Errorf("expected both testing and error_handling exemplars, got test=%v, err=%v", foundTest, foundErr)
	}

	// Custom registration
	customReg := NewExemplarRegistry()
	customReg.Register("custom_category", CanonicalExemplar{
		Category:    "custom_category",
		Title:       "Custom Pattern",
		PatternCode: "foo()",
	})
	resolved := customReg.Resolve("custom_category")
	if len(resolved) != 1 || resolved[0].Title != "Custom Pattern" {
		t.Errorf("custom exemplar registration failed: %+v", resolved)
	}
}
