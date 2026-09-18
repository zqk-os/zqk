package processhygiene_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSecretScanningWorkflowAndDocs(t *testing.T) {
	// Find repo root from current directory
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

	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "secret-scan.yml")
	workflowBytes, err := fileutil.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("missing secret-scan.yml workflow: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(workflowBytes, &parsed); err != nil {
		t.Fatalf("invalid YAML in secret-scan.yml: %v", err)
	}

	if !strings.Contains(string(workflowBytes), "gitleaks") {
		t.Fatal("secret-scan.yml must reference gitleaks")
	}

	securityMdPath := filepath.Join(repoRoot, "SECURITY.md")
	secBytes, err := fileutil.ReadFile(securityMdPath)
	if err != nil {
		t.Fatalf("missing SECURITY.md: %v", err)
	}

	if !strings.Contains(string(secBytes), "secret scanning") && !strings.Contains(string(secBytes), "secret-scan.yml") {
		t.Fatal("SECURITY.md must document automated secret scanning")
	}
}
