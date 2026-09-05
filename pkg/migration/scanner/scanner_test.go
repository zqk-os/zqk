package scanner

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestScanYAMLFiles(t *testing.T) {
	t.Parallel()
	// Create a temporary directory structure for testing
	tempDir := t.TempDir()

	// Create test YAML files
	testFiles := []struct {
		path    string
		content string
	}{
		{
			path: "backlog/BLI-001.yaml",
			content: `id: BLI-001
kind: backlog_item
title: Test Backlog Item
status: planned
`,
		},
		{
			path: "goals/GOAL-001.yaml",
			content: `id: GOAL-001
kind: goal
title: Test Goal
status: active
`,
		},
		{
			path: "_internal/spec.yaml",
			content: `# Internal file - should be excluded
`,
		},
	}

	// Create files
	for _, tf := range testFiles {
		fullPath := filepath.Join(tempDir, tf.path)
		fileutil.MkdirAll(filepath.Dir(fullPath), paths.DirPerm755)
		//nolint:errcheck // Test cleanup - errors are acceptable
		fileutil.WriteFile(fullPath, []byte(tf.content), paths.FilePerm644)
	}

	// Test scanning
	scanner := NewYAMLScanner(tempDir)
	files, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should find 2 files (excluding _internal)
	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Verify files are correct
	foundBacklog := false
	foundGoal := false
	for _, f := range files {
		if filepath.Base(f.Path) == "BLI-001.yaml" {
			foundBacklog = true
		}
		if filepath.Base(f.Path) == "GOAL-001.yaml" {
			foundGoal = true
		}
	}

	if !foundBacklog {
		t.Error("Expected to find BLI-001.yaml")
	}
	if !foundGoal {
		t.Error("Expected to find GOAL-001.yaml")
	}
}

func TestScanYAMLFiles_ExcludePatterns(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	// Create files in excluded directories
	excludedFiles := []string{
		"_internal/spec.yaml",
		".git/config",
		"node_modules/test.yaml",
	}

	for _, path := range excludedFiles {
		fullPath := filepath.Join(tempDir, path)
		fileutil.MkdirAll(filepath.Dir(fullPath), paths.DirPerm755)
		//nolint:errcheck // Test cleanup - errors are acceptable
		fileutil.WriteFile(fullPath, []byte("test"), paths.FilePerm644)
	}

	scanner := NewYAMLScanner(tempDir)
	files, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should find 0 files (all excluded)
	if len(files) != 0 {
		t.Errorf("Expected 0 files (all excluded), got %d", len(files))
	}
}

func TestScanYAMLFiles_EmptyDirectory(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	scanner := NewYAMLScanner(tempDir)
	files, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files in empty directory, got %d", len(files))
	}
}
