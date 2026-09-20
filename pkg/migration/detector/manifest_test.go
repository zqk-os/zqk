package detector

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCompatibilityConstraints_CLIVersion(t *testing.T) {
	// Save original GetCLIVersion
	originalGetCLIVersion := GetCLIVersion
	defer func() {
		GetCLIVersion = originalGetCLIVersion
	}()

	tests := []struct {
		name        string
		cliVersion  string
		constraint  string
		expectError bool
	}{
		{
			name:        "CLI version meets minimum requirement",
			cliVersion:  "1.0.0",
			constraint:  ">=1.0.0",
			expectError: false,
		},
		{
			name:        "CLI version below minimum requirement",
			cliVersion:  "0.9.0",
			constraint:  ">=1.0.0",
			expectError: true,
		},
		{
			name:        "CLI version in compatible range",
			cliVersion:  "1.5.0",
			constraint:  "^1.0.0",
			expectError: false,
		},
		{
			name:        "CLI version outside compatible range",
			cliVersion:  objects.DefaultSchemaVersion,
			constraint:  "^1.0.0",
			expectError: true,
		},
		{
			name:        "CLI version in range",
			cliVersion:  "1.5.0",
			constraint:  ">=1.0.0 <" + objects.DefaultSchemaVersion,
			expectError: false,
		},
		{
			name:        "CLI version outside range (too high)",
			cliVersion:  objects.DefaultSchemaVersion,
			constraint:  ">=1.0.0 <" + objects.DefaultSchemaVersion,
			expectError: true,
		},
		{
			name:        "CLI version outside range (too low)",
			cliVersion:  "0.9.0",
			constraint:  ">=1.0.0 <" + objects.DefaultSchemaVersion,
			expectError: true,
		},
		{
			name:        "Dev version skips check",
			cliVersion:  "dev",
			constraint:  ">=1.0.0",
			expectError: false, // Dev version should skip check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set CLI version for test
			GetCLIVersion = func() string {
				return tt.cliVersion
			}

			compat := &CompatibilityConstraints{
				CLIVersion: tt.constraint,
			}

			err := VerifyCompatibility(compat)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for CLI version %s with constraint %s", tt.cliVersion, tt.constraint)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for CLI version %s with constraint %s: %v", tt.cliVersion, tt.constraint, err)
				}
			}
		})
	}
}

func TestCompatibilityConstraints_InvalidConstraint(t *testing.T) {
	// Save original GetCLIVersion
	originalGetCLIVersion := GetCLIVersion
	defer func() {
		GetCLIVersion = originalGetCLIVersion
	}()

	GetCLIVersion = func() string {
		return "1.0.0"
	}

	compat := &CompatibilityConstraints{
		CLIVersion: "invalid-constraint",
	}

	err := VerifyCompatibility(compat)
	if err == nil {
		t.Error("Expected error for invalid constraint")
	}
}

func TestBinaryManifest_CompatibilityPrecedence(t *testing.T) {
	// Save original GetCLIVersion
	originalGetCLIVersion := GetCLIVersion
	defer func() {
		GetCLIVersion = originalGetCLIVersion
	}()

	GetCLIVersion = func() string {
		return "1.5.0"
	}

	// Test that entry-level compatibility overrides manifest-level
	manifest := &BinaryManifest{
		Version:   "1.0.0",
		Algorithm: "sha256",
		Compatibility: &CompatibilityConstraints{
			CLIVersion: ">=1.0.0", // Manifest-level: accepts 1.5.0
		},
		Entries: []BinaryManifestEntry{
			{
				Platform: "test/arch",
				Compatibility: &CompatibilityConstraints{
					CLIVersion: ">=" + objects.DefaultSchemaVersion, // Entry-level: requires default schema floor+
				},
			},
		},
	}

	// Entry-level constraint should be used (and fail)
	entry := manifest.GetEntryForPlatform("test", "arch")
	if entry == nil {
		t.Fatal("Expected to find entry")
	}

	compat := entry.Compatibility
	if compat == nil {
		compat = manifest.Compatibility
	}

	// Re-assert mock before first check (parallel tests can overwrite GetCLIVersion)
	GetCLIVersion = func() string { return "1.5.0" }
	err := VerifyCompatibility(compat)
	if err == nil {
		t.Errorf("Expected error: entry-level constraint requires >=%s but CLI is 1.5.0", objects.DefaultSchemaVersion)
	}

	// Test manifest-level fallback
	manifest2 := &BinaryManifest{
		Version:   "1.0.0",
		Algorithm: "sha256",
		Compatibility: &CompatibilityConstraints{
			CLIVersion: ">=1.0.0", // Manifest-level: accepts 1.5.0
		},
		Entries: []BinaryManifestEntry{
			{
				Platform: "test/arch",
				// No entry-level compatibility, should use manifest-level
			},
		},
	}

	entry2 := manifest2.GetEntryForPlatform("test", "arch")
	if entry2 == nil {
		t.Fatal("Expected to find entry")
	}

	compat2 := entry2.Compatibility
	if compat2 == nil {
		compat2 = manifest2.Compatibility
	}

	// Re-assert mock before second check (parallel tests can overwrite GetCLIVersion)
	GetCLIVersion = func() string { return "1.5.0" }
	err2 := VerifyCompatibility(compat2)
	if err2 != nil {
		t.Errorf("Unexpected error: manifest-level constraint should accept 1.5.0: %v", err2)
	}
}

