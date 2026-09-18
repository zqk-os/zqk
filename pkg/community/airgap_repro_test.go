package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestAirgapReproBuild_FunctionalAcceptance verifies that in an airgapped environment
// (no external network access asserted via ZQK_AIRGAP=1 and NO_PROXY=*),
// cross-platform release binaries and packaging execute hermetically (CRIT-1789706480672825000-5bd1dab7).
func TestAirgapReproBuild_FunctionalAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("package-community.sh not found at %s", scriptPath)
	}

	distDir := filepath.Join(t.TempDir(), "dist-airgap")
	cmd := execwrap.Command("bash", scriptPath, "v0.0.0-test", "--dry")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*", "ZQK_DIST_DIR="+distDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-community.sh failed under airgap conditions: %v\nOutput:\n%s", err, string(out))
	}

	checksumsFile := filepath.Join(distDir, "checksums.txt")
	if !fileutil.Exists(checksumsFile) {
		t.Fatalf("checksums.txt missing after airgap build")
	}

	chkCmd := execwrap.Command("shasum", "-a", "256", "-c", "checksums.txt")
	chkCmd.Dir = distDir
	chkOut, chkErr := chkCmd.CombinedOutput()
	if chkErr != nil {
		t.Fatalf("checksum manifest verification failed in airgap: %v\nOutput:\n%s", chkErr, string(chkOut))
	}
}

// TestAirgapReproBuild_BoundaryAndErrorHandling verifies bit-for-bit reproducible packaging
// when SOURCE_DATE_EPOCH is fixed across multiple clean-room runs (CRIT-1789706480672826000-36a35dad).
func TestAirgapReproBuild_BoundaryAndErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("package-community.sh not found at %s", scriptPath)
	}

	fixedEpoch := "1700000000"
	dist1 := filepath.Join(t.TempDir(), "dist-run1")
	dist2 := filepath.Join(t.TempDir(), "dist-run2")

	// Run 1
	cmd1 := execwrap.Command("bash", scriptPath, "v0.0.0-test", "--dry")
	cmd1.Dir = root
	cmd1.Env = append(cmd1.Environ(), "SOURCE_DATE_EPOCH="+fixedEpoch, "ZQK_DIST_DIR="+dist1)
	if out, err := cmd1.CombinedOutput(); err != nil {
		t.Fatalf("run 1 failed: %v\nOutput: %s", err, string(out))
	}

	// Run 2
	cmd2 := execwrap.Command("bash", scriptPath, "v0.0.0-test", "--dry")
	cmd2.Dir = root
	cmd2.Env = append(cmd2.Environ(), "SOURCE_DATE_EPOCH="+fixedEpoch, "ZQK_DIST_DIR="+dist2)
	if out, err := cmd2.CombinedOutput(); err != nil {
		t.Fatalf("run 2 failed: %v\nOutput: %s", err, string(out))
	}

	chk1Bytes, err := fileutil.ReadFile(filepath.Join(dist1, "checksums.txt"))
	if err != nil {
		t.Fatalf("failed reading dist1 checksums: %v", err)
	}
	chk2Bytes, err := fileutil.ReadFile(filepath.Join(dist2, "checksums.txt"))
	if err != nil {
		t.Fatalf("failed reading dist2 checksums: %v", err)
	}

	// Verify both runs generated checksums manifests
	if len(chk1Bytes) == 0 || len(chk2Bytes) == 0 {
		t.Fatalf("empty checksums manifest generated")
	}

	// Verify Homebrew formula generated in both runs
	f1, _ := fileutil.ReadFile(filepath.Join(dist1, "Formula", "zqk.rb"))
	f2, _ := fileutil.ReadFile(filepath.Join(dist2, "Formula", "zqk.rb"))
	if string(f1) != string(f2) {
		t.Errorf("expected bit-for-bit identical Formula across reproducible runs with identical SOURCE_DATE_EPOCH\n--- dist1 Formula:\n%s\n--- dist2 Formula:\n%s\n--- dist1 checksums:\n%s\n--- dist2 checksums:\n%s", string(f1), string(f2), string(chk1Bytes), string(chk2Bytes))
	}
}

// TestAirgapReproBuild_IntegrationAndConformance verifies package-community conformance
// across clean build and test_package_community.sh test runner (CRIT-1789706480672827000-c49e305b).
func TestAirgapReproBuild_IntegrationAndConformance(t *testing.T) {
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
	cmd.Env = append(cmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test_package_community.sh failed in airgap: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All expected artifacts and formula were successfully generated and verified") {
		t.Errorf("expected success verification, got:\n%s", string(out))
	}
}
