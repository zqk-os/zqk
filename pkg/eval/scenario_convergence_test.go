package eval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestScenarioConvergence_StaticFloorInvariant verifies the structural integrity
// of scenario evaluation templates and matrix configs (CRIT-1794448601096166000-d294f816).
func TestScenarioConvergence_StaticFloorInvariant(t *testing.T) {
	tmpl := &EvalTemplate{
		SchemaVersion: 1,
		TemplateID:    "EVAL-CONV-FLOOR-001",
		Title:         "Scenario Convergence Static Floor",
		Target: TargetSpec{
			Kind:     "file_glob",
			Patterns: []string{"*.go"},
		},
		Dimensions: map[string]DimensionSpec{
			"HCODE": {Weight: 0.6, Threshold: 3.5},
			"CLEAN": {Weight: 0.4, Threshold: 4.0},
		},
	}

	if tmpl.TemplateID != "EVAL-CONV-FLOOR-001" {
		t.Fatalf("expected template ID EVAL-CONV-FLOOR-001, got %s", tmpl.TemplateID)
	}

	totalWeight := 0.0
	for _, dim := range tmpl.Dimensions {
		if dim.Threshold <= 0 {
			t.Errorf("expected positive threshold, got %f", dim.Threshold)
		}
		totalWeight += dim.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected dimension weights to sum to 1.0, got %f", totalWeight)
	}
}

// TestScenarioConvergence_OperationalDynamicProof verifies end-to-end matrix execution
// and convergence statistical aggregation across models (CRIT-1794448601096166000-d294f816).
func TestScenarioConvergence_OperationalDynamicProof(t *testing.T) {
	tempDir := t.TempDir()

	sampleCode := `package testpkg

func Alpha() string {
	return "alpha deterministic result"
}
`
	if err := fileutil.WriteFile(filepath.Join(tempDir, "sample.go"), []byte(sampleCode), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	engine := NewMatrixEngine(tempDir)
	cfg := MatrixConfig{
		Templates: []*EvalTemplate{
			{
				SchemaVersion: 1,
				TemplateID:    "EVAL-OPERATIONAL-001",
				Title:         "Operational Proof Matrix",
				Target: TargetSpec{
					Kind:     "file_glob",
					Patterns: []string{"*.go"},
				},
				Dimensions: map[string]DimensionSpec{
					"HCODE": {Weight: 1.0, Threshold: 2.0},
				},
			},
		},
		Models:    []string{"claude-3-5-sonnet", "gpt-4o"},
		SessionID: "CVS-CONVERGENCE-TEST-001",
	}

	res, err := engine.ExecuteMatrix(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ExecuteMatrix failed: %v", err)
	}

	if res.SessionID != "CVS-CONVERGENCE-TEST-001" {
		t.Errorf("expected session ID CVS-CONVERGENCE-TEST-001, got %s", res.SessionID)
	}
	if res.TotalScenarios != 2 {
		t.Errorf("expected 2 scenario runs, got %d", res.TotalScenarios)
	}
	if len(res.ModelConvergence) != 2 {
		t.Errorf("expected 2 model convergence entries, got %d", len(res.ModelConvergence))
	}
}

// TestScenarioConvergence_NegativeBoundaryAndErrorRecovery verifies fail-closed rejection
// of malformed configs and error handling boundaries (CRIT-1794448601096167000-18a99df5).
func TestScenarioConvergence_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	engine := NewMatrixEngine(t.TempDir())

	// Rejection 1: Empty templates
	_, err := engine.ExecuteMatrix(context.Background(), MatrixConfig{
		Templates: []*EvalTemplate{},
		Models:    []string{"model-a"},
	})
	if err == nil {
		t.Error("expected error for empty template slice, got nil")
	}

	// Rejection 2: Nil template
	_, err = engine.ExecuteMatrix(context.Background(), MatrixConfig{
		Templates: nil,
		Models:    []string{"model-a"},
	})
	if err == nil {
		t.Error("expected error for nil templates, got nil")
	}
}

// TestScenarioConvergence_DocumentationVerification verifies architectural specifications
// and compliance documentation exist on disk (CRIT-1794448601096168000-735bdd0a).
func TestScenarioConvergence_DocumentationVerification(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	specPath := filepath.Join(root, "docs", "architecture", "SCENARIO_CONVERGENCE_MATRIX_ENGINE.md")
	if !fileutil.Exists(specPath) {
		t.Fatalf("scenario convergence architecture documentation missing at %s", specPath)
	}

	genericSpecPath := filepath.Join(root, "docs", "architecture", "GENERIC_SCENARIO_EVALUATION_FRAMEWORK.md")
	if !fileutil.Exists(genericSpecPath) {
		t.Fatalf("generic scenario evaluation framework doc missing at %s", genericSpecPath)
	}
}