func TestLoadBinaryManifest_WithCompatibility(t *testing.T) {
	// Create a temporary manifest file with compatibility constraints
	testDir := t.TempDir()
	manifestPath := filepath.Join(testDir, "manifest.yaml")

	manifestYAML := fmt.Sprintf(`version: "1.0.0"
algorithm: "sha256"
compatibility:
  cli_version: ">=1.0.0"
  modules:
    "github.com/zqk-os/zqk/pkg/graph": "^1.0.0"
  backends:
    "memgraph": ">=%s"
entries:
  - binary: "zqk-migrate"
    version: "1.0.0"
    platform: "test/arch"
    hash: "abc123"
    source: "test"
    verified_at: "2025-12-24T10:00:00Z"
`, objects.DefaultSchemaVersion)

	if err := fileutil.WriteFile(manifestPath, []byte(manifestYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	manifest, err := LoadBinaryManifest(manifestPath)
	if err != nil {
		t.Fatalf("Failed to load manifest: %v", err)
	}

	// Verify compatibility constraints were loaded
	if manifest.Compatibility == nil {
		t.Fatal("Expected compatibility constraints to be loaded")
	}

	if manifest.Compatibility.CLIVersion != ">=1.0.0" {
		t.Errorf("Expected CLI version constraint '>=1.0.0', got '%s'", manifest.Compatibility.CLIVersion)
	}

	if len(manifest.Compatibility.Modules) != 1 {
		t.Errorf("Expected 1 module constraint, got %d", len(manifest.Compatibility.Modules))
	}

	if manifest.Compatibility.Modules["github.com/zqk-os/zqk/pkg/graph"] != "^1.0.0" {
		t.Errorf("Expected module constraint '^1.0.0', got '%s'", manifest.Compatibility.Modules["github.com/zqk-os/zqk/pkg/graph"])
	}

	if len(manifest.Compatibility.Backends) != 1 {
		t.Errorf("Expected 1 backend constraint, got %d", len(manifest.Compatibility.Backends))
	}

	wantBackend := ">=" + objects.DefaultSchemaVersion
	if manifest.Compatibility.Backends["memgraph"] != wantBackend {
		t.Errorf("Expected backend constraint %q, got '%s'", wantBackend, manifest.Compatibility.Backends["memgraph"])
	}
}

func TestLoadBinaryManifest_WithEntryLevelCompatibility(t *testing.T) {
	// Create a temporary manifest file with entry-level compatibility
	testDir := t.TempDir()
	manifestPath := filepath.Join(testDir, "manifest.yaml")

	manifestYAML := fmt.Sprintf(`version: "1.0.0"
algorithm: "sha256"
compatibility:
  cli_version: ">=1.0.0"
entries:
  - binary: "zqk-migrate"
    version: "1.0.0"
    platform: "test/arch"
    hash: "abc123"
    source: "test"
    verified_at: "2025-12-24T10:00:00Z"
    compatibility:
      cli_version: ">=%s"
`, objects.DefaultSchemaVersion)

	if err := fileutil.WriteFile(manifestPath, []byte(manifestYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	manifest, err := LoadBinaryManifest(manifestPath)
	if err != nil {
		t.Fatalf("Failed to load manifest: %v", err)
	}

	entry := manifest.GetEntryForPlatform("test", "arch")
	if entry == nil {
		t.Fatal("Expected to find entry")
	}

	// Entry should have its own compatibility constraint
	if entry.Compatibility == nil {
		t.Fatal("Expected entry to have compatibility constraints")
	}

	wantEntryCLI := ">=" + objects.DefaultSchemaVersion
	if entry.Compatibility.CLIVersion != wantEntryCLI {
		t.Errorf("Expected entry CLI version constraint %q, got '%s'", wantEntryCLI, entry.Compatibility.CLIVersion)
	}
}

func TestVerifyBinaryAgainstManifest_CompatibilityCheck(t *testing.T) {
	// Save original GetCLIVersion
	originalGetCLIVersion := GetCLIVersion
	defer func() {
		GetCLIVersion = originalGetCLIVersion
	}()

	GetCLIVersion = func() string {
		return "1.5.0"
	}

	// Create a test binary file
	testDir := t.TempDir()
	testBinary := filepath.Join(testDir, "test.bin")
	testData := []byte("test binary data")
	if err := fileutil.WriteFile(testBinary, testData, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	// Get hash
	hash, err := GetBinaryHash(testBinary)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}

	// Use actual runtime platform
	platform := runtime.GOOS + "/" + runtime.GOARCH

	// Create manifest with compatibility constraint
	manifest := &BinaryManifest{
		Version:   "1.0.0",
		Algorithm: "sha256",
		Compatibility: &CompatibilityConstraints{
			CLIVersion: ">=1.0.0", // Should pass with CLI 1.5.0
		},
		Entries: []BinaryManifestEntry{
			{
				Binary:     "test.bin",
				Version:    "1.0.0",
				Platform:   platform,
				Hash:       hash,
				Source:     "test",
				VerifiedAt: "2025-12-24T10:00:00Z",
			},
		},
	}

	// Should succeed - CLI version meets requirement
	err = VerifyBinaryAgainstManifest(testBinary, manifest)
	if err != nil {
		t.Errorf("Expected verification to succeed, got error: %v", err)
	}

	// Test with incompatible CLI version
	GetCLIVersion = func() string {
		return "0.9.0"
	}

	err = VerifyBinaryAgainstManifest(testBinary, manifest)
	if err == nil {
		t.Error("Expected verification to fail with incompatible CLI version")
	}
	if err != nil && !contains(err.Error(), "compatibility check failed") {
		t.Errorf("Expected compatibility error, got: %v", err)
	}
}

func TestVerifyCompatibility_ModulesAndBackends(t *testing.T) {
	compat := &CompatibilityConstraints{
		CLIVersion: ">=1.0.0",
		Modules: map[string]string{
			"github.com/spf13/cobra": ">=1.0.0", // Real module from project go.mod when run from repo root
		},
		Backends: map[string]string{
			"test-backend": ">=1.0.0",
		},
	}

	originalGetCLIVersion := GetCLIVersion
	defer func() {
		GetCLIVersion = originalGetCLIVersion
	}()
	GetCLIVersion = func() string { return "1.0.0" }

	err := VerifyCompatibility(compat)
	if err != nil {
		// Skip when go.mod isn't found or module not in go.mod (e.g. test run from package dir or different cwd)
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "go.mod") {
			t.Skipf("Skipping module verification (go.mod context): %v", err)
		}
		t.Errorf("Unexpected error: %v", err)
	}

	// Test with incompatible module version
	compat2 := &CompatibilityConstraints{
		CLIVersion: ">=1.0.0",
		Modules: map[string]string{
			"github.com/spf13/cobra": ">=2.0.0", // Version too high
		},
	}

	err = VerifyCompatibility(compat2)
	if err == nil {
		t.Error("Expected error for incompatible module version")
	}
}

// Helper function to check if error message contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == emptyValue ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
