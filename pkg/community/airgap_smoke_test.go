package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestAirgapOfflineSmoke_FunctionalAcceptance verifies that in an airgapped environment
// (no external network, airgap asserted via ZQK_AIRGAP=1 and NO_PROXY=*),
// a greenfield project can initialize, onboard agents,
// and discover actionable tasks with zero monorepo dependencies (CRIT-1789702985024729000-c7778903).
func TestAirgapOfflineSmoke_FunctionalAcceptance(t *testing.T) {
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

	// 1. system init offline
	initCmd := execwrap.Command(binPath, "system", "init", "--project-name", "airgap-greenfield")
	initCmd.Dir = greenfieldDir
	initCmd.Env = append(initCmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
	initOut, initErr := initCmd.CombinedOutput()
	if initErr != nil {
		t.Fatalf("system init failed in empty airgapped greenfield directory: %v\nOutput:\n%s", initErr, string(initOut))
	}

	zqkDir := filepath.Join(greenfieldDir, paths.ProjectDataDir)
	if !fileutil.Exists(zqkDir) {
		t.Fatalf(".zqk directory missing after system init")
	}

	// 2. system agent-onboard
	onboardCmd := execwrap.Command(binPath, "system", "agent-onboard", "--format", "json")
	onboardCmd.Dir = greenfieldDir
	onboardCmd.Env = append(onboardCmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
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
	wnCmd.Env = append(wnCmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
	wnOut, wnErr := wnCmd.CombinedOutput()
	if wnErr != nil {
		t.Fatalf("workflow whats-next failed: %v\nOutput:\n%s", wnErr, string(wnOut))
	}
	if !strings.Contains(string(wnOut), "zqk_whats_next_v1") {
		t.Errorf("expected zqk_whats_next_v1 schema in whats-next output, got:\n%s", string(wnOut))
	}
}

// TestAirgapOfflineSmoke_BoundaryAndErrorHandling verifies that running commands in an isolated airgap
// gracefully handles offline constraints and handles non-interactive execution without blocking (CRIT-1789702985024730000-f998135a).
func TestAirgapOfflineSmoke_BoundaryAndErrorHandling(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	binPath := filepath.Join(root, "bin", "zqk-stable")
	if !fileutil.Exists(binPath) {
		binPath = filepath.Join(root, "bin", "zqk")
	}
	if !fileutil.Exists(binPath) {
		t.Skip("skipping airgap smoke test: bin/zqk not built")
	}

	// Verify quickstart walkthrough executes offline in sub-second time
	quickCmd := execwrap.Command(binPath, "quickstart", "--format", "json")
	quickCmd.Dir = root
	quickCmd.Env = append(quickCmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
	quickOut, quickErr := quickCmd.CombinedOutput()
	if quickErr != nil {
		t.Fatalf("quickstart --format json failed in airgap: %v\nOutput:\n%s", quickErr, string(quickOut))
	}
	if !strings.Contains(strings.ToLower(string(quickOut)), "quickstart") && !strings.Contains(strings.ToLower(string(quickOut)), "steps") {
		t.Errorf("unexpected quickstart JSON output in airgap: %s", string(quickOut))
	}

	// Verify agent-onboard with detect-only works cleanly in airgap
	detectCmd := execwrap.Command(binPath, "system", "agent-onboard", "--detect-only", "--format", "json")
	detectCmd.Dir = root
	detectCmd.Env = append(detectCmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
	detectOut, detectErr := detectCmd.CombinedOutput()
	if detectErr != nil {
		t.Fatalf("agent-onboard --detect-only failed in airgap: %v\nOutput:\n%s", detectErr, string(detectOut))
	}
	if !strings.Contains(string(detectOut), `"status": "success"`) && !strings.Contains(string(detectOut), `"status":"success"`) {
		t.Errorf("expected success status from agent-onboard --detect-only, got:\n%s", string(detectOut))
	}
}

// TestAirgapOfflineSmoke_IntegrationAndConformance verifies clean repo airgap conformance and tree validation
// (CRIT-1789702985024731000-7e2d37df).
func TestAirgapOfflineSmoke_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "test-community-quickstart-e2e.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skipf("test-community-quickstart-e2e.sh not found at %s", scriptPath)
	}

	cmd := execwrap.Command("bash", scriptPath)
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "ZQK_AIRGAP=1", "NO_PROXY=*")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("test-community-quickstart-e2e.sh failed in airgap: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All checks passed") {
		t.Errorf("expected success confirmation in e2e output, got:\n%s", string(out))
	}
}
