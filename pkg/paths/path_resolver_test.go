package paths

import (
	"path/filepath"
	"testing"
)

func TestNewPathResolver_ResolveStrict_process(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ReplacePathCache(root, DefaultPathAliases())
	r := NewPathResolver(root)
	got, err := r.ResolveStrict(PathSchemePrefix + "process")
	if err != nil {
		t.Fatalf("ResolveStrict: %v", err)
	}
	want := filepath.Join(root, ProcessDir)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNewPathResolver_ResolveFromCacheOrConstant(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ReplacePathCache(root, map[string]string{"scheduler": filepath.Join(ProjectDataDir, SchedulerDir)})
	r := NewPathResolver(root)
	got := r.ResolveFromCacheOrConstant("scheduler", filepath.Join(ProjectDataDir, SchedulerDir))
	want := filepath.Join(root, ProjectDataDir, SchedulerDir)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNewPathResolver_ProjectRoot(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/tmp/proj")
	if r.ProjectRoot() != "/tmp/proj" {
		t.Errorf("ProjectRoot = %q", r.ProjectRoot())
	}
}
