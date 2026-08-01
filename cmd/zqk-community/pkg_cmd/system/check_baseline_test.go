package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/testkit"
)

// TestCheckBaseline_CollectMetrics tests baseline metric collection.
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestCheckBaseline_CollectMetrics(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "cmd.system.check_baseline"})
	testRoot := proj.Root

	// Create test objects
	testObjects := createTestObjectsForBaseline(t, testRoot, 100)

	// Run baseline collection - scan for object files
	startTime := time.Now()
	allObjects := make([]string, 0)
	processDir := datacell.ProcessPrimaryDir(testRoot)

	walkErr := filepath.Walk(processDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".yaml" {
			// Extract object ID from filename (e.g., "TEST-001.yaml" -> "TEST-001")
			baseName := filepath.Base(path)
			objectID := baseName[:len(baseName)-5] // Remove ".yaml"
			allObjects = append(allObjects, objectID)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to discover objects: %v", walkErr)
	}
	duration := time.Since(startTime)

	// Verify metrics
	if len(allObjects) != len(testObjects) {
		t.Errorf("Expected %d objects, got %d", len(testObjects), len(allObjects))
	}

	t.Logf("Baseline Metrics:")
	t.Logf("  Total Objects: %d", len(allObjects))
	t.Logf("  Duration: %v", duration)
	t.Logf("  Objects/Second: %.2f", float64(len(allObjects))/duration.Seconds())
}

// createTestObjectsForBaseline creates test objects for baseline testing
func createTestObjectsForBaseline(t *testing.T, testRoot string, count int) []string {
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := os.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	objectIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		objectID := fmt.Sprintf("TEST-%03d", i)
		filePath := filepath.Join(testDir, fmt.Sprintf("%s.yaml", objectID))

		content := fmt.Sprintf(`id: %s
kind: test_object
title: Test Object %d
status: active
schema_version: "`+objects.DefaultSchemaVersion+`"
created_at: "%s"
created_by: account:test
`, objectID, i, time.Now().Format(time.RFC3339))

		if err := os.WriteFile(filePath, []byte(content), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create test object: %v", err)
		}

		objectIDs = append(objectIDs, objectID)
	}

	return objectIDs
}
