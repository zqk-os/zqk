package scs

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := execwrap.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

func initTestRepo(t *testing.T, repoDir string) {
	t.Helper()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "checkout", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "initial")
}

func TestNew_ClientBasicRepositoryOperations(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	initTestRepo(t, repoDir)
	runGit(t, repoDir, "remote", "add", "origin", "git@github.com:example/repo.git")

	client := New(repoDir)

	branch, err := client.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("CurrentBranch() = %q, want %q", branch, "main")
	}

	if !client.BranchExists("main") {
		t.Fatalf("BranchExists(main) = false, want true")
	}
	if client.BranchExists("feature/missing") {
		t.Fatalf("BranchExists(feature/missing) = true, want false")
	}

	originURL, err := client.OriginURL()
	if err != nil {
		t.Fatalf("OriginURL() error: %v", err)
	}
	if originURL != "git@github.com:example/repo.git" {
		t.Fatalf("OriginURL() = %q, want %q", originURL, "git@github.com:example/repo.git")
	}
}

func TestNew_ClientCreateAndSwitchBranch(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	initTestRepo(t, repoDir)

	client := New(repoDir)

	if _, err := client.CreateBranch("feature/metrics-facade"); err != nil {
		t.Fatalf("CreateBranch() error: %v", err)
	}
	branch, err := client.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error after create: %v", err)
	}
	if branch != "feature/metrics-facade" {
		t.Fatalf("CurrentBranch() after create = %q, want %q", branch, "feature/metrics-facade")
	}

	if _, err := client.CheckoutBranch("main"); err != nil {
		t.Fatalf("CheckoutBranch(main) error: %v", err)
	}
	branch, err = client.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error after checkout: %v", err)
	}
	if branch != "main" {
		t.Fatalf("CurrentBranch() after checkout = %q, want %q", branch, "main")
	}
}
