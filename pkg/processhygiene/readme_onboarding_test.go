package processhygiene_test

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadmeLeadsWithStrangerUsablePath(t *testing.T) {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatal("could not locate repo root containing go.mod")
		}
		repoRoot = parent
	}

	readmePath := filepath.Join(repoRoot, "README.md")
	readmeBytes, err := fileutil.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("missing README.md: %v", err)
	}

	readmeText := string(readmeBytes)

	// Verify Community First-Run is prominent near top
	if !strings.Contains(readmeText, "COMMUNITY_FIRST_RUN.md") {
		t.Fatal("README.md must prominently link to COMMUNITY_FIRST_RUN.md")
	}

	// Verify top 50 lines do not contain dense internal jargon blocking strangers
	lines := strings.Split(readmeText, "\n")
	topLinesCount := 50
	if len(lines) < topLinesCount {
		topLinesCount = len(lines)
	}
	topContent := strings.Join(lines[:topLinesCount], "\n")

	if strings.Contains(topContent, "MMORCH") {
		t.Fatal("Top of README.md should not use unexplained MMORCH jargon")
	}
}

func TestReadmePrefersCommunityFirstRunOverStudioPack(t *testing.T) {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatal("could not locate repo root containing go.mod")
		}
		repoRoot = parent
	}

	readmePath := filepath.Join(repoRoot, "README.md")
	readmeBytes, err := fileutil.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("missing README.md: %v", err)
	}

	readmeText := string(readmeBytes)
	commIdx := strings.Index(readmeText, "COMMUNITY_FIRST_RUN.md")
	studioIdx := strings.Index(readmeText, "AI_AGENT_ONBOARDING.md")

	if commIdx == -1 {
		t.Fatal("README.md must link to COMMUNITY_FIRST_RUN.md")
	}
	if studioIdx != -1 && commIdx > studioIdx {
		t.Fatalf("COMMUNITY_FIRST_RUN.md must appear before AI_AGENT_ONBOARDING.md in README.md (got commIdx=%d, studioIdx=%d)", commIdx, studioIdx)
	}
}
