package eval

import (
	"context"
	"testing"
	"time"
)

func TestRemediationDispatcher_ThresholdFailure(t *testing.T) {
	dispatcher := NewRemediationDispatcher(nil)

	tmpl := &EvalTemplate{
		TemplateID: "ast-hygiene-v1",
		Remediation: RemediationSpec{
			AutoMint:               true,
			Persona:                "PER-COMMUNITY-SOFTWARE-ENGINEER",
			Priority:               "high",
			LinkConvergenceSession: true,
		},
	}

	result := &EvalResult{
		TemplateID:     "ast-hygiene-v1",
		Target:         "pkg/eval/dummy.go",
		AssessedAt:     time.Now(),
		Passed:         false,
		CompositeScore: 45.0,
		Violations: []Violation{
			{
				File:     "pkg/eval/dummy.go",
				Rule:     "POL-CODE-006",
				Severity: "blocker",
				Message:  "Direct standard library os call detected",
			},
		},
	}

	req := RemediationRequest{
		Template:         tmpl,
		Result:           result,
		SessionID:        "CVS-TEST-001",
		MetricTemplateID: "MTR-AST-HYGIENE",
	}

	receipt, err := dispatcher.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful dispatch, got error: %v", err)
	}

	if receipt == nil {
		t.Fatal("expected non-nil remediation receipt")
	}

	if receipt.MetricTemplateID != "MTR-AST-HYGIENE" {
		t.Errorf("expected MetricTemplateID MTR-AST-HYGIENE, got %s", receipt.MetricTemplateID)
	}

	if receipt.RemediationID == "" {
		t.Error("expected non-empty RemediationID")
	}

	if receipt.Target != "pkg/eval/dummy.go" {
		t.Errorf("expected target pkg/eval/dummy.go, got %s", receipt.Target)
	}
}

func TestMatrixProfile_Persistence(t *testing.T) {
	manager := NewMatrixProfileManager()

	profile := &MatrixProfile{
		Name:        "nightly-quality-gate",
		Description: "Nightly multi-model scenario evaluation matrix",
		SessionID:   "CVS-NIGHTLY-001",
		Templates:   []string{"ast-hygiene-v1", "test-coverage-v1"},
		Models:      []string{"claude-3-5-sonnet", "gpt-4o"},
		Targets:     []string{"pkg/eval/...", "cmd/zqk/..."},
		CreatedAt:   time.Now(),
	}

	err := manager.SaveProfile(profile)
	if err != nil {
		t.Fatalf("failed to save matrix profile: %v", err)
	}

	loaded, err := manager.GetProfile("nightly-quality-gate")
	if err != nil {
		t.Fatalf("failed to get matrix profile: %v", err)
	}

	if loaded.Name != "nightly-quality-gate" {
		t.Errorf("expected profile name nightly-quality-gate, got %s", loaded.Name)
	}
	if len(loaded.Templates) != 2 {
		t.Errorf("expected 2 templates, got %d", len(loaded.Templates))
	}
	if len(loaded.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(loaded.Models))
	}
}

func TestRemediationDispatcher_NegativeBoundaries(t *testing.T) {
	dispatcher := NewRemediationDispatcher(nil)

	// Negative boundary: empty template ID should fail
	reqEmpty := RemediationRequest{
		Template: nil,
		Result:   &EvalResult{},
	}
	_, err := dispatcher.Dispatch(context.Background(), reqEmpty)
	if err == nil {
		t.Fatal("expected error on nil template, got nil")
	}

	manager := NewMatrixProfileManager()
	// Negative boundary: empty profile name should fail
	err = manager.SaveProfile(&MatrixProfile{Name: ""})
	if err == nil {
		t.Fatal("expected error on empty profile name, got nil")
	}

	// Negative boundary: non-existent profile retrieval
	_, err = manager.GetProfile("non-existent-profile")
	if err == nil {
		t.Fatal("expected error on non-existent profile, got nil")
	}
}
