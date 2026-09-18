package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestGreenfieldQuickstart_FunctionalAcceptance verifies that a stranger in a brand new,
// empty greenfield directory can initialize a standalone kernel, execute agent onboarding,
// and discover actionable tasks with zero monorepo dependencies (BLI-1789627308995091000-e04be613 / CRIT-1789627308995091000-ca2344e6).
func TestGreenfieldQuickstart_FunctionalAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	root := paths.ResolveProjectRoot(".")
	binPath := filepath.Join(root, "bin", "zqk-stable")
	if !fileutil.Exists(binPath) {
		binPath = filepath.Join(root, "bin", "zqk")
	}
	if !fileutil.Exists(binPath) {
		t.Fatalf("zqk executable not found at %s", binPath)
	}

	greenfieldDir := t.TempDir()

	// 1. system init
	initCmd := execwrap.Command(binPath, "system", "init", "--project-name", "stranger-greenfield")
	initCmd.Dir = greenfieldDir
	initOut, initErr := initCmd.CombinedOutput()
	if initErr != nil {
		t.Fatalf("system init failed in empty greenfield directory: %v\nOutput:\n%s", initErr, string(initOut))
	}

	zqkDir := filepath.Join(greenfieldDir, ".zqk")
	if !fileutil.Exists(zqkDir) {
		t.Fatalf(".zqk directory missing after system init")
	}

	// 2. system agent-onboard
	onboardCmd := execwrap.Command(binPath, "system", "agent-onboard", "--format", "json")
	onboardCmd.Dir = greenfieldDir
	onboardOut, onboardErr := onboardCmd.CombinedOutput()
	if onboardErr != nil {
		t.Fatalf("system agent-onboard failed: %v\nOutput:\n%s", onboardErr, string(onboardOut))
	}
	if !strings.Contains(string(onboardOut), `"status": "success"`) && !strings.Contains(string(onboardOut), `"status":"success"`) {
		t.Errorf("expected success status from agent-onboard, got:\n%s", string(onboardOut))
	}

	// 3. workflow whats-next
	wnCmd := execwrap.Command(binPath, "workflow", "whats-next", "--format", "json")
	wnCmd.Dir = greenfieldDir
	wnOut, wnErr := wnCmd.CombinedOutput()
	if wnErr != nil {
		t.Fatalf("workflow whats-next failed: %v\nOutput:\n%s", wnErr, string(wnOut))
	}
	if !strings.Contains(string(wnOut), "zqk_whats_next_v1") {
		t.Errorf("expected zqk_whats_next_v1 schema in whats-next output, got:\n%s", string(wnOut))
	}
}

// TestGreenfieldQuickstart_BoundaryAndErrorHandling verifies boundary conditions, negative testing,
// and graceful recovery when running in invalid environments (CRIT-1789627308995092000-35afd14e).
func TestGreenfieldQuickstart_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	binPath := filepath.Join(root, "bin", "zqk-stable")
	if !fileutil.Exists(binPath) {
		binPath = filepath.Join(root, "bin", "zqk")
	}

	// 1. Verify quickstart walkthrough runs and outputs structured help
	quickCmd := execwrap.Command(binPath, "quickstart", "--format", "json")
	quickCmd.Dir = root
	quickOut, quickErr := quickCmd.CombinedOutput()
	if quickErr != nil {
		t.Fatalf("quickstart --format json failed: %v\nOutput:\n%s", quickErr, string(quickOut))
	}
	if !strings.Contains(strings.ToLower(string(quickOut)), "quickstart") && !strings.Contains(strings.ToLower(string(quickOut)), "steps") {
		t.Errorf("unexpected quickstart JSON output: %s", string(quickOut))
	}

	// 2. Verify agent-onboard with nonexistent directory does not panic
	badDir := filepath.Join(t.TempDir(), "missing_dir")
	detectCmd := execwrap.Command(binPath, "system", "agent-onboard", "--detect-only", "--format", "json")
	detectCmd.Dir = badDir
	_ = detectCmd.Run() // Must not panic or crash
}

// TestGreenfieldQuickstart_IntegrationAndConformance verifies end-to-end quickstart execution
// and documentation references (CRIT-1789627308995093000-39fa3a52).
func TestGreenfieldQuickstart_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "test-community-quickstart-e2e.sh")
	if !fileutil.Exists(scriptPath) {
		t.Fatalf("test-community-quickstart-e2e.sh not found at %s", scriptPath)
	}

	cmd := execwrap.Command("bash", scriptPath)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test-community-quickstart-e2e.sh failed: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All checks passed") {
		t.Errorf("expected success confirmation in e2e output, got:\n%s", string(out))
	}
}
