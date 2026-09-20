package distribution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommunityDocumentationHonesty verifies that COMMUNITY_FIRST_RUN.md and README.md
// accurately document polyglot init, native code search, and Homebrew tap installation.
func TestCommunityDocumentationHonesty(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	// Verify COMMUNITY_FIRST_RUN.md
	firstRunPath := filepath.Join(repoRoot, "docs", "onboarding", "COMMUNITY_FIRST_RUN.md")
	firstRunBytes, err := os.ReadFile(firstRunPath)
	if err != nil {
		t.Fatalf("failed to read COMMUNITY_FIRST_RUN.md: %v", err)
	}
	firstRunContent := string(firstRunBytes)

	requiredFirstRunTerms := []string{
		"no Homebrew formula and no public GitHub release",
		"zqk system init",
		"zqk grep",
	}
	for _, term := range requiredFirstRunTerms {
		if !strings.Contains(firstRunContent, term) {
			t.Errorf("COMMUNITY_FIRST_RUN.md missing required term: %q", term)
		}
	}

	// Verify README.md
	readmePath := filepath.Join(repoRoot, "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	readmeContent := string(readmeBytes)

	requiredReadmeTerms := []string{
		"no brew formula and no public GitHub release",
		"github.com/zqk-os/zqk",
		"zqk grep",
		"Polyglot",
	}
	for _, term := range requiredReadmeTerms {
		if !strings.Contains(readmeContent, term) {
			t.Errorf("README.md missing required term: %q", term)
		}
	}
}

// TestPackageCommunityScriptExecutable verifies that scripts/package-community.sh exists
// and is executable.
func TestPackageCommunityScriptExecutable(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "scripts", "package-community.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("package-community.sh not found: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("package-community.sh is not executable: mode=%v", info.Mode())
	}
}
