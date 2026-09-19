package utility

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestResolveReferencesFromIDStream(t *testing.T) {
	t.Parallel()
	// Create coordinator (required for ScenarioBuilder)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	sb := &ScenarioBuilder{
		logger:      logging.NewEventLogger(pkgctx.NewSystemContext()),
		coordinator: coordinator,
	}
	idStream := make(map[string]string)
	idStreamMu := &sync.Mutex{}

	// Setup: Add some IDs to the stream
	idStreamMu.Lock()
	idStream["WS-001"] = "WS-001"                                                     // Workstream ID
	idStream["GOAL-001"] = "GOAL-001"                                                 // Goal ID
	idStream["ACC-1785920548450214015-3df55bd1"] = "ACC-1785920548450214015-3df55bd1" // Account ID
	idStreamMu.Unlock()

	tests := []struct {
		name     string
		obj      map[string]any
		expected map[string]any
	}{
		{
			name: "resolve single reference",
			obj: map[string]any{
				objects.FieldKeyKind:          "goal",
				objects.FieldKeyTitle:         "Test Goal",
				objects.FieldKeyWorkstreamRef: "WS-001",
			},
			expected: map[string]any{
				objects.FieldKeyKind:          "goal",
				objects.FieldKeyTitle:         "Test Goal",
				objects.FieldKeyWorkstreamRef: "WS-001", // Should remain WS-001 (already in stream)
			},
		},
		{
			name: "resolve list reference",
			obj: map[string]any{
				objects.FieldKeyKind:          "requirement",
				objects.FieldKeyTitle:         "Test Requirement",
				objects.FieldKeyGoalRefs:      []any{"GOAL-001", "GOAL-002"},
				objects.FieldKeyMilestoneRefs: []any{"MIL-001"},
			},
			expected: map[string]any{
				objects.FieldKeyKind:          "requirement",
				objects.FieldKeyTitle:         "Test Requirement",
				objects.FieldKeyGoalRefs:      []any{"GOAL-001", "GOAL-002"}, // GOAL-001 resolved, GOAL-002 not in stream
				objects.FieldKeyMilestoneRefs: []any{"MIL-001"},              // Not in stream
			},
		},
		{
			name: "resolve account reference",
			obj: map[string]any{
				objects.FieldKeyKind:     "workstream",
				objects.FieldKeyTitle:    "Test Workstream",
				objects.FieldKeyOwnerRef: "ACC-1785920548450214015-3df55bd1",
			},
			expected: map[string]any{
				objects.FieldKeyKind:     "workstream",
				objects.FieldKeyTitle:    "Test Workstream",
				objects.FieldKeyOwnerRef: "ACC-1785920548450214015-3df55bd1", // Already correct format
			},
		},
		{
			name: "no references to resolve",
			obj: map[string]any{
				objects.FieldKeyKind:  "milestone",
				objects.FieldKeyTitle: "Test Milestone",
			},
			expected: map[string]any{
				objects.FieldKeyKind:  "milestone",
				objects.FieldKeyTitle: "Test Milestone",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the object to avoid modifying the original
			objCopy := make(map[string]any)
			for k, v := range tt.obj {
				objCopy[k] = v
			}

			// Resolve references
			sb.resolveReferencesFromIDStream(objCopy, idStream, idStreamMu)

			// Check single references
			for fieldName, expectedValue := range tt.expected {
				if strings.HasSuffix(fieldName, "_ref") {
					actualValue := objCopy[fieldName]
					if actualValue != expectedValue {
						t.Errorf("Field %s: expected %v, got %v", fieldName, expectedValue, actualValue)
					}
				}
			}

			// Check list references
			for fieldName, expectedValue := range tt.expected {
				if strings.HasSuffix(fieldName, "_refs") {
					actualList := objCopy[fieldName]
					expectedList := expectedValue.([]any)
					if actualList == nil {
						t.Errorf("Field %s: expected list, got nil", fieldName)
						continue
					}
					actualListTyped := actualList.([]any)
					if len(actualListTyped) != len(expectedList) {
						t.Errorf("Field %s: expected length %d, got %d", fieldName, len(expectedList), len(actualListTyped))
						continue
					}
					// Check that resolved IDs are in the list
					for i, expectedID := range expectedList {
						if i < len(actualListTyped) {
							actualID := actualListTyped[i]
							// If expected ID is in stream, actual should match stream value
							if streamValue, inStream := idStream[expectedID.(string)]; inStream {
								if actualID != streamValue {
									t.Errorf("Field %s[%d]: expected %v (from stream), got %v", fieldName, i, streamValue, actualID)
								}
							} else {
								// Not in stream, should remain unchanged
								if actualID != expectedID {
									t.Errorf("Field %s[%d]: expected %v, got %v", fieldName, i, expectedID, actualID)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestCopyBootstrapConfigFiles_Idempotent(t *testing.T) {
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
	// Create temporary directories
	// The function walks up from current directory to find .zqk/specs
	// So we need to set up the structure where the source is accessible from the test's working directory
	tmpDir := t.TempDir()

	// Set up source directory structure that the function can find
	// It walks up from current directory, so we'll create it relative to tmpDir
	sourceDir := filepath.Join(tmpDir, paths.ProcessInternalDir)
	targetDir := datacell.CellCASPrimaryDir(filepath.Join(tmpDir, "target"), "_internal")

	if err := fileutil.MkdirAll(sourceDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}
	if err := fileutil.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	// Create a test config file in source
	configContent := `version: "1.0.0"
kind_to_prefixes:
  test_kind:
    - TEST-
`
	sourceFile := filepath.Join(sourceDir, "id_prefixes_config.yaml")
	if err := fileutil.WriteFile(sourceFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Change to tmpDir so the function can find the source by walking up
	originalWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() {
		if err := fileutil.Chdir(originalWd); err != nil {
			t.Logf("Warning: failed to restore working directory: %v", err)
		}
	}()

	// Change to a subdirectory so the function can walk up to find the source
	testSubDir := filepath.Join(tmpDir, "test_subdir")
	if err := fileutil.MkdirAll(testSubDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test subdirectory: %v", err)
	}
	if err := fileutil.Chdir(testSubDir); err != nil {
		t.Fatalf("Failed to change to test directory: %v", err)
	}

	// Create scenario builder with absolute target path
	absTargetDir, err := filepath.Abs(filepath.Join(tmpDir, "target"))
	if err != nil {
		t.Fatalf("Failed to get absolute target path: %v", err)
	}

	sb := &ScenarioBuilder{
		config: &ScenarioBuilderConfig{
			TargetDir: absTargetDir,
		},
		logger: logging.NewEventLogger(pkgctx.NewSystemContext()),
	}

	// Test 1: First copy should succeed
	if err := sb.copyBootstrapConfigFiles(targetDir); err != nil {
		t.Fatalf("First copy failed: %v", err)
	}

	targetFile := filepath.Join(targetDir, "id_prefixes_config.yaml")
	if _, err := fileutil.Stat(targetFile); fileutil.IsNotExist(err) {
		t.Fatal("Target file was not created")
	}

	// Verify content
	targetContent, err := fileutil.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("Failed to read target file: %v", err)
	}
	if string(targetContent) != configContent {
		t.Errorf("Target file content doesn't match source")
	}

	// Test 2: Second copy should be idempotent (no change if source hasn't changed)
	// Get target mtime before second copy
	targetInfo1, err := fileutil.Stat(targetFile)
	if err != nil {
		t.Fatalf("Failed to stat target file: %v", err)
	}
	targetMTime1 := targetInfo1.ModTime()

	// Wait a bit to ensure mtime would change if file was rewritten
	time.Sleep(2 * time.Second)

	// Second copy should not change the file (source hasn't changed)
	// Note: The function may return an error if no files need copying (all up to date)
	// This is expected behavior - the function returns error if copiedCount == 0
	// But in our case, the file should exist and be up to date, so it should skip it
	// However, if all files are skipped (up to date), it returns an error
	// So we need to ensure at least one file would be copied, or handle the error case
	err = sb.copyBootstrapConfigFiles(targetDir)
	// The function returns error if no files were copied (all skipped)
	// This is actually correct behavior - if all files are up to date, it means nothing to do
	// But for idempotency testing, we want to verify the file wasn't changed
	if err != nil && !strings.Contains(err.Error(), "no bootstrap config files found to copy") {
		t.Fatalf("Second copy failed with unexpected error: %v", err)
	}
	// If error is "no bootstrap config files found to copy", that's OK - it means all files were skipped (up to date)

	targetInfo2, err := fileutil.Stat(targetFile)
	if err != nil {
		t.Fatalf("Failed to stat target file after second copy: %v", err)
	}
	targetMTime2 := targetInfo2.ModTime()

	// Mtime should be preserved (not updated) since source hasn't changed
	// Note: On some filesystems, mtime might be slightly different due to precision
	// So we check that it's close (within 1 second)
	if targetMTime2.After(targetMTime1.Add(1 * time.Second)) {
		t.Errorf("Target file mtime changed when it shouldn't have (idempotent check failed)")
	}

	// Test 3: Update source file and verify it gets copied
	updatedContent := `version: "1.0.0"
kind_to_prefixes:
  test_kind:
    - TEST-
    - TEST2-
`
	if err := fileutil.WriteFile(sourceFile, []byte(updatedContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update source file: %v", err)
	}

	// Wait to ensure mtime changes
	time.Sleep(2 * time.Second)

	// Third copy should update the target (source is newer)
	// This should succeed because source file was updated
	if err := sb.copyBootstrapConfigFiles(targetDir); err != nil {
		t.Fatalf("Third copy failed: %v", err)
	}

	// Verify updated content
	targetContent2, err := fileutil.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("Failed to read target file after update: %v", err)
	}
	if string(targetContent2) != updatedContent {
		t.Errorf("Target file was not updated with new content")
	}
}

func TestConfigFileWatcher_RegisterAndDetectChanges(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping ConfigFileWatcher test in short mode (long poll/sleep and goroutine teardown)")
	}
	tmpDir := t.TempDir()
	internalDir := filepath.Join(tmpDir, paths.ProcessInternalDir)
	if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create internal directory: %v", err)
	}

	// Create a mock coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	watcher := NewConfigFileWatcher(tmpDir, coordinator, logger)

	// Register a test file
	var reloadCalled atomic.Bool
	watcher.RegisterFile("test_config.yaml", "config_test_changed", func() error {
		reloadCalled.Store(true)
		return nil
	})

	// Create the file
	testFile := filepath.Join(internalDir, "test_config.yaml")
	initialContent := "version: 1.0.0\n"
	if err := fileutil.WriteFile(testFile, []byte(initialContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Start watcher
	ctx := pkgctx.NewSystemContext()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait a bit for initial state to be recorded
	time.Sleep(1 * time.Second)

	// Modify the file
	updatedContent := "version: 2.0.0\n"
	if err := fileutil.WriteFile(testFile, []byte(updatedContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	// Wait for watcher to detect change (poll interval is 5 seconds)
	time.Sleep(6 * time.Second)

	// Verify reload was called
	if !reloadCalled.Load() {
		t.Error("Reload function was not called when file changed")
	}
}
