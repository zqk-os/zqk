package ci

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestAdoptionSprint3_CIAuditWorkflow verifies CRIT-1793499995819907000-6c5d9378:
// Valid GitHub Actions workflow scaffolding and Homebrew formula template.
func TestAdoptionSprint3_CIAuditWorkflow(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	res, err := InitCI(ctx, InitOptions{
		RepoRoot: tempDir,
		DryRun:   false,
		Force:    true,
	})
	if err != nil {
		t.Fatalf("InitCI failed: %v", err)
	}

	if !fileutil.Exists(res.WorkflowPath) {
		t.Fatalf("expected workflow file at %s, but does not exist", res.WorkflowPath)
	}
	if !fileutil.Exists(res.FormulaPath) {
		t.Fatalf("expected formula file at %s, but does not exist", res.FormulaPath)
	}

	// Verify valid YAML
	workflowBytes, err := fileutil.ReadFile(res.WorkflowPath)
	if err != nil {
		t.Fatalf("failed to read workflow file: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(workflowBytes, &parsed); err != nil {
		t.Fatalf("workflow file is not valid YAML: %v", err)
	}

	if parsed["name"] == nil || !strings.Contains(parsed["name"].(string), "Guardrail") {
		t.Fatalf("workflow name unexpected: %v", parsed["name"])
	}

	// Verify formula contents
	formulaBytes, err := fileutil.ReadFile(res.FormulaPath)
	if err != nil {
		t.Fatalf("failed to read formula file: %v", err)
	}
	if !strings.Contains(string(formulaBytes), "class Zqk < Formula") {
		t.Fatalf("formula does not contain expected class definition")
	}
}

// TestAdoptionSprint3_CISwarmRunner verifies CRIT-1793499995819908000-1986fc16:
// Dry-run execution and one-click swarm runner dispatch with token budgeting and AST evidence.
func TestAdoptionSprint3_CISwarmRunner(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. Verify ci init --dry-run
	res, err := InitCI(ctx, InitOptions{
		RepoRoot: tempDir,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("InitCI dry-run failed: %v", err)
	}
	if !res.DryRun {
		t.Errorf("expected DryRun=true")
	}
	if fileutil.Exists(res.WorkflowPath) {
		t.Errorf("dry-run should not write workflow to disk")
	}

	// 2. Verify swarm runner dispatch
	swarmRes, err := DispatchSwarmRunner(ctx, SwarmRunnerOptions{
		RepoRoot:    tempDir,
		TokenBudget: 60000,
		MaxParallel: 3,
		Entrypoint:  "auto-pilot",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("DispatchSwarmRunner failed: %v", err)
	}

	if !swarmRes.ASTVerified {
		t.Errorf("expected ASTVerified=true")
	}
	if swarmRes.AllocatedBudgetPerWorker != 20000 {
		t.Errorf("expected allocated budget 20000, got %d", swarmRes.AllocatedBudgetPerWorker)
	}
	if !strings.Contains(swarmRes.ExecutionEvidence, "AST_INVARIANTS=VALIDATED") {
		t.Errorf("expected AST evidence in execution receipts")
	}
}

// TestAdoptionSprint3_CIBoundary verifies CRIT-1793499995819909000-a96fb9c9:
// Negative adversarial boundaries: symlinks pointing outside root, path traversal, and non-positive budgets.
func TestAdoptionSprint3_CIBoundary(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	outsideDir := t.TempDir()

	// 1. Path traversal outside repo root rejection
	_, err := InitCI(ctx, InitOptions{
		RepoRoot:     tempDir,
		WorkflowPath: "../../../escaped-workflow.yml",
	})
	if err == nil {
		t.Fatalf("expected error on path traversal outside repo root, got nil")
	}
	if !strings.Contains(err.Error(), "escapes repository root") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 2. Symlink pointing outside repo root rejection
	outsideTarget := filepath.Join(outsideDir, "malicious.yml")
	if err := fileutil.WriteFile(outsideTarget, []byte("evil: true"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create outside target: %v", err)
	}

	symlinkPath := filepath.Join(tempDir, "symlinked-workflow.yml")
	if err := fileutil.Symlink(outsideTarget, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	_, err = InitCI(ctx, InitOptions{
		RepoRoot:     tempDir,
		WorkflowPath: symlinkPath,
	})
	if err == nil {
		t.Fatalf("expected error on external symlink target, got nil")
	}
	if !strings.Contains(err.Error(), "symlink pointing outside repository root") {
		t.Errorf("expected external symlink error, got: %v", err)
	}

	// 3. Non-positive or missing token budget rejection
	_, err = DispatchSwarmRunner(ctx, SwarmRunnerOptions{
		RepoRoot:    tempDir,
		TokenBudget: 0,
	})
	if err == nil {
		t.Fatalf("expected error on zero token budget, got nil")
	}
	if !strings.Contains(err.Error(), "missing or non-positive token budget") {
		t.Errorf("expected non-positive token budget error, got: %v", err)
	}

	_, err = DispatchSwarmRunner(ctx, SwarmRunnerOptions{
		RepoRoot:    tempDir,
		TokenBudget: -500,
	})
	if err == nil {
		t.Fatalf("expected error on negative token budget, got nil")
	}
	if !strings.Contains(err.Error(), "missing or non-positive token budget") {
		t.Errorf("expected negative token budget error, got: %v", err)
	}

	// 4. Zero-overwrite protection without force flag
	if _, err := InitCI(ctx, InitOptions{RepoRoot: tempDir, Force: true}); err != nil {
		t.Fatalf("setup InitCI failed: %v", err)
	}
	_, err = InitCI(ctx, InitOptions{RepoRoot: tempDir, Force: false})
	if err == nil {
		t.Fatalf("expected error overwriting without force flag, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected already exists error, got: %v", err)
	}
}
