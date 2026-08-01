package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
)

// findModuleRoot returns the module root (directory containing go.mod) by walking up from cwd.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// TestArchiveGenerationIncludesNewObjectSpec ensures that when we add a new object spec
// under docs/architecture/_internal and run the bootstrap archive build script, the new file
// is included in the archive and listed in the manifest. This validates that "every new
// build" (which runs bootstrap-archive) picks up the latest _internal content.
func TestArchiveGenerationIncludesNewObjectSpec(t *testing.T) {
	t.Parallel()
	moduleRoot, err := findModuleRoot()
	if err != nil || moduleRoot == emptyValue {
		t.Skipf("could not find module root (go.mod): %v", err)
	}
	scriptPath := filepath.Join(moduleRoot, "scripts", "build-bootstrap-archive.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Skipf("build-bootstrap-archive.sh not found: %v", err)
	}

	// Temp dir as fake repo root with docs/architecture/_internal
	tmp := t.TempDir()
	internalRoot := datacell.CellCASPrimaryDir(tmp, "_internal")
	objectSpecsDir := filepath.Join(internalRoot, "object_specs")
	if err := os.MkdirAll(objectSpecsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir object_specs: %v", err)
	}

	// Add a new object spec that would not exist in the real repo
	newSpecName := "test_archive_new_spec.yaml"
	newSpecPath := filepath.Join(objectSpecsDir, newSpecName)
	const minimalSpec = "kind: object_spec\nid_prefix: tst\n"
	if err := os.WriteFile(newSpecPath, []byte(minimalSpec), paths.FilePerm644); err != nil {
		t.Fatalf("write new spec: %v", err)
	}

	// Run the build script with temp dir as REPO_ROOT; it will create tmp/internal/bootstrap/archive
	cmd := exec.Command("sh", scriptPath, tmp)
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build-bootstrap-archive.sh failed: %v\n%s", err, out)
	}

	manifestPath := filepath.Join(tmp, "internal", "bootstrap", "archive", "manifest.txt")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestContent := string(data)
	expectedEntry := "object_specs/" + newSpecName
	if !strings.Contains(manifestContent, expectedEntry) {
		t.Errorf("manifest should contain %q after adding new spec; manifest:\n%s", expectedEntry, manifestContent)
	}
}

// TestManifestPaths_EmbeddedArchive verifies the embedded archive (from the last build)
// has a non-empty manifest and includes at least one expected path so we know the
// binary was built with bootstrap-archive run.
func TestManifestPaths_EmbeddedArchive(t *testing.T) {
	t.Parallel()
	paths := ManifestPaths()
	if paths == nil {
		t.Skip("no embedded manifest (dev build without archive?)")
	}
	if len(paths) == 0 {
		t.Error("embedded manifest should not be empty after build")
	}
	// At least one object_spec or known _internal path should be present (manifest may use "./" prefix)
	hasSpec := false
	hasCLISpecs := false
	for _, p := range paths {
		norm := strings.TrimPrefix(p, "./")
		if strings.HasPrefix(norm, "object_specs/") && strings.HasSuffix(norm, ".yaml") {
			hasSpec = true
		}
		if strings.HasPrefix(norm, "cli_specs/") {
			hasCLISpecs = true
		}
		if hasSpec && hasCLISpecs {
			break
		}
	}
	if !hasSpec {
		t.Errorf("embedded manifest should include at least one object_specs/*.yaml; got %d paths", len(paths))
	}
	if !hasCLISpecs {
		t.Errorf("embedded manifest should include cli_specs/ (command specs) for clean install; got %d paths", len(paths))
	}
}
