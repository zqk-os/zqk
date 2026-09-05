package detector

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const emptyValue = ""

// BinaryManifest represents a manifest of migration binary information
type BinaryManifest struct {
	Version       string                    `yaml:"version"`
	Algorithm     string                    `yaml:"algorithm"`               // e.g., "sha256"
	Compatibility *CompatibilityConstraints `yaml:"compatibility,omitempty"` // Compatibility requirements
	Entries       []BinaryManifestEntry     `yaml:"entries"`
}

// CompatibilityConstraints specifies version requirements for dependencies
type CompatibilityConstraints struct {
	// CLI version requirements
	CLIVersion string `yaml:"cli_version,omitempty"` // e.g., ">=1.0.0", "^1.0.0"

	// Module/dependency requirements
	Modules map[string]string `yaml:"modules,omitempty"` // module -> version constraint
	// Example: modules: { "github.com/lanceman/zqk/pkg/graph": ">=1.0.0" }

	// Backend requirements
	Backends map[string]string `yaml:"backends,omitempty"` // backend -> version constraint
	// Example: backends: { "memgraph": ">=2.0.0" }
}

// BinaryManifestEntry represents a platform-specific binary entry
type BinaryManifestEntry struct {
	Binary     string `yaml:"binary"`
	Version    string `yaml:"version"`
	Platform   string `yaml:"platform"` // e.g., "darwin/arm64"
	Hash       string `yaml:"hash"`
	Signature  string `yaml:"signature,omitempty"` // Optional: code signing signature info
	Source     string `yaml:"source"`
	VerifiedAt string `yaml:"verified_at"`

	// Entry-specific compatibility (overrides manifest-level if set)
	Compatibility *CompatibilityConstraints `yaml:"compatibility,omitempty"`
}

// LoadBinaryManifest loads the migration binary manifest
func LoadBinaryManifest(manifestPath string) (*BinaryManifest, error) {
	if manifestPath == emptyValue {
		manifestPath = filepath.Join(paths.ProjectDataDir, "migration-binary-manifest.yaml")
	}

	data, err := fileutil.ReadFile(manifestPath)
	if err != nil {
		return nil, errfmt.Newf("failed to read manifest").Wrap(err)
	}

	var manifest BinaryManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, errfmt.Newf("failed to parse manifest").Wrap(err)
	}

	return &manifest, nil
}

// SaveBinaryManifest saves the migration binary manifest
func SaveBinaryManifest(manifest *BinaryManifest, manifestPath string) error {
	if manifestPath == emptyValue {
		manifestPath = filepath.Join(paths.ProjectDataDir, "migration-binary-manifest.yaml")
	}

	// Ensure directory exists
	if err := fileutil.MkdirAll(filepath.Dir(manifestPath), paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create manifest directory").Wrap(err)
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return errfmt.Newf("failed to marshal manifest").Wrap(err)
	}

	return fileutil.WriteFile(manifestPath, data, paths.FilePerm644)
}

// GetEntryForPlatform gets the manifest entry for the current platform
func (m *BinaryManifest) GetEntryForPlatform(osName, arch string) *BinaryManifestEntry {
	platform := fmt.Sprintf("%s/%s", osName, arch)
	for i := range m.Entries {
		if m.Entries[i].Platform == platform {
			return &m.Entries[i]
		}
	}
	return nil
}

// UpdateEntry updates or adds an entry for a platform
func (m *BinaryManifest) UpdateEntry(entry *BinaryManifestEntry) {
	platform := entry.Platform
	for i := range m.Entries {
		if m.Entries[i].Platform == platform {
			m.Entries[i] = *entry
			return
		}
	}
	// Add new entry
	m.Entries = append(m.Entries, *entry)
}

