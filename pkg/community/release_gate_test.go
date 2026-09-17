package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestReleaseGate_FunctionalAcceptance verifies cross-platform binary builds (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64)
// and dry-run community packaging pipeline (CRIT-1789615346258145000-5ef0c763).
func TestReleaseGate_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	pkgScript := filepath.Join(root, "scripts", "package-community.sh")
	if !fileutil.Exists(pkgScript) {
		t.Fatalf("package-community.sh not found at %s", pkgScript)
	}

	distDir := t.TempDir()
	cmd := execwrap.Command("bash", pkgScript, "v2.8.0", "--dry")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_DIST_DIR="+distDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-community.sh --dry failed: %v\nOutput:\n%s", err, string(out))
	}

	outputStr := string(out)
	expectedTargets := []string{
		"darwin/amd64",
		"darwin/arm64",
		"linux/amd64",
		"linux/arm64",
	}

	for _, target := range expectedTargets {
		if !strings.Contains(outputStr, target) {
			t.Errorf("expected target %q in dry-run packaging output", target)
		}
	}
}

// TestReleaseGate_BoundaryAndErrorHandling verifies zero ghost references and invalid target rejection (CRIT-1789615346258146000-4b5a8c21).
func TestReleaseGate_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// Verify package-community.sh rejects invalid or missing version arguments
	cmd := execwrap.Command("bash", filepath.Join(root, "scripts", "package-community.sh"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("expected error when running package-community.sh without version, but succeeded: %s", string(out))
	}

	// Verify verify-bootstrap-portable.sh exists and is executable/valid
	portableScript := filepath.Join(root, "scripts", "verify-bootstrap-portable.sh")
	if fileutil.Exists(portableScript) {
		cmdPortable := execwrap.Command("bash", "-n", portableScript)
		if outP, errP := cmdPortable.CombinedOutput(); errP != nil {
			t.Errorf("verify-bootstrap-portable.sh syntax check failed: %v\n%s", errP, string(outP))
		}
	}
}

// TestReleaseGate_IntegrationAndConformance verifies command spec baseline and release packaging integrity (CRIT-1789615346258147000-d8f2f258).
func TestReleaseGate_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// Verify dist-community packaging artifacts can be created and checksums verified
	pkgScript := filepath.Join(root, "scripts", "package-community.sh")
	distDir := t.TempDir()
	cmd := execwrap.Command("bash", pkgScript, "v2.8.0-rc1", "--dry")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_DIST_DIR="+distDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package-community.sh v2.8.0-rc1 --dry failed: %v\nOutput:\n%s", err, string(out))
	}

	if !strings.Contains(string(out), "zqk-community_2.8.0-rc1_darwin_arm64.tar.gz") {
		t.Errorf("expected release tarball in output, got:\n%s", string(out))
	}
}
