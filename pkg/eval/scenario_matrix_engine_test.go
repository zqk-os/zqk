package eval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestScenarioMatrixEngine_ExecuteMatrix(t *testing.T) {
	// Setup temporary workspace
	tempDir := t.TempDir()

	// Create a dummy Go source file to evaluate
	goodSrc := `package sample

func GoodFunction() string {
	return "deterministic output"
}
`
	err := fileutil.WriteFile(filepath.Join(tempDir, "sample.go"), []byte(goodSrc), 0644)
	if err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	tmpl1 := &EvalTemplate{
		SchemaVersion: 1,
		TemplateID:    "EVAL-SCENARIO-001",
		Title:         "Sample Determinism Evaluation",
		Target: TargetSpec{
			Kind:     "file_glob",
			Patterns: []string{"*.go"},
		},
		Dimensions: map[string]DimensionSpec{
			"HCODE": {Weight: 1.0, Threshold: 3.0},
		},
	}

	tmpl2 := &EvalTemplate{
		SchemaVersion: 1,
		TemplateID:    "EVAL-SCENARIO-002",
		Title:         "Sample Quality Evaluation",
		Target: TargetSpec{
			Kind:     "file_glob",
			Patterns: []string{"*.go"},
		},
		Dimensions: map[string]DimensionSpec{
			"HCODE": {Weight: 1.0, Threshold: 4.0},
		},
	}

	matrixEngine := NewMatrixEngine(tempDir)

	cfg := MatrixConfig{
		Templates: []*EvalTemplate{tmpl1, tmpl2},
		Models:    []string{"model-alpha", "model-beta"},
	}

	ctx := context.Background()
	result, err := matrixEngine.ExecuteMatrix(ctx, cfg)
	if err != nil {
		t.Fatalf("ExecuteMatrix failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil matrix result")
	}

	if result.TotalScenarios != 4 {
		t.Errorf("expected 4 total scenario executions (2 templates x 2 models), got %d", result.TotalScenarios)
	}

	if len(result.ModelConvergence) != 2 {
		t.Errorf("expected 2 models in ModelConvergence, got %d", len(result.ModelConvergence))
	}

	if _, ok := result.ModelConvergence["model-alpha"]; !ok {
		t.Error("expected model-alpha in convergence summary")
	}

	if _, ok := result.ModelConvergence["model-beta"]; !ok {
		t.Error("expected model-beta in convergence summary")
	}

	// Verify aggregate scoring calculation
	if result.AverageCompositeScore <= 0 {
		t.Errorf("expected positive average composite score, got %f", result.AverageCompositeScore)
	}
}

func TestScenarioMatrixEngine_EmptyConfig(t *testing.T) {
	tempDir := t.TempDir()
	matrixEngine := NewMatrixEngine(tempDir)

	_, err := matrixEngine.ExecuteMatrix(context.Background(), MatrixConfig{})
	if err == nil {
		t.Fatal("expected error on empty matrix config, got nil")
	}
}
