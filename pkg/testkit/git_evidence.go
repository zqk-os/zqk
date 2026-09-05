package testkit

import (
	"os/exec"
	"strings"
	"testing"
)

// ProductCommitMentioningBacklog inits git at root if needed, commits the
// already-written relPath with bliID in the message, and returns HEAD.
// Isolated complete gates require a non-merge product commit that names the BLI.
func ProductCommitMentioningBacklog(t testing.TB, root, bliID, relPath string) string {
	t.Helper()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "test")
	runGit(t, root, "add", relPath)
	runGit(t, root, "commit", "-m", "feat: work "+bliID)
	return strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
}

func runGit(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
