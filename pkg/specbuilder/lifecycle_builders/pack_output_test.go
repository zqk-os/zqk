package lifecycle_builders

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedGoalLifecycleCompiles(t *testing.T) {
	root := lifecycleModuleRoot(t)
	packRoot := filepath.Join(root, "packs", "workgen")
	t.Cleanup(func() { _ = os.RemoveAll(packRoot) })
	yamlPath := filepath.Join(root, "packs", "work", "lifecycles", "goal_lifecycle.yaml")
	toolDir := filepath.Join(packRoot, "lifecycle_builders")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := GenerateBuilderFromYAML(yamlPath, toolDir); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./packs/workgen/bldr_lifecycle_v1")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated goal lifecycle did not compile: %v\n%s", err, out)
	}
}

func lifecycleModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
