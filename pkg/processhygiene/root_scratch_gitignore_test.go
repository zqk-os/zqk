package processhygiene_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRootScratchGitignoreAndHygiene(t *testing.T) {
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

	// 1. Verify LICENSE and NOTICE exist at root (CRIT-CEF-ROOT-SCRATCH-GITIGNORE-001)
	licensePath := filepath.Join(repoRoot, "LICENSE")
	if _, err := fileutil.Stat(licensePath); err != nil {
		t.Errorf("missing LICENSE at repo root: %v", err)
	}
	noticePath := filepath.Join(repoRoot, "NOTICE")
	if _, err := fileutil.Stat(noticePath); err != nil {
		t.Errorf("missing NOTICE at repo root: %v", err)
	}

	// 2. Verify .gitignore exists and covers required scratch patterns
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	gitignoreBytes, err := fileutil.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("missing .gitignore at repo root: %v", err)
	}
	gitignoreContent := string(gitignoreBytes)

	requiredPatterns := []string{
		"whats_next*.json",
		"prompt_*.txt",
		"prompt_*.md",
		"fix*.py",
	}

	for _, pattern := range requiredPatterns {
		if !strings.Contains(gitignoreContent, pattern) {
			t.Errorf(".gitignore missing required scratch pattern: %q", pattern)
		}
	}

	// 3. Verify that forbidden scratch files are not tracked in git at root
	cmd := exec.Command("git", "-C", repoRoot, "ls-files") //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}

	trackedFiles := strings.Split(string(out), "\n")
	forbiddenRootPrefixes := []string{
		"whats_next",
		"actual_prompt",
		"prompt_census",
		"prompt_trash",
		"prompt_twins",
		"prompt_atk",
		"prompt_subagent",
		"scratch.go",
		"scratch.sed",
		"rename_cas.py",
		"run_sed.py",
		"fix_graph_logging.py",
		"fix_graph_storage.py",
		"fix_imports.py",
		"fix_os_exec.py",
		"fix_health.sh",
		"bli_1.yaml",
		"errors.yaml",
		"panics.yaml",
		"mcp.yaml",
	}

	for _, f := range trackedFiles {
		f = strings.TrimSpace(f)
		if f == "" || strings.Contains(f, "/") {
			// Only checking root files
			continue
		}
		if strings.HasSuffix(f, ".test") {
			t.Errorf("tracked .test binary found at root: %s", f)
		}
		for _, forbidden := range forbiddenRootPrefixes {
			if f == forbidden || strings.HasPrefix(f, forbidden) {
				t.Errorf("forbidden scratch file tracked in git: %s", f)
			}
		}
	}
}
