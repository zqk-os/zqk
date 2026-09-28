package lifecycle_builders

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGeneratedGoalLifecycleCompiles(t *testing.T) {
	root := lifecycleModuleRoot(t)
	packRoot := filepath.Join(root, "packs", "workgen")
	t.Cleanup(func() { _ = fileutil.RemoveAll(packRoot) })

	yamlPath := filepath.Join(root, paths.ProcessInternalLifecyclesDir, "pm", "goal_lifecycle.yaml")
	if !fileutil.Exists(yamlPath) {
		yamlPath = filepath.Join(root, "packs", "work", "lifecycles", "goal_lifecycle.yaml")
	}
	if !fileutil.Exists(yamlPath) {
		t.Skipf("goal lifecycle YAML not found at expected paths")
	}

	toolDir := filepath.Join(packRoot, "lifecycle_builders")
	if err := fileutil.MkdirAll(toolDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := GenerateBuilderFromYAML(yamlPath, toolDir); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "./packs/workgen/bldr_lifecycle_v1")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated goal lifecycle did not compile: %v\n%s", err, out)
	}
}

func lifecycleModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := fileutil.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			line, _, _ := strings.Cut(string(data), "\n")
			if strings.TrimSpace(line) == "module github.com/zqk-os/zqk" {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
