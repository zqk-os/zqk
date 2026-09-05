package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestAgentWorktreeDir_defaultOutsideProject(t *testing.T) {
	t.Parallel()
	proj := t.TempDir()
	got := AgentWorktreeDir(proj, "ATK-1")
	rel, err := filepath.Rel(proj, got)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != ".." {
		t.Fatalf("worktree must not live under project root: project=%s wt=%s rel=%s", proj, got, rel)
	}
	if !strings.Contains(got, "ATK-1") {
		t.Fatalf("path should include task id: %s", got)
	}
}

func TestAgentWorktreeDir_envOverride(t *testing.T) {
	base := t.TempDir()
	t.Setenv(zqkenv.AgentWorktreeRoot(), base)
	proj := filepath.Join(base, "proj")
	got := AgentWorktreeDir(proj, "ATK-9")
	want := filepath.Join(base, "ATK-9")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestAgentWorktreeDir_emptyTask(t *testing.T) {
	t.Parallel()
	got := AgentWorktreeDir(t.TempDir(), "  ")
	if !strings.Contains(got, "unnamed") {
		t.Fatalf("empty task id should use unnamed: %s", got)
	}
}

func TestAgentWorktreeDir_notNestedInProjectEvenIfEnvInside(t *testing.T) {
	proj := t.TempDir()
	inside := filepath.Join(proj, ".zqk", "worktrees")
	t.Setenv(zqkenv.AgentWorktreeRoot(), inside)
	got := AgentWorktreeDir(proj, "ATK-in")
	rel, err := filepath.Rel(proj, got)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("refused in-project override: still under project: %s", got)
	}
}
