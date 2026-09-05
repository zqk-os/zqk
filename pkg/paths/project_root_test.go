package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestResolveProjectRoot_EnvPrecedence(t *testing.T) {
	t.Setenv(zqkenv.ProjectRoot(), "/tmp/zqk-explicit-root")
	t.Setenv(zqkenv.TestRoot(), "/tmp/zqk-test-root-should-lose")
	got := ResolveProjectRoot(".")
	want, err := filepath.Abs("/tmp/zqk-explicit-root")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ResolveProjectRoot = %q, want %q", got, want)
	}
}

func TestIsValidProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if IsValidProjectRoot(dir) {
		t.Fatal("empty dir should not be a project root")
	}
	if err := os.Mkdir(filepath.Join(dir, ProjectDataDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsValidProjectRoot(dir) {
		t.Fatal("dir with .zqk should be a project root")
	}
}
