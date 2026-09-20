package object

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestGoalAndMilestoneLifecycle_ForwardPercentCompleteDefaults validates goal and milestone lifecycle defaults.
// Goal originated (5) -> proposed (10) and Milestone originated (5) -> not_started (15) must be
// strictly forward by percent_complete so promotion is not skipped.
func TestGoalAndMilestoneLifecycle_ForwardPercentCompleteDefaults(t *testing.T) {
	lifecyclesDir := filepath.Join("..", "..", "..", paths.ProcessInternalLifecyclesDir)
	loader := objects.NewLifecycleLoader(lifecyclesDir)

	// 1. Goal lifecycle validation
	goalLifecycle, err := loader.LoadLifecycle("goal")
	if err != nil {
		t.Fatalf("Failed to load goal lifecycle: %v", err)
	}
	if goalLifecycle == nil {
		t.Fatal("Goal lifecycle is nil")
	}
	goalDefaults := goalLifecycle.PercentComplete.DefaultByStatus
	if goalDefaults == nil {
		t.Fatal("Goal lifecycle default_by_status is nil")
	}

	originatedGoalPercent := objects.LifecycleProgressPercent("originated", goalLifecycle.PercentComplete)
	proposedGoalPercent := objects.LifecycleProgressPercent("proposed", goalLifecycle.PercentComplete)

	if proposedGoalPercent <= originatedGoalPercent {
		t.Errorf("Goal proposed percent (%.1f) must be strictly greater than originated (%.1f) to permit forward promotion",
			proposedGoalPercent, originatedGoalPercent)
	}
	if proposedGoalPercent != 10 {
		t.Errorf("Expected goal proposed percent to be 10, got %.1f", proposedGoalPercent)
	}

	// 2. Milestone lifecycle validation
	milestoneLifecycle, err := loader.LoadLifecycle("milestone")
	if err != nil {
		t.Fatalf("Failed to load milestone lifecycle: %v", err)
	}
	if milestoneLifecycle == nil {
		t.Fatal("Milestone lifecycle is nil")
	}

	originatedMsPercent := objects.LifecycleProgressPercent("originated", milestoneLifecycle.PercentComplete)
	notStartedMsPercent := objects.LifecycleProgressPercent("not_started", milestoneLifecycle.PercentComplete)

	if notStartedMsPercent <= originatedMsPercent {
		t.Errorf("Milestone not_started percent (%.1f) must be strictly greater than originated (%.1f) to permit forward promotion",
			notStartedMsPercent, originatedMsPercent)
	}
	if notStartedMsPercent != 15 {
		t.Errorf("Expected milestone not_started percent to be 15, got %.1f", notStartedMsPercent)
	}
}

// TestWorktreeChangeIntent_ScriptsUseGitTopLevel validates scripts resolve git top level.
// Git hooks and adapters must use `git rev-parse --show-toplevel` so worktree commits do not
// look for intent files on the studio repo root.
func TestWorktreeChangeIntent_ScriptsUseGitTopLevel(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	checkScriptPath := filepath.Join(repoRoot, "scripts", "check-change-intent.sh")
	checkBytes, err := os.ReadFile(checkScriptPath)
	if err != nil {
		t.Fatalf("Failed to read check-change-intent.sh: %v", err)
	}
	checkContent := string(checkBytes)
	if !strings.Contains(checkContent, "git rev-parse --show-toplevel") {
		t.Errorf("scripts/check-change-intent.sh must resolve REPO_ROOT via git rev-parse --show-toplevel")
	}

	declareScriptPath := filepath.Join(repoRoot, "scripts", "declare-change-intent.sh")
	declareBytes, err := os.ReadFile(declareScriptPath)
	if err != nil {
		t.Fatalf("Failed to read declare-change-intent.sh: %v", err)
	}
	declareContent := string(declareBytes)
	if !strings.Contains(declareContent, "git rev-parse --show-toplevel") {
		t.Errorf("scripts/declare-change-intent.sh must resolve REPO_ROOT via git rev-parse --show-toplevel")
	}
}

// TestWorktreeChangeIntent_ExecutionInGitWorktree validates execution in git worktree end-to-end.
// Even when ZQK_PROJECT_ROOT is set in the environment to another directory (e.g. studio root),
// declare-change-intent and check-change-intent resolve the local worktree via git rev-parse --show-toplevel.
func TestWorktreeChangeIntent_ExecutionInGitWorktree(t *testing.T) {
	tempDir := t.TempDir()

	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = tempDir
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, string(out))
	}

	scriptsDir := filepath.Join(tempDir, "scripts")
	if err := os.MkdirAll(filepath.Join(scriptsDir, "fixtures", "change_intents"), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}

	realRepoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}

	for _, f := range []string{"check-change-intent.sh", "declare-change-intent.sh", "check_change_intent.py"} {
		src := filepath.Join(realRepoRoot, "scripts", f)
		dst := filepath.Join(scriptsDir, f)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, paths.DirPerm755); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}

	intentContent := `{
		"schema": "zqk_change_intent_v1",
		"id": "2026-09-17-worktree-test",
		"summary": "verify worktree change intent",
		"changes": [{"path": "tracked.txt", "reason": "test worktree commit"}]
	}`
	intentPath := filepath.Join(scriptsDir, "fixtures", "change_intents", "2026-09-17-worktree-test.json")
	if err := os.WriteFile(intentPath, []byte(intentContent), paths.FilePerm644); err != nil {
		t.Fatalf("write intent file: %v", err)
	}

	trackedFile := filepath.Join(tempDir, "tracked.txt")
	if err := os.WriteFile(trackedFile, []byte("hello"), paths.FilePerm644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	cmdAdd := exec.Command("git", "add", "tracked.txt")
	cmdAdd.Dir = tempDir
	if out, err := cmdAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v: %s", err, string(out))
	}

	studioDir := t.TempDir()

	cmdDeclare := exec.Command("sh", "scripts/declare-change-intent.sh", "2026-09-17-worktree-test")
	cmdDeclare.Dir = tempDir
	cmdDeclare.Env = append(os.Environ(), "ZQK_PROJECT_ROOT="+studioDir)
	if out, err := cmdDeclare.CombinedOutput(); err != nil {
		t.Fatalf("declare-change-intent failed: %v: %s", err, string(out))
	}

	pointerPath := filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, "change_intent_active")
	if _, err := os.Stat(pointerPath); err != nil {
		t.Fatalf("expected pointer at %s, but stat failed: %v", pointerPath, err)
	}

	cmdCheck := exec.Command("sh", "scripts/check-change-intent.sh")
	cmdCheck.Dir = tempDir
	cmdCheck.Env = append(os.Environ(), "ZQK_PROJECT_ROOT="+studioDir, "CURSOR_EXTENSION_HOST_ROLE=agent-exec")
	if out, err := cmdCheck.CombinedOutput(); err != nil {
		t.Fatalf("check-change-intent failed: %v: %s", err, string(out))
	}
}
