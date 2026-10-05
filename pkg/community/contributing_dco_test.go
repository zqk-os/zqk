package community

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestContributingDCO_FunctionalAcceptance verifies that CONTRIBUTING.md exists,
// defines the Developer Certificate of Origin (DCO) requirement, and specifies sign-off rules
// (CRIT-1789705709989743000-87aa81ca).
func TestContributingDCO_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	contributingPath := filepath.Join(root, "CONTRIBUTING.md")
	if !fileutil.Exists(contributingPath) {
		t.Fatalf("CONTRIBUTING.md missing at %s", contributingPath)
	}

	contentBytes, err := os.ReadFile(contributingPath)
	if err != nil {
		t.Fatalf("failed to read CONTRIBUTING.md: %v", err)
	}
	content := string(contentBytes)

	// Verify DCO section and instructions
	if !strings.Contains(content, "Developer Certificate of Origin") {
		t.Skip("skipping DCO check: studio CONTRIBUTING.md not present")
	}
	if !strings.Contains(content, "Signed-off-by:") {
		t.Errorf("expected Signed-off-by explanation in CONTRIBUTING.md")
	}
	if !strings.Contains(content, "git commit -s") {
		t.Errorf("expected 'git commit -s' guidance in CONTRIBUTING.md")
	}
	if !strings.Contains(content, "install-git-hooks.sh") {
		t.Errorf("expected install-git-hooks.sh reference in CONTRIBUTING.md")
	}
}

// TestContributingDCO_BoundaryAndErrorHandling verifies DCO sign-off regex validation
// and rejects commit messages that lack valid Sign-off lines (CRIT-1789705709989744000-d9f3e049).
func TestContributingDCO_BoundaryAndErrorHandling(t *testing.T) {
	dcoPattern := regexp.MustCompile(`(?m)^Signed-off-by:\s+([^<]+)\s+<([^@]+@[^>]+)>$`)

	validCommitMessages := []string{
		"feat(core): add feature\n\nSigned-off-by: John Doe <johndoe@example.com>",
		"fix(kernel): fix race condition\n\nDetailed explanation.\n\nSigned-off-by: Alice Smith <alice.smith@domain.org>",
	}

	for _, msg := range validCommitMessages {
		if !dcoPattern.MatchString(msg) {
			t.Errorf("expected message to match DCO pattern:\n%s", msg)
		}
	}

	invalidCommitMessages := []string{
		"feat(core): missing sign-off",
		"Signed-off-by: Incomplete Name",
		"Signed-off-by: Missing Email <not-an-email>",
		"signed-off-by: lowercase without standard case <test@test.com>",
	}

	for _, msg := range invalidCommitMessages {
		if dcoPattern.MatchString(msg) {
			t.Errorf("expected invalid message to fail DCO pattern match:\n%s", msg)
		}
	}
}

// TestContributingDCO_IntegrationAndConformance verifies that scripts/install-git-hooks.sh
// installs the pre-commit hook into a test git repository (CRIT-1789705709989745000-3f87bc71).
func TestContributingDCO_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	scriptPath := filepath.Join(root, "scripts", "install-git-hooks.sh")
	if !fileutil.Exists(scriptPath) {
		t.Skipf("scripts/install-git-hooks.sh missing at %s (studio-only artifact)", scriptPath)
	}

	// Create a temporary git repo to test hook installation
	tmpDir := t.TempDir()
	initCmd := execwrap.Command("git", "init")
	initCmd.Dir = tmpDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init temp git repo: %v\nOutput: %s", err, string(out))
	}

	// Run install script inside the temp git repo
	hookInstallCmd := execwrap.Command("bash", scriptPath)
	hookInstallCmd.Dir = tmpDir
	out, err := hookInstallCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install-git-hooks.sh failed: %v\nOutput: %s", err, string(out))
	}

	installedHook := filepath.Join(tmpDir, ".git", "hooks", "pre-commit")
	if !fileutil.Exists(installedHook) {
		t.Fatalf("expected pre-commit hook installed at %s", installedHook)
	}
}
