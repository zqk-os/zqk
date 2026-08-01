package testscan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestScanner_Scan(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create a test package structure
	testPkgDir := filepath.Join(projectRoot, "pkg", "example")
	if err := os.MkdirAll(testPkgDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test package directory: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(testPkgDir, "example_test.go")
	testContent := `package example

import "testing"

func TestExample(t *testing.T) {
	t.Parallel()
	// Test implementation
}

func TestSequential(t *testing.T) {
	// This test does not call t.Parallel()
	// Test implementation
}

func TestHelper(t *testing.T) {
	// Helper function, not a test
}
`
	if err := os.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create scanner and scan
	scanner := NewScanner(projectRoot)
	tests, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Verify we found the test functions
	if len(tests) < 2 {
		t.Errorf("Expected at least 2 test functions, got %d", len(tests))
	}

	// Find TestExample and verify it's marked as parallel
	foundParallel := false
	foundSequential := false
	for _, test := range tests {
		if test.Name == "TestExample" {
			if !test.IsParallel {
				t.Error("TestExample should be marked as parallel")
			}
			foundParallel = true
		}
		if test.Name == "TestSequential" {
			if test.IsParallel {
				t.Error("TestSequential should not be marked as parallel")
			}
			foundSequential = true
		}
	}

	if !foundParallel {
		t.Error("Expected to find TestExample")
	}
	if !foundSequential {
		t.Error("Expected to find TestSequential")
	}
}

func TestScanner_BundleTests(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	scanner := NewScanner(projectRoot)

	// Create mock test functions
	tests := []*TestFunction{
		{
			Name:              "TestA",
			Package:           "example",
			PackagePath:       "pkg/example",
			IsParallel:        true,
			EstimatedDuration: 10 * time.Second,
		},
		{
			Name:              "TestB",
			Package:           "example",
			PackagePath:       "pkg/example",
			IsParallel:        true,
			EstimatedDuration: 20 * time.Second,
		},
		{
			Name:              "TestC",
			Package:           "example",
			PackagePath:       "pkg/example",
			IsParallel:        false,
			EstimatedDuration: 15 * time.Second,
		},
		{
			Name:              "TestD",
			Package:           "other",
			PackagePath:       "pkg/other",
			IsParallel:        true,
			EstimatedDuration: 5 * time.Second,
		},
	}

	// Bundle tests with max size of 2
	bundles := scanner.BundleTests(tests, 2)

	// Verify we have bundles
	if len(bundles) == 0 {
		t.Fatal("Expected at least one bundle")
	}

	// Verify bundle properties
	for _, bundle := range bundles {
		if len(bundle.Tests) == 0 {
			t.Error("Bundle should not be empty")
		}
		if bundle.PackagePath == "" {
			t.Error("Bundle should have a package path")
		}

		// Verify all tests in bundle have same parallel status
		for _, test := range bundle.Tests {
			if test.IsParallel != bundle.IsParallel {
				t.Errorf("Test %s has mismatched parallel status in bundle", test.Name)
			}
		}
	}
}

func TestScanner_ScheduleBundles(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	scanner := NewScanner(projectRoot)

	// Create mock bundles
	bundles := []*TestBundle{
		{
			ID:                "bundle-1",
			IsParallel:        true,
			EstimatedDuration: 10 * time.Second,
		},
		{
			ID:                "bundle-2",
			IsParallel:        true,
			EstimatedDuration: 20 * time.Second,
		},
		{
			ID:                "bundle-3",
			IsParallel:        false,
			EstimatedDuration: 15 * time.Second,
		},
		{
			ID:                "bundle-4",
			IsParallel:        false,
			EstimatedDuration: 5 * time.Second,
		},
	}

	// Schedule bundles
	scheduled := scanner.ScheduleBundles(bundles, 4)

	// Verify scheduling order: parallel bundles first, then sequential
	// Within each group, should be sorted by duration (longest first)
	if len(scheduled) != len(bundles) {
		t.Errorf("Expected %d scheduled bundles, got %d", len(bundles), len(scheduled))
	}

	// Verify parallel bundles come before sequential bundles
	foundSequential := false
	for i, bundle := range scheduled {
		if !bundle.IsParallel {
			foundSequential = true
		}
		if foundSequential && bundle.IsParallel {
			t.Error("Parallel bundles should come before sequential bundles")
		}
		// Verify bundles are sorted by duration within their group
		if i > 0 {
			prevBundle := scheduled[i-1]
			if prevBundle.IsParallel == bundle.IsParallel {
				if prevBundle.EstimatedDuration < bundle.EstimatedDuration {
					t.Error("Bundles should be sorted by duration (longest first)")
				}
			}
		}
	}
}

func TestTimingDataAccessor(t *testing.T) {
	accessor := NewTimingDataAccessor()

	// Test getting timing for non-existent test
	timing := accessor.GetTiming("NonExistentTest", "example")
	if timing != nil {
		t.Error("Expected nil for non-existent test")
	}

	// Note: We can't easily test with real timing data without setting up
	// the timing file, but the accessor should handle missing data gracefully
}

func TestSuggestedTimeoutForPackage(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := tmpDir
	testPkgDir := filepath.Join(projectRoot, "pkg", "example")
	if err := os.MkdirAll(testPkgDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test package directory: %v", err)
	}
	testFile := filepath.Join(testPkgDir, "example_test.go")
	content := `package example
import "testing"
func TestA(t *testing.T) {}
func TestB(t *testing.T) {}
`
	if err := os.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	seconds, err := SuggestedTimeoutForPackage(projectRoot, "pkg/example")
	if err != nil {
		t.Fatalf("SuggestedTimeoutForPackage: %v", err)
	}
	// Formula: sum(estimated) * 1.5, min 300; pkg/example gets 30s per test default, so 60*1.5=90 -> floor 300
	if seconds < 300 {
		t.Errorf("expected at least 300s, got %d", seconds)
	}
	if seconds > 3600 {
		t.Errorf("expected at most 3600s, got %d", seconds)
	}

	// With ./ prefix
	seconds2, err := SuggestedTimeoutForPackage(projectRoot, "./pkg/example")
	if err != nil {
		t.Fatalf("SuggestedTimeoutForPackage(./pkg/example): %v", err)
	}
	if seconds2 != seconds {
		t.Errorf("expected same result with ./ prefix: got %d vs %d", seconds2, seconds)
	}
}
