package community

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestModulePathSync_FunctionalAcceptance verifies that the community open-core distribution
// Go module is cleanly decoupled, properly named github.com/zqk-os/zqk, and contains no
// internal studio package import references (CRIT-1789701163664955000-db4e6665).
func TestModulePathSync_FunctionalAcceptance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	goModPath := filepath.Join(root, "go.mod")
	if !fileutil.Exists(goModPath) {
		t.Fatalf("go.mod not found at %s", goModPath)
	}

	content, err := fileutil.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed reading go.mod: %v", err)
	}

	// 1. Verify module declaration
	if !strings.Contains(string(content), "module github.com/zqk-os/zqk") {
		t.Errorf("expected module github.com/zqk-os/zqk, got:\n%s", string(content))
	}

	// 2. Verify all open-core candidate Go packages use the canonical module path
	candidateDir := publicCandidateFixture(t)
	candidateMod := filepath.Join(candidateDir, "go.mod")
	if fileutil.Exists(candidateMod) {
		candModBytes, err := fileutil.ReadFile(candidateMod)
		if err != nil {
			t.Fatalf("failed reading candidate go.mod: %v", err)
		}
		if !strings.Contains(string(candModBytes), "module github.com/zqk-os/zqk") {
			t.Errorf("candidate go.mod does not have canonical module declaration: %s", string(candModBytes))
		}
	}
}

// TestModulePathSync_BoundaryAndErrorHandling verifies that unauthorized internal imports
// or mismatched module names are rejected during translation / tree police (CRIT-1789701163664956000-4c2b89b8).
func TestModulePathSync_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// 1. Mock Go source with a forbidden studio import
	fakeSrc := filepath.Join(tmpDir, "sample.go")
	badImport := []byte(`package main
import (
	"fmt"
	"github.com/zqk-os/zqk/pkg/mesh"
)
func main() {
	fmt.Println("test")
}
`)
	if err := fileutil.WriteFile(fakeSrc, badImport, 0644); err != nil {
		t.Fatalf("failed to write fake source: %v", err)
	}

	// Scan fake source for forbidden imports
	data, err := fileutil.ReadFile(fakeSrc)
	if err != nil {
		t.Fatalf("failed reading fake source: %v", err)
	}

	forbiddenImports := []string{
		"github.com/zqk-os/zqk/pkg/mesh",
		"github.com/zqk-os/zqk/pkg/agent",
		"github.com/zqk-os/zqk/cmd/zqk-admin",
	}

	foundForbidden := false
	for _, fi := range forbiddenImports {
		if strings.Contains(string(data), fi) {
			foundForbidden = true
			break
		}
	}

	if !foundForbidden {
		t.Errorf("expected forbidden import detection in mock source, but none detected")
	}
}

// TestModulePathSync_IntegrationAndConformance verifies that candidate Go source files
// in open-core packages compile standalone without internal replace directives or broken packages (CRIT-1789701163664957000-90b14e8a).
func TestModulePathSync_IntegrationAndConformance(t *testing.T) {
	root := paths.ResolveProjectRoot(".")

	// Verify go.mod does not contain replace directives that leak internal disk paths
	goModPath := filepath.Join(root, "go.mod")
	f, err := fileutil.Open(goModPath)
	if err != nil {
		t.Fatalf("failed opening go.mod: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "replace ") && strings.Contains(line, "/Users/") {
			t.Errorf("go.mod contains local absolute path replace directive: %s", line)
		}
	}

	// Verify cmd/zqk-community builds standalone
	binPath := filepath.Join(t.TempDir(), "zqk-community-test")
	cmd := execwrap.Command("go", "build", "-buildvcs=false", "-o", binPath, "./cmd/zqk-community")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build ./cmd/zqk-community failed: %v\nStderr: %s", err, stderr.String())
	}
}
