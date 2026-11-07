package eval_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/eval"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

func TestASTCompliance(t *testing.T) {
	auditor := qa.NewASTAuditor()
	files := []string{"engine.go", "template.go", "types.go", "../../cmd/zqk/eval/eval.go"}
	for _, f := range files {
		v, err := auditor.AuditFile(f)
		require.NoError(t, err)
		for _, item := range v {
			if item.Severity == "high" || item.Severity == "medium" {
				t.Errorf("[%s] %s:%d: %s (%s)", item.Severity, f, item.Pos.Line, item.Message, item.Type)
			}
		}
	}
}

func TestEvalTemplate_StaticInvariants(t *testing.T) {
	t.Parallel()

	// 1. Valid template passes
	valid := &eval.EvalTemplate{
		SchemaVersion: 1,
		TemplateID:    "EVAL-TEST-001",
		Title:         "Test Evaluation Template",
		Dimensions: map[string]eval.DimensionSpec{
			"HCODE":   {Weight: 0.5, Threshold: 4.0},
			"EFFPERF": {Weight: 0.5, Threshold: 3.5},
		},
	}
	require.NoError(t, eval.ValidateTemplate(valid))

	// 2. Missing template_id fails
	missingID := &eval.EvalTemplate{
		Title: "No ID",
	}
	require.Error(t, eval.ValidateTemplate(missingID))
	assert.Contains(t, eval.ValidateTemplate(missingID).Error(), "template_id is required")

	// 3. Missing title fails
	missingTitle := &eval.EvalTemplate{
		TemplateID: "EVAL-TEST-002",
	}
	require.Error(t, eval.ValidateTemplate(missingTitle))
	assert.Contains(t, eval.ValidateTemplate(missingTitle).Error(), "title is required")

	// 4. Weights not summing to 1.0 fails
	badWeights := &eval.EvalTemplate{
		TemplateID: "EVAL-TEST-003",
		Title:      "Bad Weights",
		Dimensions: map[string]eval.DimensionSpec{
			"HCODE":   {Weight: 0.3, Threshold: 4.0},
			"EFFPERF": {Weight: 0.3, Threshold: 3.5},
		},
	}
	require.Error(t, eval.ValidateTemplate(badWeights))
	assert.Contains(t, eval.ValidateTemplate(badWeights).Error(), "dimension weights must sum to 1.0")

	// 5. Threshold out of range fails
	badThreshold := &eval.EvalTemplate{
		TemplateID: "EVAL-TEST-004",
		Title:      "Bad Threshold",
		Dimensions: map[string]eval.DimensionSpec{
			"HCODE": {Weight: 1.0, Threshold: 5.5},
		},
	}
	require.Error(t, eval.ValidateTemplate(badThreshold))
	assert.Contains(t, eval.ValidateTemplate(badThreshold).Error(), "must be in range [1.0, 5.0]")
}

func TestEvalEngine_DynamicProofAndBoundary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Clean file without raw os calls
	cleanPkg := filepath.Join(tmpDir, "pkg", "clean")
	require.NoError(t, fileutil.MkdirAll(cleanPkg, paths.DirPerm755))
	cleanFile := filepath.Join(cleanPkg, "clean.go")
	cleanSrc := `package clean

func Process() string {
	return "ok"
}
`
	require.NoError(t, fileutil.WriteFile(cleanFile, []byte(cleanSrc), paths.FilePerm644))

	// Violating file with raw os call
	dirtyPkg := filepath.Join(tmpDir, "pkg", "dirty")
	require.NoError(t, fileutil.MkdirAll(dirtyPkg, paths.DirPerm755))
	dirtyFile := filepath.Join(dirtyPkg, "dirty.go")
	dirtySrc := `package dirty

import "os"

func Run() {
	os.Exit(1)
}
`
	require.NoError(t, fileutil.WriteFile(dirtyFile, []byte(dirtySrc), paths.FilePerm644))

	engine := eval.NewEngine(tmpDir)
	tmpl := &eval.EvalTemplate{
		TemplateID: "EVAL-GO-HYGIENE-V1",
		Title:      "Go Code Hygiene",
		Dimensions: map[string]eval.DimensionSpec{
			"HCODE": {Weight: 1.0, Threshold: 4.5},
		},
		Remediation: eval.RemediationSpec{
			AutoMint: true,
			Persona:  "PER-COMMUNITY-SOFTWARE-ENGINEER",
		},
	}

	// 1. Clean file passes evaluation
	cleanResult, err := engine.Execute(context.Background(), tmpl, filepath.Join("pkg", "clean", "clean.go"))
	require.NoError(t, err)
	assert.True(t, cleanResult.Passed)
	assert.Equal(t, 5.0, cleanResult.CompositeScore)
	assert.Empty(t, cleanResult.Violations)

	// 2. Dirty file fails evaluation with POL-CODE-006 blocker
	dirtyResult, err := engine.Execute(context.Background(), tmpl, filepath.Join("pkg", "dirty", "dirty.go"))
	require.NoError(t, err)
	assert.False(t, dirtyResult.Passed)
	assert.Less(t, dirtyResult.CompositeScore, 5.0)
	assert.NotEmpty(t, dirtyResult.Violations)
	assert.Equal(t, "POL-CODE-006", dirtyResult.Violations[0].Rule)
	assert.NotEmpty(t, dirtyResult.Remediations)

	// 3. Negative boundary: non-existent target fails gracefully
	_, err = engine.Execute(context.Background(), tmpl, filepath.Join("pkg", "nonexistent"))
	require.Error(t, err)
}

func TestLoadTemplate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid_tmpl.yaml")
	validYAML := `schema_version: 1
kind: eval_template
template_id: EVAL-CANONICAL-001
title: Canonical Go Evaluation
dimensions:
  HCODE:
    weight: 1.0
    threshold: 4.0
`
	require.NoError(t, fileutil.WriteFile(validPath, []byte(validYAML), paths.FilePerm644))

	tmpl, err := eval.LoadTemplate(validPath)
	require.NoError(t, err)
	assert.Equal(t, "EVAL-CANONICAL-001", tmpl.TemplateID)

	// Missing file
	_, err = eval.LoadTemplate(filepath.Join(tmpDir, "missing.yaml"))
	require.Error(t, err)

	// Empty path
	_, err = eval.LoadTemplate("")
	require.Error(t, err)

	// Malformed YAML
	badPath := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badPath, []byte("invalid: [unclosed"), paths.FilePerm644))
	_, err = eval.LoadTemplate(badPath)
	require.Error(t, err)
}
