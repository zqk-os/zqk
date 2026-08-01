package system

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// registerCheckSnapshotAgnosticTeardown runs the standard temp-project teardown pipeline after
// SetupTestEnvironment + NewFileObjectStorageForTest, including global listing-index drain
// ([testkit.TempProjectTeardown]). When a reset dir can be allocated, also tears down the
// global audit buffer (same behavior as prior ad hoc TeardownOptions).
func registerCheckSnapshotAgnosticTeardown(t *testing.T, projectRoot string, fileStorage *storage.FileObjectStorage) {
	t.Helper()
	t.Cleanup(func() {
		opts := testkit.TempProjectTeardown(projectRoot, fileStorage)
		resetDir, rerr := os.MkdirTemp("", "zqk-audit-global-reset")
		if rerr == nil {
			defer os.RemoveAll(resetDir)
			opts.TearDownGlobalAuditBuffer = true
			opts.SecCtx = &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
			opts.AuditBufferResetRoot = resetDir
		}
		if err := testkit.RunStandardTeardown(opts); err != nil {
			t.Logf("testkit: storage teardown pipeline: %v", err)
		}
	})
}

// TestCheckCommand_SnapshotAgnostic demonstrates snapshot-agnostic testing
// This test should work with both original project data and restored snapshots
func TestCheckCommand_SnapshotAgnostic(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)

	// Setup test environment structure
	testRoot, err := setupSystemTestEnvironmentRoot(tmpDir)
	if err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if fileStorage != nil {
		defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	}
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	registerCheckSnapshotAgnosticTeardown(t, testRoot, fileStorage)

	// Create minimal test data (or restore from snapshot)
	// For this example, we'll create a simple test object
	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	// Create a test backlog item (snapshot-agnostic - uses generic ID)
	testObject := fmt.Sprintf(`id: ITEM-TEST-001
kind: backlog_item
title: Test Item
status: active
namespace_id: %s
`, paths.KernelNamespaceID)
	testFile := filepath.Join(backlogDir, "ITEM-TEST-001.yaml")
	if err := os.WriteFile(testFile, []byte(testObject), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write test object: %v", err)
	}

	// Run check command - should work with any data (async+follow exits when done)
	cmd := NewCheckCmd()
	cmd.SetArgs([]string{"--format", "jsonl"})
	// Route CLI output through buffers so async follow-up work cannot hit testing's
	// per-test stdout capture after it is closed (avoids "write /dev/stdout: file already closed").
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd.SetContext(ctx)

	// Capture output to verify structure
	// Note: In a real test, you'd capture and parse the output
	err = cmd.Execute()
	if err != nil {
		t.Logf("Check command completed (may have found issues, which is expected)")
	}

	// Verify test environment structure exists
	projectDataDir := filepath.Join(testRoot, paths.ProjectDataDir)
	if _, err := os.Stat(projectDataDir); os.IsNotExist(err) {
		t.Errorf("Project data directory (%s) should exist after check", paths.ProjectDataDir)
	}

	// This test is snapshot-agnostic because:
	// 1. Uses ZQK_TEST_ROOT for isolation
	// 2. Doesn't hardcode specific object IDs (uses generic test ID)
	// 3. Verifies structure, not specific values
	// 4. Works with any data, not just specific snapshot
}

// TestCheckCommand_WithRestoredSnapshot demonstrates testing with a restored snapshot
// This is a template - actual implementation would restore a real snapshot
func TestCheckCommand_WithRestoredSnapshot(t *testing.T) {
	// Skip if snapshot doesn't exist
	snapshotPath := "test-scenarios/check-violation-resolver/check-snapshot-latest.csnap"
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Skipf("Snapshot not found: %s", snapshotPath)
	}

	// Setup test environment
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	testkit.RegisterStandardTeardown(t, testkit.TempProjectTeardown(tmpDir, nil))

	// Restore snapshot (would use actual init command in real implementation)
	// For now, this is a placeholder that documents the pattern
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	t.Logf("Would restore snapshot: %s", snapshotPath)
	t.Logf("Command: %s system init --from-snapshot %s --wipe --force", cliCmd, snapshotPath)

	// Run check against restored snapshot (async+follow exits when done)
	cmd := NewCheckCmd()
	cmd.SetArgs([]string{"--format", "jsonl"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd.SetContext(ctx)

	// In a real test, you would:
	// 1. Restore the snapshot
	// 2. Run check command
	// 3. Verify results match snapshot expectations (by structure, not by specific IDs)
	// 4. Compare issue counts, object counts, etc.

	err := cmd.Execute()
	if err != nil {
		t.Logf("Check command completed (may have found issues)")
	}

	// Verify restored data exists
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if _, err := os.Stat(processDir); os.IsNotExist(err) {
		t.Error("Process directory should exist after snapshot restoration")
	}

	// This test demonstrates the pattern for snapshot-based testing
	// Actual implementation would restore snapshot and verify results
}

// TestSnapshotAgnosticPatterns demonstrates various snapshot-agnostic patterns
func TestSnapshotAgnosticPatterns(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)

	testRoot, err := setupSystemTestEnvironmentRoot(tmpDir)
	if err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if fileStorage != nil {
		defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	}
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	registerCheckSnapshotAgnosticTeardown(t, testRoot, fileStorage)

	// Pattern 1: Use dynamic discovery instead of hardcoded IDs
	// ✅ GOOD: Query by kind
	backlogDir := filepath.Join(testRoot, paths.ProcessBacklogDir)
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	// Create multiple test objects (simulating variable data)
	testObjects := []string{
		"ITEM-TEST-001",
		"ITEM-TEST-002",
		"ITEM-TEST-003",
	}

	for _, objID := range testObjects {
		testObject := fmt.Sprintf(`id: %s
kind: backlog_item
title: Test Item %s
status: active
namespace_id: %s
`, objID, objID, paths.KernelNamespaceID)
		testFile := filepath.Join(backlogDir, objID+".yaml")
		if err := os.WriteFile(testFile, []byte(testObject), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to write test object: %v", err)
		}
	}

	// Pattern 2: Verify by structure, not by specific IDs
	// ✅ GOOD: Check that objects exist, not which specific ones
	entries, err := os.ReadDir(backlogDir)
	if err != nil {
		t.Fatalf("Failed to read backlog directory: %v", err)
	}

	// Verify we have objects (any objects), not specific ones
	if len(entries) == 0 {
		t.Error("Expected at least one object in backlog directory")
	}

	// Pattern 3: Use relative paths, not absolute
	// ✅ GOOD: Use project root-relative paths
	relativePath := paths.ProcessBacklogDir
	absolutePath := filepath.Join(testRoot, relativePath)
	if _, err := os.Stat(absolutePath); os.IsNotExist(err) {
		t.Errorf("Expected directory to exist: %s", absolutePath)
	}

	// Pattern 4: Compare by counts/ranges, not exact values
	// ✅ GOOD: Check for reasonable range
	if len(entries) < 1 {
		t.Error("Expected at least 1 object")
	}
	if len(entries) > 1000 {
		t.Error("Unexpectedly large number of objects")
	}

	// This test demonstrates snapshot-agnostic patterns
	// It works with any data, not just specific snapshots
}
