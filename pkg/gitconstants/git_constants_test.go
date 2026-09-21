package gitconstants

import "testing"

func TestGitConstantsSanity(t *testing.T) {
	t.Parallel()

	if BinaryGit != "git" {
		t.Fatalf("BinaryGit = %q, want git", BinaryGit)
	}
	if SubcmdWorktree != "worktree" {
		t.Fatalf("SubcmdWorktree = %q, want worktree", SubcmdWorktree)
	}
	if ActionRemove != "remove" {
		t.Fatalf("ActionRemove = %q, want remove", ActionRemove)
	}
	if FlagGitCommonDir != "--git-common-dir" {
		t.Fatalf("FlagGitCommonDir = %q, want --git-common-dir", FlagGitCommonDir)
	}
	if FlagDeleteForce != "-D" {
		t.Fatalf("FlagDeleteForce = %q, want -D", FlagDeleteForce)
	}
}
