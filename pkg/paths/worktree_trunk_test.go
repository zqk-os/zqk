package paths

import "testing"

func TestIsGitTrunkBranch(t *testing.T) {
	t.Parallel()
	if !IsGitTrunkBranch("main") || !IsGitTrunkBranch("master") {
		t.Fatal("expected main/master to be trunk")
	}
	if IsGitTrunkBranch("origin/main") || IsGitTrunkBranch("integration/pri-x") {
		t.Fatal("origin/main and plan branches must not count as a trunk lock")
	}
}

func TestShouldEvictLinkedTrunkLock(t *testing.T) {
	t.Parallel()
	primary := "/Users/me/zqk"
	if ShouldEvictLinkedTrunkLock(primary, "main", primary) {
		t.Fatal("primary checkout on main must not be evicted")
	}
	if !ShouldEvictLinkedTrunkLock("/tmp/zqk-worktrees/x/land-r20-r18", "main", primary) {
		t.Fatal("linked worktree on main must be evicted")
	}
}
