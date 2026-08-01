package detector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestGetModuleVersion(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	goModPath := filepath.Join(testDir, "go.mod")

	goModContent := `module github.com/test/project

go 1.24.0

require (
	github.com/neo4j/neo4j-go-driver/v5 v5.28.4
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)
`

	if err := os.WriteFile(goModPath, []byte(goModContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	tests := []struct {
		name        string
		module      string
		expected    string
		expectError bool
	}{
		{
			name:        "Direct dependency",
			module:      "github.com/neo4j/neo4j-go-driver/v5",
			expected:    "5.28.4",
			expectError: false,
		},
		{
			name:        "Another direct dependency",
			module:      "github.com/spf13/cobra",
			expected:    "1.10.2",
			expectError: false,
		},
		{
			name:        "Indirect dependency",
			module:      "github.com/inconshreveable/mousetrap",
			expected:    "1.1.0",
			expectError: false,
		},
		{
			name:        "Module not found",
			module:      "github.com/nonexistent/module",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := GetModuleVersionFromFile(goModPath, tt.module)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for module %s", tt.module)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if version != tt.expected {
					t.Errorf("Expected version %s, got %s", tt.expected, version)
				}
			}
		})
	}
}

func TestVerifyModuleVersion(t *testing.T) {
	// Do not t.Parallel: mutates package-global GetModuleVersion; parallel subtests race each other.
	testDir := t.TempDir()
	goModPath := filepath.Join(testDir, "go.mod")

	goModContent := `module github.com/test/project

go 1.24.0

require (
	github.com/neo4j/neo4j-go-driver/v5 v5.28.4
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)
`

	if err := os.WriteFile(goModPath, []byte(goModContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	originalGetModuleVersion := GetModuleVersion
	defer func() { GetModuleVersion = originalGetModuleVersion }()
	GetModuleVersion = func(module string) (string, error) {
		return GetModuleVersionFromFile(goModPath, module)
	}

	tests := []struct {
		name        string
		module      string
		constraint  string
		expectError bool
	}{
		{
			name:        "Version meets minimum requirement",
			module:      "github.com/neo4j/neo4j-go-driver/v5",
			constraint:  ">=5.0.0",
			expectError: false,
		},
		{
			name:        "Version below minimum requirement",
			module:      "github.com/neo4j/neo4j-go-driver/v5",
			constraint:  ">=6.0.0",
			expectError: true,
		},
		{
			name:        "Version in compatible range",
			module:      "github.com/spf13/cobra",
			constraint:  "^1.0.0",
			expectError: false,
		},
		{
			name:        "Version outside compatible range",
			module:      "github.com/spf13/cobra",
			constraint:  "^2.0.0",
			expectError: true,
		},
		{
			name:        "Version in range",
			module:      "gopkg.in/yaml.v3",
			constraint:  ">=3.0.0 <4.0.0",
			expectError: false,
		},
		{
			name:        "Version outside range",
			module:      "gopkg.in/yaml.v3",
			constraint:  ">=4.0.0",
			expectError: true,
		},
		{
			name:        "Exact version match",
			module:      "github.com/spf13/cobra",
			constraint:  "1.10.2",
			expectError: false,
		},
		{
			name:        "Exact version mismatch",
			module:      "github.com/spf13/cobra",
			constraint:  "1.10.1",
			expectError: true,
		},
		{
			name:        "Module not found",
			module:      "github.com/nonexistent/module",
			constraint:  ">=1.0.0",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyModuleVersion(tt.module, tt.constraint)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for module %s with constraint %s", tt.module, tt.constraint)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestVerifyModuleVersion_ErrorMessages(t *testing.T) {
	// Do not t.Parallel: mutates package-global GetModuleVersion.
	// Create a temporary go.mod file
	testDir := t.TempDir()
	goModPath := filepath.Join(testDir, "go.mod")

	goModContent := `module github.com/test/project

go 1.24.0

require (
	github.com/test/module v1.0.0
)
`

	if err := os.WriteFile(goModPath, []byte(goModContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	originalGetModuleVersion := GetModuleVersion
	defer func() { GetModuleVersion = originalGetModuleVersion }()
	GetModuleVersion = func(module string) (string, error) {
		return GetModuleVersionFromFile(goModPath, module)
	}

	// Test error message includes module path (version 1.0.0 does not meet >=2.0.0)
	err := VerifyModuleVersion("github.com/test/module", ">=2.0.0")
	if err == nil {
		t.Fatal("Expected error")
	}

	errorMsg := err.Error()
	if !contains(errorMsg, "github.com/test/module") {
		t.Errorf("Error message should include module path, got: %s", errorMsg)
	}

	// Test error message includes actual version
	if !contains(errorMsg, "1.0.0") {
		t.Errorf("Error message should include actual version, got: %s", errorMsg)
	}

	// Test error message includes constraint
	if !contains(errorMsg, ">=2.0.0") {
		t.Errorf("Error message should include constraint, got: %s", errorMsg)
	}
}

func TestFindGoMod(t *testing.T) {
	t.Parallel()
	// Create nested directory structure
	testDir := t.TempDir()
	subDir := filepath.Join(testDir, "sub", "dir")
	if err := os.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create go.mod in root
	goModPath := filepath.Join(testDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Change to subdirectory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Should find go.mod in parent directory
	found := findGoMod()
	if found == emptyValue {
		t.Error("Expected to find go.mod, got empty string")
	}
	// Compare using filepath.EvalSymlinks to handle macOS /var -> /private/var symlink
	//nolint:errcheck // Test helper - error acceptable
	foundResolved, _ := filepath.EvalSymlinks(found)
	//nolint:errcheck // Test helper - error acceptable
	expectedResolved, _ := filepath.EvalSymlinks(goModPath)
	if foundResolved != expectedResolved {
		t.Errorf("Expected to find go.mod at %s (resolved: %s), got %s (resolved: %s)", goModPath, expectedResolved, found, foundResolved)
	}
}
