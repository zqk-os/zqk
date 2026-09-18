package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestChangeLogReleaseNotes_FunctionalAcceptance verifies that CHANGELOG.md and RELEASE_NOTES.md
// are synthesized during packaging and contain release highlights (CRIT-1789706079947905000-27446a5f).
func TestChangeLogReleaseNotes_FunctionalAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("package-community.sh missing at %s", scriptPath)
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

	// 1. Verify CHANGELOG.md exists and contains version and highlights
	changelogPath := filepath.Join(distDir, "CHANGELOG.md")
	if !fileutil.Exists(changelogPath) {
		t.Fatalf("CHANGELOG.md missing in %s", distDir)
	}
	changelogBytes, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("failed to read CHANGELOG.md: %v", err)
	}
	changelogContent := string(changelogBytes)
	if !strings.Contains(changelogContent, "v0.0.0-test") {
		t.Errorf("expected version v0.0.0-test in CHANGELOG.md, got:\n%s", changelogContent)
	}
	if !strings.Contains(changelogContent, "Highlights") {
		t.Errorf("expected Highlights section in CHANGELOG.md")
	}

	// 2. Verify RELEASE_NOTES.md exists and contains installation instructions
	notesPath := filepath.Join(distDir, "RELEASE_NOTES.md")
	if !fileutil.Exists(notesPath) {
		t.Fatalf("RELEASE_NOTES.md missing in %s", distDir)
	}
	notesBytes, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("failed to read RELEASE_NOTES.md: %v", err)
	}
	notesContent := string(notesBytes)
	if !strings.Contains(notesContent, "brew tap lanceman/zqk") {
		t.Errorf("expected brew tap instructions in RELEASE_NOTES.md")
	}
	if !strings.Contains(notesContent, "checksums.txt") {
		t.Errorf("expected checksums.txt instructions in RELEASE_NOTES.md")
	}
}

// TestChangeLogReleaseNotes_BoundaryAndErrorHandling verifies that packaging fails-closed
// if required environment or arguments are invalid (CRIT-1789706079947906000-f2512688).
func TestChangeLogReleaseNotes_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("package-community.sh missing at %s", scriptPath)
	}

	// Test with invalid version string
	cmd := execwrap.Command("bash", scriptPath, "invalid-semver", "--dry")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("expected packaging to fail with invalid-semver, but succeeded:\n%s", string(out))
	}
}

// TestChangeLogReleaseNotes_IntegrationAndConformance verifies that tarball checksums
// and release notes match artifact outputs (CRIT-1789706079947907000-68fe754f).
func TestChangeLogReleaseNotes_IntegrationAndConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	testScript := filepath.Join(root, "scripts", "test_package_community.sh")
	if !fileutil.Exists(testScript) {
		t.Fatalf("scripts/test_package_community.sh missing at %s", testScript)
	}

	cmd := execwrap.Command("bash", testScript)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test_package_community.sh failed: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All expected artifacts and formula were successfully generated and verified") {
		t.Errorf("expected success verification in packaging output, got:\n%s", string(out))
	}
}
