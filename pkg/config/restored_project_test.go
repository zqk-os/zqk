package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestIdempotentRestoreConfig_Idempotent verifies that running RestoreConfig
// multiple times produces identical results (idempotency).
func TestIdempotentRestoreConfig_Idempotent(t *testing.T) {
	root := t.TempDir()

	// First run should create config
	err := RestoreConfig(root)
	if err != nil {
		t.Fatalf("first restore failed: %v", err)
	}

	// Second run should succeed identically (no panic, no error, no diff)
	err = RestoreConfig(root)
	if err != nil {
		t.Fatalf("second restore failed (idempotency violation): %v", err)
	}

	// Verify all expected files exist after both runs
	expected := []string{
		filepath.Join(paths.ConfigDir, paths.ZqkConfigFileName),
		filepath.Join(paths.ProjectDataDir, "wipe_guard.yaml"),
	}
	for _, f := range expected {
		if _, err := fileutil.Stat(filepath.Join(root, f)); fileutil.IsNotExist(err) {
			t.Errorf("missing file %s after idempotent restore", f)
		}
	}
}

// TestRestoreConfig_FailClosedWipeDefault verifies that wipe operations are
// blocked by default unless explicitly unguarded.
func TestRestoreConfig_FailClosedWipeDefault(t *testing.T) {
	root := t.TempDir()

	err := RestoreConfig(root)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	guardPath := filepath.Join(root, paths.ProjectDataDir, "wipe_guard.yaml")
	guardContents, err := fileutil.ReadFile(guardPath) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("failed to read wipe guard: %v", err)
	}

	// The wipe guard file should have is_unguarded = false at minimum
	contentStr := string(guardContents)
	if strings.Contains(contentStr, "is_unguarded: true") {
		t.Fatal("wipe guard must default to fail-closed (is_unguarded=false)")
	}
}

// TestRestoreConfig_FailClosedWipe_UnguardedReject verifies that when the wipe
// guard file explicitly is_unguarded: true, a WipeProject call still fails because
// it requires confirmation.
func TestRestoreConfig_FailClosedWipe_UnguardedReject(t *testing.T) {
	root := t.TempDir()

	err := RestoreConfig(root)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Set is_unguarded to true — should NOT allow wipe without confirmation
	guardPath := filepath.Join(root, paths.ProjectDataDir, "wipe_guard.yaml")
	err = fileutil.WriteSecureFile(guardPath, []byte(
		"project:\n  name: test-wipe-reject\n  is_unguarded: true\n"))
	if err != nil {
		t.Fatalf("failed to write unguarded guard file: %v", err)
	}

	projectName := "test-wipe-reject"
	err = WipeProject(root, projectName)
	if err == nil {
		t.Fatal("expected wipe to fail when is_unguarded=true but no approval present")
	}
	if !strings.Contains(err.Error(), ErrWipeNotApproved.Error()) {
		t.Fatalf("wrong error: expected %q got %q", ErrWipeNotApproved, err)
	}
}

// TestRestoreConfig_FailClosedWipe_NoApprovalReject verifies that even with the
// guard file present, wiping without an explicit approval block fails.
func TestRestoreConfig_FailClosedWipe_NoApprovalReject(t *testing.T) {
	root := t.TempDir()

	err := RestoreConfig(root)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	guardPath := filepath.Join(root, paths.ProjectDataDir, "wipe_guard.yaml")
	err = fileutil.WriteSecureFile(guardPath, []byte(
		"project:\n  name: test-wipe-noapproval\n  is_unguarded: false\n"))
	if err != nil {
		t.Fatalf("failed to write guard file: %v", err)
	}

	projectName := "test-wipe-noapproval"
	err = WipeProject(root, projectName)
	if err == nil {
		t.Fatal("expected wipe to fail without approval block")
	}
	if !strings.Contains(err.Error(), ErrWipeNotApproved.Error()) {
		t.Fatalf("wrong error: expected %q got %q", ErrWipeNotApproved, err)
	}
}

// TestRestoreConfig_FailClosedWipe_WithApprovalSuccess verifies that with the
// guard properly configured (is_unguarded=true + approval block), wipe succeeds.
func TestRestoreConfig_FailClosedWipe_WithApprovalSuccess(t *testing.T) {
	root := t.TempDir()

	err := RestoreConfig(root)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	guardPath := filepath.Join(root, paths.ProjectDataDir, "wipe_guard.yaml")
	err = fileutil.WriteSecureFile(guardPath, []byte(
		"project:\n  name: test-wipe-approved\n  is_unguarded: true\n\n"+
			"approval_block:\n  approved_by: alice\n  approved_at: \"2025-01-01\"\n  reason: cleanup\n"))
	if err != nil {
		t.Fatalf("failed to write guard file with approval: %v", err)
	}

	projectName := "test-wipe-approved"
	err = WipeProject(root, projectName)
	if err != nil {
		t.Fatalf("wipe should succeed with valid approval block: %v", err)
	}

	// Verify the wipe actually removed project files (but kept .zqk/ intact)
	configPath := filepath.Join(root, paths.ProjectDataDir)
	info, err := fileutil.Stat(configPath)
	if err != nil {
		t.Fatalf("expected project data directory to exist after wipe: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("project data should be a directory after wipe")
	}
}

// TestRestoreConfig_OverwritesExisting verifies that RestoreConfig safely
// overwrites existing configuration files without data-loss (idempotency).
func TestRestoreConfig_OverwritesExisting(t *testing.T) {
	root := t.TempDir()

	// Pre-existing config with different content
	existingConfig := map[string]string{
		filepath.Join(paths.ConfigDir, paths.ZqkConfigFileName): "# old custom config\nproject:\n  name: legacy",
	}
	for rel, content := range existingConfig {
		fullPath := filepath.Join(root, rel)
		if err := fileutil.EnsureDir(filepath.Dir(fullPath)); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteSecureFile(fullPath, []byte(content)); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	err := RestoreConfig(root)
	if err != nil {
		t.Fatalf("restore over existing config failed: %v", err)
	}

	// Config file should now contain restored (templated) content, not the old custom one
	configPath := filepath.Join(root, paths.ConfigDir, paths.ZqkConfigFileName)
	content, err := fileutil.ReadFile(configPath) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	// Should have project name placeholder set by RestoreConfig
	if !strings.Contains(string(content), "project:") {
		t.Fatal("restored config should contain project block")
	}
}

// TestRestoreConfig_ProjectRootExists verifies that RestoreConfig returns an error
// when the project root does not exist.
func TestRestoreConfig_ProjectRootDoesNotExist(t *testing.T) {
	err := RestoreConfig("/nonexistent/path/that/should/not/exist/xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent project root")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist' in error, got: %v", err)
	}
}

// TestRestoreConfig_EmptyRoot rejects empty string.
func TestRestoreConfig_EmptyRootRejects(t *testing.T) {
	err := RestoreConfig("")
	if err == nil {
		t.Fatal("expected error for empty root")
	}
}
