package agentrules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestForbidGitCleanProcess_blocksStashUntracked(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "forbid-git-clean-process.sh")
	if _, err := fileutil.Stat(script); err != nil {
		t.Skip("scripts/forbid-git-clean-process.sh absent (studio script)")
	}

	cmd := execwrap.Command("sh", script, "stash", "-u")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "ZQK_ALLOW_PROCESS_GIT_CLEAN=")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("stash -u should be blocked, got exit 0\n%s", out)
	} else if cmd.ProcessState.ExitCode() != 2 {
		t.Fatalf("stash -u exit = %d want 2\n%s", cmd.ProcessState.ExitCode(), out)
	}

	ok := execwrap.Command("sh", script, "status")
	ok.Dir = root
	if out, err := ok.CombinedOutput(); err != nil {
		t.Fatalf("status should pass: %v\n%s", err, out)
	}
}

func TestScanStashForProcessCAS_emptyStashExitsZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...) //nolint:gosec
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "gate@test")
	run("config", "user.name", "gate")
	if err := fileutil.WriteFile(filepath.Join(dir, "README"), []byte("x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "base")

	root := repoRoot(t)
	scanScript := filepath.Join(root, "scripts", "scan-stash-for-process-cas.sh")
	if _, err := fileutil.Stat(scanScript); err != nil {
		t.Skip("scripts/scan-stash-for-process-cas.sh absent (studio script)")
	}
	cmd := execwrap.Command("sh", scanScript) //nolint:gosec
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("empty stash should exit 0: %v\n%s", err, out)
	}
}
