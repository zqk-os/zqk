package processhygiene_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func findRepoRoot(t *testing.T) string {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			return repoRoot
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatal("could not locate repo root containing go.mod")
		}
		repoRoot = parent
	}
}

// TestCommandSpecSSOTDecisionRecorded verifies that DEC-1786732826125502000-ef80a104
// is recorded in the kernel and adopts file DNA as the single source of truth for CLI commands
// (BLI-1786686475378466000-405c6afc / CRIT-1786696710475120000-ac5e4d34).
func TestCommandSpecSSOTDecisionRecorded(t *testing.T) {
	repoRoot := findRepoRoot(t)
	decisionsDir := filepath.Join(repoRoot, paths.ProcessDir, "decisions")

	entries, err := fileutil.ReadDir(decisionsDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			t.Skip("skipping in git-clean environment without local process state")
		}
		t.Fatalf("ReadDir %s: %v", decisionsDir, err)
	}

	found := false
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := fileutil.ReadFile(filepath.Join(decisionsDir, entry.Name()))
		if err != nil {
			continue
		}
		text := string(data)
		if strings.Contains(text, "DEC-1786732826125502000-ef80a104") {
			found = true
			if !strings.Contains(text, paths.CLICommandSpecsDir) {
				t.Errorf("Decision does not cite %s as file DNA: %s", paths.CLICommandSpecsDir, text)
			}
			if !strings.Contains(text, "Adopt file DNA as the CLI command single source of truth") {
				t.Errorf("Decision title unexpected: %s", text)
			}
			break
		}
	}

	if !found {
		t.Skipf("DEC-1786732826125502000-ef80a104 not found in %s (instance decision object not shipped in public open-core candidate)", decisionsDir)
	}
}

// TestCommandSpecFileDNADirectoryExists verifies that .zqk/cli/specs/ exists and contains specs.
func TestCommandSpecFileDNADirectoryExists(t *testing.T) {
	repoRoot := findRepoRoot(t)
	specsDir := filepath.Join(repoRoot, paths.ProjectDataDir, "cli", "specs")

	info, err := fileutil.Stat(specsDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("Expected .zqk/cli/specs directory to exist: %v", err)
	}

	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", specsDir, err)
	}

	yamlCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			yamlCount++
		}
	}

	if yamlCount == 0 {
		t.Errorf("Expected .zqk/cli/specs to contain YAML command specs, found 0")
	}
}
