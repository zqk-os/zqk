package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCountAheadUpstream(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := exec.Command("git", "init", "--bare", remote).Run(); err != nil {
		t.Fatalf("bare: %v", err)
	}

	work := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	if err := fileutil.WriteFile(filepath.Join(work, "a"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a")
	run("commit", "-m", "base")
	run("branch", "-M", "main")
	run("remote", "add", "origin", remote)
	run("push", "-u", "origin", "HEAD")

	f := NewFacade(work)
	if n := f.CountAheadUpstream(); n != 0 {
		t.Fatalf("synced ahead=%d", n)
	}

	if err := fileutil.WriteFile(filepath.Join(work, "b"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "b")
	run("commit", "-m", "local")
	if n := f.CountAheadUpstream(); n != 1 {
		t.Fatalf("ahead=%d want 1", n)
	}

	if n := NewFacade(t.TempDir()).CountAheadUpstream(); n != 0 {
		t.Fatalf("non-repo must be 0, got %d", n)
	}
}
