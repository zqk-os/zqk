package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), base)
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
	t.Setenv(zqkenv.AgentWorktreeRoot().Name(), inside)
	got := AgentWorktreeDir(proj, "ATK-in")
	rel, err := filepath.Rel(proj, got)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("refused in-project override: still under project: %s", got)
	}
}

func TestAgentWorktreeMainRepo_seatedKernelNotParentHops(t *testing.T) {
	t.Parallel()
	studio := t.TempDir()
	wt := filepath.Join(t.TempDir(), agentWorktreeTempBucket, "repo-key", "ATK-1")
	if err := BootstrapWorktreeConfig(wt, studio); err != nil {
		t.Fatal(err)
	}
	got, err := AgentWorktreeMainRepo(wt)
	if err != nil {
		t.Fatal(err)
	}
	if got != studio {
		t.Fatalf("got %q want seated kernel %q", got, studio)
	}
	hop := filepath.Dir(filepath.Dir(filepath.Dir(wt)))
	if got == hop {
		t.Fatalf("must not resolve studio by three Dir hops (%s)", hop)
	}
}

func TestAgentWorktreeMainRepo_refusesUnbondedIsolatedTree(t *testing.T) {
	t.Parallel()
	wt := filepath.Join(t.TempDir(), agentWorktreeTempBucket, "repo-key", "ATK-unbonded")
	if err := fileutil.MkdirAll(wt, DirPerm755); err != nil {
		t.Fatal(err)
	}
	got, err := AgentWorktreeMainRepo(wt)
	if err == nil {
		t.Fatalf("unbonded isolated worktree must not guess a parent; got %q", got)
	}
}

func TestAgentWorktreeMainRepo_gitCommonDir(t *testing.T) {
	t.Parallel()
	studio := t.TempDir()
	initGitRepo(t, studio)
	wt := filepath.Join(t.TempDir(), agentWorktreeTempBucket, "repo-key", "ATK-git")
	cmd := execwrap.Command("git", "worktree", "add", "--detach", wt)
	cmd.Dir = studio
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v (%s)", err, out)
	}
	got, err := AgentWorktreeMainRepo(wt)
	if err != nil {
		t.Fatal(err)
	}
	studioAbs, err := filepath.EvalSymlinks(studio)
	if err != nil {
		studioAbs, _ = filepath.Abs(studio)
	}
	gotAbs, err := filepath.EvalSymlinks(got)
	if err != nil {
		gotAbs, _ = filepath.Abs(got)
	}
	if gotAbs != studioAbs {
		t.Fatalf("git common-dir = %q want %q", gotAbs, studioAbs)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := execwrap.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	for _, args := range [][]string{
		{"config", "user.name", "Test"},
		{"config", "user.email", "test@test.local"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd = execwrap.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
}

func TestBootstrapWorktreeConfig_AssumeUnchanged(t *testing.T) {
	t.Parallel()
	repoDir := t.TempDir()
	seatedRoot := t.TempDir()

	cmd := execwrap.Command("git", "init")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (out: %s)", err, string(out))
	}

	cmd = execwrap.Command("git", "config", "user.name", "Test")
	cmd.Dir = repoDir
	_ = cmd.Run()
	cmd = execwrap.Command("git", "config", "user.email", "test@test.local")
	cmd.Dir = repoDir
	_ = cmd.Run()

	cfgDir := filepath.Join(repoDir, ConfigDir)
	if err := fileutil.MkdirAll(cfgDir, DirPerm755); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(cfgDir, ZqkLocalConfigFileName)
	if err := fileutil.WriteFile(cfgFile, []byte("initial: true\n"), FilePerm600); err != nil {
		t.Fatal(err)
	}

	cmd = execwrap.Command("git", "add", "config/zqk-local.yaml")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (out: %s)", err, string(out))
	}

	cmd = execwrap.Command("git", "commit", "-m", "init")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v (out: %s)", err, string(out))
	}

	if err := BootstrapWorktreeConfig(repoDir, seatedRoot); err != nil {
		t.Fatalf("BootstrapWorktreeConfig failed: %v", err)
	}

	data, err := fileutil.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), seatedRoot) {
		t.Fatalf("expected config to contain seatedRoot %q, got: %s", seatedRoot, string(data))
	}

	cmd = execwrap.Command("git", "status", "--porcelain")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v (out: %s)", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected clean porcelain due to assume-unchanged, got: %q", string(out))
	}
}
