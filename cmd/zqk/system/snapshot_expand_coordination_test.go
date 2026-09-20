package system

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Tests using setupSnapshotExpandTest must not use t.Parallel(): ZQK_TEST_ROOT is process-global.

// setupSnapshotExpandTest creates a test environment for snapshot-expand tests via [testkit.PrepareIsolatedTempProject].
func setupSnapshotExpandTest(t *testing.T) (string, storage.ObjectStorageProvider, func()) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.system.snapshot_expand"})
	return proj.Root, proj.FileStorage, func() {}
}

// createTestSnapshot creates a minimal test snapshot file using the storage package functions
func createTestSnapshot(t *testing.T, snapshotPath string, objects []map[string]any) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	timestamp := time.Now().UTC()

	// Create compressed snapshot using the storage package function
	cs, err := storage.CreateCompressedSnapshot(objects, timestamp, logger)
	if err != nil {
		t.Fatalf("Failed to create compressed snapshot: %v", err)
	}

	// Write snapshot file
	if err := storage.WriteCompressedSnapshot(cs, snapshotPath); err != nil {
		t.Fatalf("Failed to write snapshot file: %v", err)
	}
}

// TestSnapshotExpand_WithCoordinator tests snapshot-expand with coordinator integration
func TestSnapshotExpand_WithCoordinator(t *testing.T) {
	testRoot, fileStorage, cleanup := setupSnapshotExpandTest(t)
	defer cleanup()

	// Create test snapshot file
	snapshotPath := filepath.Join(testRoot, "test.csnap")
	testObjects := []map[string]any{
		{
			objects.FieldKeyID:            "TEST-001",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Test Item 1",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "TEST-002",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Test Item 2",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
	}
	createTestSnapshot(t, snapshotPath, testObjects)

	// Create output directory
	outputDir := filepath.Join(testRoot, "expanded")
	if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	// Change to test root directory so command can find project root
	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	if err := fileutil.Chdir(testRoot); err != nil {
		t.Fatalf("Failed to change to test root: %v", err)
	}

	// Create command
	cmd := NewSnapshotExpandCmd()
	cmd.SetArgs([]string{snapshotPath, "--output-dir", outputDir})

	// Execute command
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("snapshot-expand failed: %v", err)
	}

	// Verify expanded files exist (files are written to subdirectories by kind)
	// Objects are written to outputDir/kind/ID.yaml (e.g., expanded/backlog/TEST-001.yaml)
	backlogDir := filepath.Join(outputDir, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	expandedFiles, err := filepath.Glob(filepath.Join(backlogDir, "*.yaml"))
	if err != nil {
		t.Fatalf("Failed to list expanded files: %v", err)
	}

	if len(expandedFiles) != len(testObjects) {
		t.Errorf("Expected %d expanded files in backlog directory, got %d. Files: %v", len(testObjects), len(expandedFiles), expandedFiles)
	}

	// Give async operations time to complete (audit events are async and best-effort)
	// Note: High-volume events are buffered - low/medium severity events are aggregated
	// and flushed periodically (default: hourly) or when threshold is exceeded (default: 10 events).
	// High-severity events are written immediately. Events may not appear immediately in storage.
	time.Sleep(1000 * time.Millisecond)

	// Verify audit events were created (best-effort, so log but don't fail)
	// Events may be buffered and not yet flushed to storage, which is expected behavior.
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:  "audit_event",
		Limit: 20,
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Logf("Failed to list audit events (non-critical): %v", err)
	} else if len(result.Objects) == 0 {
		t.Logf("No audit events found (events are best-effort and may not be created in test environment)")
	} else {
		t.Logf("Created %d audit events for snapshot-expand", len(result.Objects))
		// Verify we have events related to snapshot expansion
		foundSnapshotEvent := false
		for _, event := range result.Objects {
			operation, _ := event[objects.FieldKeyOperation].(string)
			eventType, _ := event[objects.FieldKeyEventType].(string)
			if (operation != emptyValue && strings.Contains(strings.ToLower(operation), "snapshot")) ||
				(eventType != emptyValue && strings.Contains(strings.ToLower(eventType), "snapshot")) {
				foundSnapshotEvent = true
				t.Logf("Found snapshot event: operation=%v, event_type=%v", operation, eventType)
				break
			}
		}
		if !foundSnapshotEvent {
			t.Logf("No snapshot-specific events found, but %d total audit events were created", len(result.Objects))
		}
	}
}

// TestSnapshotExpand_ErrorHandling tests error handling with coordinator
func TestSnapshotExpand_ErrorHandling(t *testing.T) {
	testRoot, fileStorage, cleanup := setupSnapshotExpandTest(t)
	defer cleanup()

	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	if err := fileutil.Chdir(testRoot); err != nil {
		t.Fatalf("Failed to change to test root: %v", err)
	}

	// Try to expand non-existent snapshot
	cmd := NewSnapshotExpandCmd()
	cmd.SetArgs([]string{"nonexistent.csnap"})

	// Execute should fail
	err = cmd.Execute()
	if err == nil {
		t.Error("Expected error for non-existent snapshot file")
	}

	// Give async operations time to complete
	time.Sleep(500 * time.Millisecond)

	// Verify error audit event may have been created (best-effort, so may or may not exist)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:  "audit_event",
		Limit: 10,
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err == nil {
		t.Logf("Found %d audit events after error (may include error events)", len(result.Objects))
	}
}

// TestSnapshotExpand_VerifyOnly tests verify-only mode with coordinator
func TestSnapshotExpand_VerifyOnly(t *testing.T) {
	testRoot, _, cleanup := setupSnapshotExpandTest(t)
	defer cleanup()

	// Create test snapshot
	snapshotPath := filepath.Join(testRoot, "test.csnap")
	createTestSnapshot(t, snapshotPath, []map[string]any{
		{
			objects.FieldKeyID:            "TEST-001",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Test Item",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
	})

	originalDir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer fileutil.Chdir(originalDir)

	if err := fileutil.Chdir(testRoot); err != nil {
		t.Fatalf("Failed to change to test root: %v", err)
	}

	// Create command with verify-only flag
	cmd := NewSnapshotExpandCmd()
	cmd.SetArgs([]string{snapshotPath, "--verify-only"})

	// Execute should succeed (verify-only doesn't expand)
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("snapshot-expand --verify-only failed: %v", err)
	}

	// Give async operations time to complete
	time.Sleep(500 * time.Millisecond)

	// Command should complete successfully
}
