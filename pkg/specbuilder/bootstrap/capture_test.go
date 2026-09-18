package bootstrap

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCaptureAndRestore(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	bootstrapFile := filepath.Join(t.TempDir(), "bootstrap.yaml")

	// Create test structure in source
	setupTestStructure(t, sourceDir)

	// Capture state
	capture, err := CaptureCurrentState(sourceDir)
	if err != nil {
		t.Fatalf("Failed to capture state: %v", err)
	}

	// Verify capture
	if len(capture.ObjectSpecs) == 0 {
		t.Error("Expected object specs to be captured")
	}
	if capture.Metadata.FileCounts["object_specs"] == 0 {
		t.Error("Expected object specs count to be set")
	}

	// Save to file
	if err := capture.SaveToFile(bootstrapFile); err != nil {
		t.Fatalf("Failed to save bootstrap file: %v", err)
	}

	// Load from file
	loadedCapture, err := LoadFromFile(bootstrapFile)
	if err != nil {
		t.Fatalf("Failed to load bootstrap file: %v", err)
	}

	// Verify loaded capture
	if len(loadedCapture.ObjectSpecs) != len(capture.ObjectSpecs) {
		t.Errorf("Expected %d object specs, got %d", len(capture.ObjectSpecs), len(loadedCapture.ObjectSpecs))
	}

	// Restore to target
	if err := RestoreState(loadedCapture, targetDir, false); err != nil {
		t.Fatalf("Failed to restore state: %v", err)
	}

	// Verify restored files
	verifyRestoredFiles(t, sourceDir, targetDir)
}

func setupTestStructure(t *testing.T, baseDir string) {
	// Create object_specs directory
	specsDir := filepath.Join(baseDir, "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}

	// Create a test spec file
	specContent := `ontology: test_object
description: Test object spec
fields:
  name:
    type: string
    validation:
      required: true
`
	specFile := filepath.Join(specsDir, "test_object.yaml")
	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Create a test config file
	configContent := fmt.Sprintf("version: %q\ntest: true\n", BootstrapCaptureFormatVersion)
	configFile := filepath.Join(baseDir, "test_config.yaml")
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
}

func verifyRestoredFiles(t *testing.T, sourceDir, targetDir string) {
	// Verify object spec was restored
	sourceSpec := filepath.Join(sourceDir, "object_specs", "test_object.yaml")
	targetSpec := filepath.Join(targetDir, "object_specs", "test_object.yaml")

	sourceContent, err := fileutil.ReadFile(sourceSpec)
	if err != nil {
		t.Fatalf("Failed to read source spec: %v", err)
	}

	targetContent, err := fileutil.ReadFile(targetSpec)
	if err != nil {
		t.Fatalf("Failed to read target spec: %v", err)
	}

	if !bytes.Equal(sourceContent, targetContent) {
		t.Error("Restored spec content does not match source")
	}

	// Verify config file was restored
	sourceConfig := filepath.Join(sourceDir, "test_config.yaml")
	targetConfig := filepath.Join(targetDir, "test_config.yaml")

	sourceConfigContent, err := fileutil.ReadFile(sourceConfig)
	if err != nil {
		t.Fatalf("Failed to read source config: %v", err)
	}

	targetConfigContent, err := fileutil.ReadFile(targetConfig)
	if err != nil {
		t.Fatalf("Failed to read target config: %v", err)
	}

	if !bytes.Equal(sourceConfigContent, targetConfigContent) {
		t.Error("Restored config content does not match source")
	}
}

func TestInitializeFromBootstrap(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	bootstrapFile := filepath.Join(t.TempDir(), "bootstrap.yaml")

	// Setup test structure
	setupTestStructure(t, sourceDir)

	// Capture and save
	capture, err := CaptureCurrentState(sourceDir)
	if err != nil {
		t.Fatalf("Failed to capture state: %v", err)
	}

	if err := capture.SaveToFile(bootstrapFile); err != nil {
		t.Fatalf("Failed to save bootstrap file: %v", err)
	}

	// Initialize from bootstrap
	if err := InitializeFromBootstrap(bootstrapFile, targetDir, false); err != nil {
		t.Fatalf("Failed to initialize from bootstrap: %v", err)
	}

	// Verify files were restored
	verifyRestoredFiles(t, sourceDir, targetDir)
}
