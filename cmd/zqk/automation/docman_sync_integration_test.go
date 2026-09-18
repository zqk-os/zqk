package automation

import (
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

// TestDocmanSyncCmd_Integration tests the actual command execution
func TestDocmanSyncCmd_Integration(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup temporary project root
	tmpDir := t.TempDir()
	projectRoot := setupTestProject(t, tmpDir)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "integration-test.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Integration Test Document

## Overview

This is a test document for integration testing of docman-sync command.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Change to project root
	oldDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(oldDir)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to change to project root: %v", err)
	}

	// Build the CLI binary (if not already built)
	cliBin := filepath.Join(projectRoot, paths.CLICommandName)
	if _, err := fileutil.Stat(cliBin); fileutil.IsNotExist(err) {
		// Try to find it in the original directory
		originalBin := filepath.Join(oldDir, paths.CLICommandName)
		if _, err := fileutil.Stat(originalBin); err == nil {
			cliBin = originalBin
		} else {
			t.Skipf("%s binary not found, skipping integration test", paths.CLICommandName)
		}
	}

	// Run the command
	cmd := execwrap.Command(cliBin, "automation", "docman-sync", "--context", automationProfileAIAgent)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, output)
	}

	// Verify output contains expected messages
	outputStr := string(output)
	if !contains(outputStr, "Created") && !contains(outputStr, "Skipped") {
		t.Errorf("expected output to contain 'Created' or 'Skipped', got: %s", outputStr)
	}
}

// TestDocmanSyncCmd_DryRun tests the dry-run mode
func TestDocmanSyncCmd_DryRun(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup temporary project root
	tmpDir := t.TempDir()
	projectRoot := setupTestProject(t, tmpDir)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "dry-run-test.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Dry Run Test Document

## Overview

This is a test document for dry-run testing.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Change to project root
	oldDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(oldDir)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to change to project root: %v", err)
	}

	// Build the CLI binary (if not already built)
	cliBin := filepath.Join(projectRoot, paths.CLICommandName)
	if _, err := fileutil.Stat(cliBin); fileutil.IsNotExist(err) {
		// Try to find it in the original directory
		originalBin := filepath.Join(oldDir, paths.CLICommandName)
		if _, err := fileutil.Stat(originalBin); err == nil {
			cliBin = originalBin
		} else {
			t.Skipf("%s binary not found, skipping integration test", paths.CLICommandName)
		}
	}

	// Run the command with dry-run
	cmd := execwrap.Command(cliBin, "automation", "docman-sync", "--dry-run", "--context", automationProfileAIAgent)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, output)
	}

	// Verify output contains "Would create"
	outputStr := string(output)
	if !contains(outputStr, "Would create") {
		t.Errorf("expected output to contain 'Would create', got: %s", outputStr)
	}
}

// TestDocmanSyncCmd_CheckOnly tests the check-only mode
func TestDocmanSyncCmd_CheckOnly(t *testing.T) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup temporary project root
	tmpDir := t.TempDir()
	projectRoot := setupTestProject(t, tmpDir)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "check-only-test.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Check Only Test Document

## Overview

This is a test document for check-only testing.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Change to project root
	oldDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(oldDir)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to change to project root: %v", err)
	}

	// Build the CLI binary (if not already built)
	cliBin := filepath.Join(projectRoot, paths.CLICommandName)
	if _, err := fileutil.Stat(cliBin); fileutil.IsNotExist(err) {
		// Try to find it in the original directory
		originalBin := filepath.Join(oldDir, paths.CLICommandName)
		if _, err := fileutil.Stat(originalBin); err == nil {
			cliBin = originalBin
		} else {
			t.Skipf("%s binary not found, skipping integration test", paths.CLICommandName)
		}
	}

	// Run the command with check-only (should exit with code 1 if registration needed)
	cmd := execwrap.Command(cliBin, "automation", "docman-sync", "--check-only", "--context", automationProfileAIAgent)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	err = cmd.Run()

	// Should exit with code 1 if registration is needed
	if err == nil {
		t.Error("expected command to exit with error code 1 when registration is needed")
	}

	// Verify exit code is 1
	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() != 1 {
			t.Errorf("expected exit code 1, got %d", exitError.ExitCode())
		}
	} else {
		t.Errorf("expected ExitError, got %v", err)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == emptyValue ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
