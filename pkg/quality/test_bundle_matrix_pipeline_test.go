package quality

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/pipeline"
)

func TestRunTestBundleMatrixPipeline_nilOptions(t *testing.T) {
	err := RunTestBundleMatrixPipeline(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "options required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTestBundleMatrixPipeline_emptyProjectRoot(t *testing.T) {
	err := RunTestBundleMatrixPipeline(context.Background(), &TestBundleMatrixOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ProjectRoot required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTestBundleMatrixPipeline_missingGenerateScript(t *testing.T) {
	dir := t.TempDir()
	err := RunTestBundleMatrixPipeline(context.Background(), &TestBundleMatrixOptions{
		ProjectRoot: dir,
		SkipVerify:  true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing script") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_matrixVerifySucceededForContextEvent(t *testing.T) {
	cases := []struct {
		name string
		out  map[string]any
		want bool
	}{
		{"nil", nil, false},
		{"skipped", map[string]any{pipeline.OutcomeKeyVerifySkipped: true}, false},
		{"skipped_and_exit_true", map[string]any{pipeline.OutcomeKeyVerifySkipped: true, pipeline.OutcomeKeyVerifyExit: true}, false},
		{"exit_false", map[string]any{pipeline.OutcomeKeyVerifyExit: false}, false},
		{"exit_true", map[string]any{pipeline.OutcomeKeyVerifyExit: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matrixVerifySucceededForContextEvent(tc.out); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestRunTestBundleMatrixPipeline_smokeNoVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("runs python generate script")
	}
	root := moduleRootFromGoEnvForQualityTest(t)
	err := RunTestBundleMatrixPipeline(context.Background(), &TestBundleMatrixOptions{
		ProjectRoot: root,
		SkipVerify:  true,
	})
	if err != nil {
		t.Fatalf("smoke: %v", err)
	}
}

// moduleRootFromGoEnvForQualityTest returns the directory containing go.mod (go env GOMOD), same contract as pkg/testing.ModuleRootFromGoEnv.
func moduleRootFromGoEnvForQualityTest(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return filepath.Dir(modPath)
}
