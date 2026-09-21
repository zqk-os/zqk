package testing

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// SnapshotTestHelper provides utilities for running tests against restored snapshots
// This ensures tests are snapshot-agnostic and can work with both original and restored data
type SnapshotTestHelper struct {
	TestRoot     string
	SnapshotPath string
	IsRestored   bool
	OriginalRoot string
}

// SetupSnapshotTestEnvironment sets up a test environment from a snapshot
// This is the recommended way to test against restored snapshot data
func SetupSnapshotTestEnvironment(t *testing.T, snapshotPath string) (*SnapshotTestHelper, error) {
	// Create temporary directory for restored snapshot
	tmpDir := t.TempDir()

	// Set ZQK_TEST_ROOT to use the temporary directory
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	// Find original project root (for reading objects during restoration)
	originalRoot := findOriginalProjectRoot(snapshotPath)
	if originalRoot == emptyValue {
		return nil, errfmt.Errorf("cannot determine original project root for snapshot restoration")
	}

	helper := &SnapshotTestHelper{
		TestRoot:     tmpDir,
		SnapshotPath: snapshotPath,
		IsRestored:   false,
		OriginalRoot: originalRoot,
	}

	return helper, nil
}

// RestoreSnapshot restores a snapshot to the test environment
// This should be called after SetupSnapshotTestEnvironment
func (h *SnapshotTestHelper) RestoreSnapshot(t *testing.T) error {
	// Run init command with snapshot
	// This would typically call: zqk system init --from-snapshot <snapshot> --wipe --force
	// For now, we'll document the pattern - actual implementation would use exec.Command

	// TODO: Implement actual snapshot restoration via CLI command
	// For now, this is a placeholder that documents the pattern

	t.Logf("Snapshot restoration would be performed here")
	t.Logf(paths.RewriteCanonicalCLIInvocations("Command: zqk system init --from-snapshot %s --wipe --force"), h.SnapshotPath)

	h.IsRestored = true
	return nil
}

// GetProjectRoot returns the project root to use for tests
// This respects ZQK_TEST_ROOT if set
func (h *SnapshotTestHelper) GetProjectRoot() string {
	if h.TestRoot != emptyValue {
		return h.TestRoot
	}
	return h.OriginalRoot
}

// findOriginalProjectRoot attempts to find the original project root
// This is used when restoring snapshots that need to read objects from the original project
func findOriginalProjectRoot(snapshotPath string) string {
	// If snapshot is in test-scenarios, original is 2 levels up
	snapshotDir := filepath.Dir(snapshotPath)
	if contains(snapshotDir, "test-scenarios") {
		originalRoot := filepath.Clean(filepath.Join(snapshotDir, "..", ".."))
		// Verify it has .zqk/process
		if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(originalRoot)); err == nil {
			return originalRoot
		}
	}

	// Try to find from current working directory
	cwd, err := fileutil.Getwd()
	if err == nil {
		// Look for .zqk or .zqk/process
		for dir := cwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
				return dir
			}
			if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
				return dir
			}
		}
	}

	return ""
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || containsMiddle(s, substr))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SnapshotAgnosticTestPattern provides a pattern for writing snapshot-agnostic tests
// Tests should:
// 1. Use ZQK_TEST_ROOT for isolation
// 2. Avoid hardcoding object IDs or file paths
// 3. Use dynamic discovery of objects
// 4. Compare results by structure, not by specific IDs
type SnapshotAgnosticTestPattern struct {
	// TestRoot is the root directory for test data
	TestRoot string

	// UseSnapshot indicates whether to use snapshot data
	UseSnapshot bool

	// SnapshotPath is the path to the snapshot file (if UseSnapshot is true)
	SnapshotPath string
}

// NewSnapshotAgnosticTest creates a new snapshot-agnostic test pattern
func NewSnapshotAgnosticTest(t *testing.T, useSnapshot bool, snapshotPath string) *SnapshotAgnosticTestPattern {
	tmpDir := t.TempDir()

	// Set ZQK_TEST_ROOT
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	return &SnapshotAgnosticTestPattern{
		TestRoot:     tmpDir,
		UseSnapshot:  useSnapshot,
		SnapshotPath: snapshotPath,
	}
}

// GetTestRoot returns the test root directory
func (p *SnapshotAgnosticTestPattern) GetTestRoot() string {
	return p.TestRoot
}
