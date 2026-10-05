package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestReleaseTagDistribution_FunctionalAcceptance verifies automated release tagging,
// cross-platform artifact packaging, manifest generation, and distribution readiness
// for Community Edition (CRIT-1789704096616271000-b268b57b).
func TestReleaseTagDistribution_FunctionalAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("package-community.sh not found at %s", scriptPath)
	}

	distDir := filepath.Join(t.TempDir(), "dist-community")
	testVersion := "v0.0.0-test"

	cmd := execwrap.Command("bash", scriptPath, testVersion, "--dry")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_DIST_DIR="+distDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-community.sh --dry failed: %v\nOutput:\n%s", err, string(out))
	}

	// Verify all expected platform archives exist
	expectedArchives := []string{
		"zqk-community_0.0.0-test_darwin_amd64.tar.gz",
		"zqk-community_0.0.0-test_darwin_arm64.tar.gz",
		"zqk-community_0.0.0-test_linux_amd64.tar.gz",
		"zqk-community_0.0.0-test_linux_arm64.tar.gz",
	}

	for _, archive := range expectedArchives {
		archivePath := filepath.Join(distDir, archive)
		if !fileutil.Exists(archivePath) {
			t.Fatalf("missing expected release archive: %s", archivePath)
		}
	}

	// Verify checksums.txt exists and matches files
	checksumPath := filepath.Join(distDir, "checksums.txt")
	if !fileutil.Exists(checksumPath) {
		t.Fatalf("checksums.txt missing in %s", distDir)
	}

	chkCmd := execwrap.Command("shasum", "-a", "256", "-c", "checksums.txt")
	chkCmd.Dir = distDir
	chkOut, chkErr := chkCmd.CombinedOutput()
	if chkErr != nil {
		t.Fatalf("shasum checksum verification failed: %v\nOutput:\n%s", chkErr, string(chkOut))
	}

	// Verify Homebrew formula generated in dist
	formulaPath := filepath.Join(distDir, "Formula", "zqk.rb")
	if !fileutil.Exists(formulaPath) {
		t.Fatalf("Formula/zqk.rb missing in %s", distDir)
	}
	formulaBytes, err := fileutil.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("failed reading formula: %v", err)
	}
	formulaContent := string(formulaBytes)
	if !strings.Contains(formulaContent, `version "0.0.0-test"`) {
		t.Errorf("Formula does not contain expected version 0.0.0-test:\n%s", formulaContent)
	}
}

// TestReleaseTagDistribution_BoundaryAndErrorHandling verifies semver validation,
// invalid tag format rejection, and fail-closed behavior on missing or corrupted arguments
// (CRIT-1789704096616272000-3972951f).
func TestReleaseTagDistribution_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("package-community.sh not found at %s", scriptPath)
	}

	invalidVersions := []string{
		"invalid-tag",
		"2.7.0",     // missing 'v' prefix
		"v1",        // incomplete semver
		"v1.2",      // incomplete semver
		"v1.2.3.4",  // excess components
		"v1.0.0;rm", // command injection pattern
		"",          // empty
	}

	for _, invalid := range invalidVersions {
		args := []string{scriptPath}
		if invalid != "" {
			args = append(args, invalid, "--dry")
		}
		cmd := execwrap.Command("bash", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("expected error for invalid tag '%s', but command succeeded:\n%s", invalid, string(out))
		}
	}
}

// TestReleaseTagDistribution_IntegrationAndConformance verifies end-to-end conformance
// by running scripts/test_package_community.sh and asserting Homebrew formula SHA conformance
// (CRIT-1789704096616273000-33972037).
func TestReleaseTagDistribution_IntegrationAndConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	testScript := filepath.Join(root, "scripts", "test_package_community.sh")
	if !fileutil.Exists(testScript) {
		t.Skipf("scripts/test_package_community.sh not found at %s (studio-only artifact)", testScript)
	}

	cmd := execwrap.Command("bash", testScript)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test_package_community.sh failed: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All expected artifacts and formula were successfully generated and verified") {
		t.Errorf("expected success message in script output, got:\n%s", string(out))
	}
}