// VerifyBinaryAgainstManifest verifies binary against manifest entry
func VerifyBinaryAgainstManifest(binaryPath string, manifest *BinaryManifest) error {
	entry := manifest.GetEntryForPlatform(runtime.GOOS, runtime.GOARCH)
	if entry == nil {
		return errfmt.Errorf("no manifest entry for platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Verify hash
	if err := VerifyBinaryIntegrity(binaryPath, entry.Hash); err != nil {
		return err
	}

	// Verify compatibility constraints
	compat := entry.Compatibility
	if compat == nil {
		compat = manifest.Compatibility
	}

	if compat != nil {
		if err := VerifyCompatibility(compat); err != nil {
			return errfmt.Newf("compatibility check failed").Wrap(err)
		}
	}

	return nil
}

// VerifyCompatibility verifies that the current environment meets compatibility requirements
func VerifyCompatibility(compat *CompatibilityConstraints) error {
	// Verify CLI version if specified
	if compat.CLIVersion != emptyValue {
		if err := VerifyCLIVersion(compat.CLIVersion); err != nil {
			return errfmt.Newf("CLI version requirement not met").Wrap(err)
		}
	}

	// Verify module versions if specified
	for module, constraint := range compat.Modules {
		if err := VerifyModuleVersion(module, constraint); err != nil {
			return errfmt.Errorf("module %s version requirement not met: %w", module, err)
		}
	}

	// Verify backend versions if specified
	for backend, constraint := range compat.Backends {
		if err := VerifyBackendVersion(backend, constraint); err != nil {
			return errfmt.Errorf("backend %s version requirement not met: %w", backend, err)
		}
	}

	return nil
}

// GetCLIVersion returns the current CLI version
// This should be set at build time via ldflags
var GetCLIVersion = func() string {
	// Default to "dev" if not set
	return "dev"
}

// VerifyCLIVersion verifies the CLI version meets the constraint
func VerifyCLIVersion(constraintStr string) error {
	cliVersion := GetCLIVersion()
	if cliVersion == "dev" {
		// In development mode, skip version check (or make it a warning)
		return nil
	}

	constraint, err := ParseVersionConstraint(constraintStr)
	if err != nil {
		return errfmt.Newf("invalid CLI version constraint").Wrap(err)
	}

	version, err := ParseVersion(cliVersion)
	if err != nil {
		return errfmt.Newf("failed to parse CLI version").Wrap(err)
	}

	if !constraint.Matches(version) {
		return errfmt.Errorf("CLI version %s does not meet requirement %s", cliVersion, constraintStr)
	}

	return nil
}

// VerifyModuleVersion verifies a module version meets the constraint
// Reads module version from go.mod and compares against constraint
func VerifyModuleVersion(module, constraintStr string) error {
	// Parse constraint
	constraint, err := ParseVersionConstraint(constraintStr)
	if err != nil {
		return errfmt.Newf("invalid module version constraint").Wrap(err)
	}

	// Get module version from go.mod
	moduleVersion, err := GetModuleVersion(module)
	if err != nil {
		return errfmt.Errorf("failed to get module version for %s: %w", module, err)
	}

	// Parse module version
	version, err := ParseVersion(moduleVersion)
	if err != nil {
		return errfmt.Errorf("failed to parse module version %s: %w", moduleVersion, err)
	}

	// Verify constraint
	if !constraint.Matches(version) {
		return errfmt.Errorf("module %s version %s does not meet requirement %s", module, moduleVersion, constraintStr)
	}

	return nil
}

// GetModuleVersion reads the version of a module from go.mod (found via findGoMod).
// Returns the version string (e.g., "1.0.0" without the "v" prefix).
// Tests may replace this to use a specific go.mod path (see GetModuleVersionFromFile).
var GetModuleVersion = func(modulePath string) (string, error) {
	goModPath := findGoMod()
	if goModPath == emptyValue {
		return "", errfmt.Errorf("go.mod not found")
	}
	return GetModuleVersionFromFile(goModPath, modulePath)
}

// GetModuleVersionFromFile reads the version of a module from the given go.mod file path.
// Used by tests to avoid relying on process working directory.
func GetModuleVersionFromFile(goModPath string, modulePath string) (string, error) {
	data, err := fileutil.ReadFile(goModPath)
	if err != nil {
		return "", errfmt.Newf("failed to read go.mod").Wrap(err)
	}
	return parseModuleVersionFromGoModContent(data, modulePath)
}

func parseModuleVersionFromGoModContent(data []byte, modulePath string) (string, error) {
	lines := strings.Split(string(data), "\n")
	inRequire := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] == modulePath {
				if len(parts) >= 3 {
					return strings.TrimSuffix(strings.TrimPrefix(parts[2], "v"), "+incompatible"), nil
				}
			}
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 1 && parts[0] == modulePath {
				if len(parts) >= 2 {
					version := strings.TrimPrefix(parts[1], "v")
					version = strings.TrimSuffix(version, "+incompatible")
					return version, nil
				}
			}
		}
	}

	return "", errfmt.Errorf("module %s not found in go.mod", modulePath)
}

// findGoMod searches for go.mod file starting from current directory
func findGoMod() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	// Search up to 10 directories
	for i := 0; i < 10; i++ {
		goModPath := filepath.Join(wd, "go.mod")
		if _, err := fileutil.Stat(goModPath); err == nil {
			return goModPath
		}
		// Move up one directory
		parent := filepath.Dir(wd)
		if parent == wd {
			break // Reached root
		}
		wd = parent
	}

	return ""
}

// VerifyBackendVersion verifies a backend version meets the constraint
func VerifyBackendVersion(backend, constraintStr string) error {
	// TODO: Implement backend version checking
	// This could query the backend for its version
	// For now, return nil (no-op) - can be implemented when needed
	return nil
}

// UpdateManifestAfterInstall updates manifest with new binary hash
func UpdateManifestAfterInstall(binaryPath, version, manifestPath string) error {
	// Calculate hash of installed binary
	hash, err := GetBinaryHash(binaryPath)
	if err != nil {
		return err
	}

	// Load or create manifest
	manifest, err := LoadBinaryManifest(manifestPath)
	if err != nil {
		manifest = &BinaryManifest{
			Version:   "1.0.0",
			Algorithm: "sha256",
			Entries:   []BinaryManifestEntry{},
		}
	}

	// Add/update entry for current platform
	entry := &BinaryManifestEntry{
		Binary:     "zqk-migrate",
		Version:    version,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Hash:       hash,
		Source:     "local_install",
		VerifiedAt: zqktime.NowRFC3339UTC(),
	}

	manifest.UpdateEntry(entry)

	// Save manifest
	return SaveBinaryManifest(manifest, manifestPath)
}
