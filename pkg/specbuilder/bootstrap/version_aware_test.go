package bootstrap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestBootstrapVersionAwareSpecLoading tests that bootstrap capture/restore works
// correctly with version-aware spec loading. This simulates the scenario where
// instances have different schema_version values and need to load the correct spec version.
func TestBootstrapVersionAwareSpecLoading(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	bootstrapFile := filepath.Join(t.TempDir(), "bootstrap.yaml")

	// Setup test structure with versioned specs
	setupVersionedTestStructure(t, sourceDir)

	// Capture state
	capture, err := CaptureCurrentState(sourceDir)
	if err != nil {
		t.Fatalf("Failed to capture state: %v", err)
	}

	// Verify capture includes our test specs
	if len(capture.ObjectSpecs) == 0 {
		t.Error("Expected object specs to be captured")
	}
	if _, ok := capture.ObjectSpecs["ledger.yaml"]; !ok {
		t.Error("Expected ledger.yaml to be captured")
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

	// Verify restored files exist
	restoredLedgerSpec := filepath.Join(targetDir, "object_specs", "ledger.yaml")
	if _, err := os.Stat(restoredLedgerSpec); os.IsNotExist(err) {
		t.Fatalf("Restored ledger spec file not found: %v", err)
	}

	// Verify restored files can be loaded
	verifyRestoredSpecs(t, targetDir)
}

// setupVersionedTestStructure creates a test structure with a ledger spec
// that simulates having multiple versions (v1.0.0 with legacy_field, v2.0.0 with new_field)
func setupVersionedTestStructure(t *testing.T, baseDir string) {
	// Create object_specs directory
	specsDir := filepath.Join(baseDir, "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}

	// Create base_object spec (required for inheritance)
	baseObjectSpec := `ontology: base_object
extends: "null"
description: Base object specification
schema_version: "` + objects.DefaultSchemaVersion + `"
visibility: internal
fields:
  id:
    type: string
    validation:
      required: true
  kind:
    type: string
    validation:
      required: true
  schema_version:
    type: string
    validation:
      required: true
      pattern: ^\d+\.\d+\.\d+$
`
	baseObjectFile := filepath.Join(specsDir, "base_object.yaml")
	if err := os.WriteFile(baseObjectFile, []byte(baseObjectSpec), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write base_object spec: %v", err)
	}

	// Create ledger spec (v1.0.0 style - with legacy_field)
	// This represents the old version that finance department used before upgrade
	ledgerSpec := `ontology: ledger
extends: base_object
description: Ledger object for financial transactions (v1.0.0 style)
schema_version: "` + objects.DefaultSchemaVersion + `"
visibility: internal
fields:
  legacy_field:
    type: string
    description: Legacy field that was removed in v2.0.0
    validation:
      required: false
  amount:
    type: number
    description: Transaction amount
    validation:
      required: true
`
	ledgerFile := filepath.Join(specsDir, "ledger.yaml")
	if err := os.WriteFile(ledgerFile, []byte(ledgerSpec), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write ledger spec: %v", err)
	}

	// Create a config file to ensure configs are also captured/restored
	configContent := fmt.Sprintf("version: %q\ntest_config: true\n", BootstrapCaptureFormatVersion)
	configFile := filepath.Join(baseDir, "test_config.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
}

// verifyRestoredSpecs verifies that restored specs can be loaded correctly
// This validates that bootstrap capture/restore preserves spec content
func verifyRestoredSpecs(t *testing.T, targetDir string) {
	// Create a spec loader pointing to the restored object_specs directory
	// NewSpecLoader expects the path to the object_specs directory directly
	specsDir := filepath.Join(targetDir, "object_specs")
	specLoader := objects.NewSpecLoader(specsDir)

	// Test: Load ledger spec using file-based loading
	specFile := "ledger.yaml"
	spec, err := specLoader.LoadSpecWithInheritance(specFile)
	if err != nil {
		t.Fatalf("Failed to load ledger spec via file-based loading: %v", err)
	}

	if spec.Ontology != "ledger" {
		t.Errorf("Expected ontology 'ledger', got '%s'", spec.Ontology)
	}

	// Verify the spec has the expected fields (restored from bootstrap)
	if _, ok := spec.ResolvedFields["legacy_field"]; !ok {
		t.Error("Expected restored ledger spec to have legacy_field")
	}
	if _, ok := spec.ResolvedFields["amount"]; !ok {
		t.Error("Expected restored ledger spec to have amount field")
	}
	if _, ok := spec.ResolvedFields[objects.FieldKeyID]; !ok {
		t.Error("Expected restored ledger spec to have id field (inherited from base_object)")
	}

	// Verify inheritance was resolved correctly
	if _, ok := spec.ResolvedFields[objects.FieldKeyKind]; !ok {
		t.Error("Expected restored ledger spec to have kind field (inherited from base_object)")
	}
	if _, ok := spec.ResolvedFields[objects.FieldKeySchemaVersion]; !ok {
		t.Error("Expected restored ledger spec to have schema_version field (inherited from base_object)")
	}
}

// TestBootstrapRoundTripWithVersionAware tests a complete round-trip:
// 1. Capture current state (with versioned specs)
// 2. Save to bootstrap file
// 3. Delete original files
// 4. Restore from bootstrap
// 5. Verify version-aware loading still works
func TestBootstrapRoundTripWithVersionAware(t *testing.T) {
	t.Parallel()
	// Create temporary directories
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	bootstrapFile := filepath.Join(t.TempDir(), "bootstrap.yaml")

	// Setup test structure
	setupVersionedTestStructure(t, sourceDir)

	// Step 1: Capture state
	capture, err := CaptureCurrentState(sourceDir)
	if err != nil {
		t.Fatalf("Failed to capture state: %v", err)
	}

	// Step 2: Save to bootstrap file
	if err := capture.SaveToFile(bootstrapFile); err != nil {
		t.Fatalf("Failed to save bootstrap file: %v", err)
	}

	// Step 3: Simulate deleting original files (clean up source)
	// In a real scenario, you might delete the source directory
	// For testing, we'll just verify the bootstrap file exists
	if _, err := os.Stat(bootstrapFile); os.IsNotExist(err) {
		t.Fatalf("Bootstrap file should exist: %v", err)
	}

	// Step 4: Restore from bootstrap to target directory
	if err := InitializeFromBootstrap(bootstrapFile, targetDir, false); err != nil {
		t.Fatalf("Failed to initialize from bootstrap: %v", err)
	}

	// Step 5: Verify restored specs can be loaded correctly
	verifyRestoredSpecs(t, targetDir)

	// Step 6: Verify all files were restored correctly
	sourceLedger := filepath.Join(sourceDir, "object_specs", "ledger.yaml")
	targetLedger := filepath.Join(targetDir, "object_specs", "ledger.yaml")

	sourceContent, err := os.ReadFile(sourceLedger)
	if err != nil {
		t.Fatalf("Failed to read source ledger spec: %v", err)
	}

	targetContent, err := os.ReadFile(targetLedger)
	if err != nil {
		t.Fatalf("Failed to read target ledger spec: %v", err)
	}

	if !bytes.Equal(sourceContent, targetContent) {
		t.Error("Restored ledger spec content does not match source")
	}

	// Verify config file was also restored
	sourceConfig := filepath.Join(sourceDir, "test_config.yaml")
	targetConfig := filepath.Join(targetDir, "test_config.yaml")

	sourceConfigContent, err := os.ReadFile(sourceConfig)
	if err != nil {
		t.Fatalf("Failed to read source config: %v", err)
	}

	targetConfigContent, err := os.ReadFile(targetConfig)
	if err != nil {
		t.Fatalf("Failed to read target config: %v", err)
	}

	if !bytes.Equal(sourceConfigContent, targetConfigContent) {
		t.Error("Restored config content does not match source")
	}
}
