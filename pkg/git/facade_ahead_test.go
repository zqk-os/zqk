package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCountAheadUpstream(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := execwrap.Command("git", "init", "--bare", remote).Run(); err != nil {
		t.Fatalf("bare: %v", err)
	}

	work := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := execwrap.Command("git", args...)
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
	if err := fileutil.WriteFile(filepath.Join(work, "a"), []byte("1"), paths.FilePerm644); err != nil {
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

	if err := fileutil.WriteFile(filepath.Join(work, "b"), []byte("2"), paths.FilePerm644); err != nil {
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

func TestFindCommitHashesByGrep(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	work := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := execwrap.Command("git", args...)
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
	if err := fileutil.WriteFile(filepath.Join(work, "file.txt"), []byte("data"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	run("add", "file.txt")
	run("commit", "-m", "feat(cli): initial commit for BLI-1234")

	f := NewFacade(work)
	ctx := context.Background()
	hashes, err := f.FindCommitHashesByGrep(ctx, 5, "BLI-1234")
	if err != nil {
		t.Fatalf(`FindCommitHashesByGrep error: %v`, err)
	}
	if len(hashes) != 1 {
		t.Fatalf(`expected 1 commit hash, got %d`, len(hashes))
	}

	empty, err := f.FindCommitHashesByGrep(ctx, 5, "NONEXISTENT")
	if err != nil {
		t.Fatalf(`FindCommitHashesByGrep nonexistent error: %v`, err)
	}
	if len(empty) != 0 {
		t.Fatalf(`expected 0 commit hashes, got %d`, len(empty))
	}
}
