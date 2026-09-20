package processhygiene_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSBOMAndDependabotConfiguration(t *testing.T) {
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

	dependabotPath := filepath.Join(repoRoot, ".github", "dependabot.yml")
	depBytes, err := fileutil.ReadFile(dependabotPath)
	if err != nil {
		t.Fatalf("missing dependabot.yml: %v", err)
	}

	var depParsed map[string]any
	if err := yaml.Unmarshal(depBytes, &depParsed); err != nil {
		t.Fatalf("invalid YAML in dependabot.yml: %v", err)
	}

	if !strings.Contains(string(depBytes), "gomod") || !strings.Contains(string(depBytes), "github-actions") {
		t.Fatal("dependabot.yml must configure gomod and github-actions")
	}

	sbomWorkflowPath := filepath.Join(repoRoot, ".github", "workflows", "sbom.yml")
	sbomBytes, err := fileutil.ReadFile(sbomWorkflowPath)
	if err != nil {
		t.Fatalf("missing sbom.yml workflow: %v", err)
	}

	var sbomParsed map[string]any
	if err := yaml.Unmarshal(sbomBytes, &sbomParsed); err != nil {
		t.Fatalf("invalid YAML in sbom.yml: %v", err)
	}

	if !strings.Contains(string(sbomBytes), "sbom") {
		t.Fatal("sbom.yml must reference sbom generation")
	}

	securityMdPath := filepath.Join(repoRoot, "SECURITY.md")
	secBytes, err := fileutil.ReadFile(securityMdPath)
	if err != nil {
		t.Fatalf("missing SECURITY.md: %v", err)
	}

	if !strings.Contains(string(secBytes), "SBOM") || !strings.Contains(string(secBytes), "Dependabot") {
		t.Fatal("SECURITY.md must document SBOM and Dependabot")
	}
}
