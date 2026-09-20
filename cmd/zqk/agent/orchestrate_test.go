package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestOrchestratePriorityPlanExecutionFacingStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   bool
	}{
		{status: objects.ObjectStatusActive, want: true},
		{status: objects.ObjectStatusInProgress, want: true},
		{status: objects.ObjectStatusPaused, want: true},
		{status: objects.ObjectStatusPrioritizing, want: false},
		{status: objects.ObjectStatusComplete, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			got := objects.PlanStatusExecutionFacing(objects.KindPriorityPlan, tc.status)
			if got != tc.want {
				t.Fatalf("PlanStatusExecutionFacing(priority_plan, %q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestBacklogItemEligibleForOrchestration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   bool
	}{
		{status: objects.ObjectStatusPlanned, want: true},
		{status: objects.ObjectStatusInProgress, want: true},
		{status: objects.ObjectStatusValidated, want: true},
		{status: objects.ObjectStatusComplete, want: false},
		{status: objects.ObjectStatusArchived, want: false},
		{status: objects.ObjectStatusCancelled, want: false},
		{status: objects.ObjectStatusImplemented, want: false},
		{status: objects.ObjectStatusPendingVerification, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			got := backlogItemEligibleForOrchestration(tc.status)
			if got != tc.want {
				t.Fatalf("backlogItemEligibleForOrchestration(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestOrchestratedTaskStatusIsCASable(t *testing.T) {
	t.Parallel()

	if orchestratedTaskStatus != objects.ObjectStatusApproved {
		t.Fatalf("orchestrated task status = %q, want CASable %q", orchestratedTaskStatus, objects.ObjectStatusApproved)
	}
}

func TestAppendStringReference(t *testing.T) {
	t.Parallel()

	got := appendStringReference([]string{"first"}, "second")
	got = appendStringReference(got, "second")
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("appendStringReference() = %#v, want [first second] without duplicates", got)
	}
}

func TestCollectOrchestrationCommitManifest(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()
	runTestGit(t, worktree, "init")
	if err := fileutil.WriteFile(filepath.Join(worktree, "result.txt"), []byte("base\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, worktree, "add", "result.txt")
	runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "-m", "base")
	baseSHA := runTestGit(t, worktree, "rev-parse", "HEAD")

	if err := fileutil.WriteFile(filepath.Join(worktree, "result.txt"), []byte("delivered\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, worktree, "add", "result.txt")
	runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "-m", "deliver")

	manifest, err := collectOrchestrationCommitManifest(context.Background(), worktree, "ATK-test", baseSHA)
	if err != nil {
		t.Fatalf("collect manifest: %v", err)
	}
	if manifest["commit_sha"] == baseSHA {
		t.Fatalf("manifest did not advance from base SHA %s", baseSHA)
	}
	changedPaths, ok := manifest[objects.FieldKeyChangedPaths].([]string)
	if !ok || len(changedPaths) != 1 || changedPaths[0] != "result.txt" {
		t.Fatalf("changed_paths = %#v, want [result.txt]", manifest[objects.FieldKeyChangedPaths])
	}
}

func TestAutoCommitWorktreeChanges(t *testing.T) {
	orig := worktreeBuildCheck
	t.Cleanup(func() { worktreeBuildCheck = orig })
	worktreeBuildCheck = func(ctx context.Context, root string) error {
		return nil
	}

	worktree := t.TempDir()
	runTestGit(t, worktree, "init")
	if err := fileutil.WriteFile(filepath.Join(worktree, "base.txt"), []byte("base\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, worktree, "add", "base.txt")
	runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "-m", "base")
	baseSHA := runTestGit(t, worktree, "rev-parse", "HEAD")

	// 1. No uncommitted work: autoCommitWorktreeChanges does nothing
	if err := autoCommitWorktreeChanges(context.Background(), worktree, "ATK-auto-test"); err != nil {
		t.Fatalf("unexpected error on clean tree: %v", err)
	}
	if currentSHA := runTestGit(t, worktree, "rev-parse", "HEAD"); currentSHA != baseSHA {
		t.Fatalf("HEAD should not change when clean")
	}

	// 2. Uncommitted file modified by agent
	if err := fileutil.WriteFile(filepath.Join(worktree, "feature.go"), []byte("package main\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if err := autoCommitWorktreeChanges(context.Background(), worktree, "ATK-auto-test"); err != nil {
		t.Fatalf("auto-commit failed: %v", err)
	}

	// 3. Verify manifest can be collected cleanly
	manifest, err := collectOrchestrationCommitManifest(context.Background(), worktree, "ATK-auto-test", baseSHA)
	if err != nil {
		t.Fatalf("collect manifest failed after auto-commit: %v", err)
	}
	if manifest["commit_sha"] == baseSHA {
		t.Fatalf("commit_sha must advance")
	}
	changedPaths, _ := manifest[objects.FieldKeyChangedPaths].([]string)
	if len(changedPaths) != 1 || changedPaths[0] != "feature.go" {
		t.Fatalf("changed_paths = %#v, want [feature.go]", changedPaths)
	}

	// 4. Auto-commit with itemID includes BLI citation in commit message
	if err := fileutil.WriteFile(filepath.Join(worktree, "feature2.go"), []byte("package main\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if err := autoCommitWorktreeChanges(context.Background(), worktree, "ATK-auto-test-2", "BLI-auto-test-2"); err != nil {
		t.Fatalf("auto-commit with itemID failed: %v", err)
	}
	msg := runTestGit(t, worktree, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "BLI-auto-test-2") {
		t.Fatalf("commit message %q does not contain itemID BLI-auto-test-2", msg)
	}
	wantName, wantEmail := swarmGitAuthorIdentity()
	if got := runTestGit(t, worktree, "log", "-1", "--format=%an"); got != wantName {
		t.Fatalf("author name = %q, want %q", got, wantName)
	}
	if got := runTestGit(t, worktree, "log", "-1", "--format=%ae"); got != wantEmail {
		t.Fatalf("author email = %q, want %q", got, wantEmail)
	}
}

func TestSwarmGitAuthorIdentity_Defaults(t *testing.T) {
	t.Setenv(zqkenv.AgentGitName().Name(), "")
	t.Setenv(zqkenv.AgentGitEmail().Name(), "")
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	name, email := swarmGitAuthorIdentity()
	if name != defaultSwarmGitAuthorName || email != defaultSwarmGitAuthorEmail {
		t.Fatalf("got %q <%q>", name, email)
	}
}

func TestSwarmGitAuthorIdentity_EnvOverride(t *testing.T) {
	t.Setenv(zqkenv.AgentGitName().Name(), "Community Swarm")
	t.Setenv(zqkenv.AgentGitEmail().Name(), "swarm@example.test")
	t.Setenv("GIT_AUTHOR_NAME", "ignored-git-author")
	t.Setenv("GIT_AUTHOR_EMAIL", "ignored@example.test")
	name, email := swarmGitAuthorIdentity()
	if name != "Community Swarm" || email != "swarm@example.test" {
		t.Fatalf("got %q <%q>", name, email)
	}
}

func TestSwarmGitAuthorIdentity_GitAuthorFallback(t *testing.T) {
	t.Setenv(zqkenv.AgentGitName().Name(), "")
	t.Setenv(zqkenv.AgentGitEmail().Name(), "")
	t.Setenv("GIT_AUTHOR_NAME", "Wrapper Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "wrapper@example.test")
	name, email := swarmGitAuthorIdentity()
	if name != "Wrapper Author" || email != "wrapper@example.test" {
		t.Fatalf("got %q <%q>", name, email)
	}
}

func TestOrchestrationExecutorDirtIgnoresDraftPlaneSymlink(t *testing.T) {
	t.Parallel()
	drafts := paths.ObjectDraftsRel()
	if got := orchestrationExecutorDirt("?? " + drafts); got != "" {
		t.Fatalf("symlink porcelain = %q, want empty", got)
	}
	if got := orchestrationExecutorDirt("?? " + drafts + "/\n M result.txt"); !strings.Contains(got, "result.txt") || strings.Contains(got, paths.ObjectDraftsDir) {
		t.Fatalf("kept dirt = %q, want result.txt only", got)
	}
}

func TestOrchestrationExecutorDirtIgnoresWorktreeLocalConfig(t *testing.T) {
	t.Parallel()
	local := paths.WorktreeLocalConfigRel()
	yml := paths.WorktreeLocalConfigRelYml()
	cases := []string{
		" M " + local,
		"M  " + local,
		"MM " + local,
		"?? " + local,
		local,
		" M " + yml,
		"R  old -> " + local,
	}
	for _, c := range cases {
		if got := orchestrationExecutorDirt(c); got != "" {
			t.Fatalf("porcelain %q produced dirt %q, want empty", c, got)
		}
	}
	if got := orchestrationExecutorDirt(" M " + paths.WorktreeLocalConfigRel() + "\n M result.txt"); !strings.Contains(got, "result.txt") || strings.Contains(got, paths.ZqkLocalConfigFileName) {
		t.Fatalf("kept dirt = %q, want result.txt only", got)
	}
}

func TestCollectOrchestrationCommitManifestIgnoresDraftPlaneSymlink(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()
	runTestGit(t, worktree, "init")
	if err := os.WriteFile(filepath.Join(worktree, "result.txt"), []byte("base\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(worktree, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, paths.ProjectDataDir, ".keep"), []byte("keep\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, worktree, "add", "result.txt", filepath.Join(paths.ProjectDataDir, ".keep"))
	runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "-m", "base")
	baseSHA := runTestGit(t, worktree, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(worktree, "result.txt"), []byte("delivered\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, worktree, "add", "result.txt")
	runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "-m", "deliver")

	if err := os.Symlink(t.TempDir(), filepath.Join(worktree, paths.ProjectDataDir, paths.ObjectDraftsDir)); err != nil {
		t.Fatal(err)
	}

	manifest, err := collectOrchestrationCommitManifest(context.Background(), worktree, "ATK-draft-plane", baseSHA)
	if err != nil {
		t.Fatalf("draft-plane symlink must not fail manifest: %v", err)
	}
	if manifest["commit_sha"] == baseSHA {
		t.Fatal("expected a delivered commit")
	}
}

func TestCollectOrchestrationCommitManifestRejectsMissingEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*testing.T, string)
		wantErrSub string
	}{
		{
			name:       "no commit",
			mutate:     func(*testing.T, string) {},
			wantErrSub: "produced no commit",
		},
		{
			name: "dirty worktree",
			mutate: func(t *testing.T, worktree string) {
				t.Helper()
				if err := fileutil.WriteFile(filepath.Join(worktree, "result.txt"), []byte("dirty\n"), paths.FilePerm600); err != nil {
					t.Fatal(err)
				}
			},
			wantErrSub: "left uncommitted work",
		},
		{
			name: "empty commit",
			mutate: func(t *testing.T, worktree string) {
				t.Helper()
				runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "--allow-empty", "-m", "empty")
			},
			wantErrSub: "changed no paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree := t.TempDir()
			runTestGit(t, worktree, "init")
			if err := fileutil.WriteFile(filepath.Join(worktree, "result.txt"), []byte("base\n"), paths.FilePerm600); err != nil {
				t.Fatal(err)
			}
			runTestGit(t, worktree, "add", "result.txt")
			runTestGit(t, worktree, "-c", "user.name=CAP Test", "-c", "user.email=cap@example.invalid", "commit", "-m", "base")
			baseSHA := runTestGit(t, worktree, "rev-parse", "HEAD")

			tt.mutate(t, worktree)
			_, err := collectOrchestrationCommitManifest(context.Background(), worktree, "ATK-fault", baseSHA)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErrSub)
			}
		})
	}
}

func TestConfigureOrchestrationExecutorProcessFailsClosed(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-c", "exit 0")
	configureOrchestrationExecutorProcess(cmd)

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("executor must run in its own process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("executor must install process-group cancellation")
	}
	if err := cmd.Cancel(); err != os.ErrProcessDone {
		t.Fatalf("cancel before start = %v, want %v", err, os.ErrProcessDone)
	}
	if cmd.WaitDelay != 5*time.Second {
		t.Fatalf("wait delay = %v, want 5s", cmd.WaitDelay)
	}
}

func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestSeedAgentWorktreeRuntime(t *testing.T) {
	t.Parallel()

	const taskID = "ATK-TEST"
	mainRoot := t.TempDir()
	worktreeRoot := t.TempDir()
	accountDir := filepath.Join(mainRoot, paths.ProcessDir, "accounts")
	personaDir := filepath.Join(mainRoot, paths.ProcessDir, "personas")
	draftDir := filepath.Join(storage.ObjectDraftPlaneRoot(mainRoot), objects.KindAgentTask)
	for _, dir := range []string{accountDir, personaDir, draftDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
	}
	if err := fileutil.WriteFile(filepath.Join(accountDir, ".account.index"), []byte("account-index"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(personaDir, ".persona.index"), []byte("persona-index"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	taskData := []byte("id: ATK-TEST\n")
	if err := fileutil.WriteFile(filepath.Join(draftDir, taskID+".yaml"), taskData, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	if err := seedAgentWorktreeRuntime(mainRoot, worktreeRoot); err != nil {
		t.Fatalf("seedAgentWorktreeRuntime: %v", err)
	}
	for _, rel := range []string{
		filepath.Join(paths.ProcessDir, "accounts", ".account.index"),
		filepath.Join(paths.ProcessDir, "personas", ".persona.index"),
	} {
		if _, err := fileutil.Stat(filepath.Join(worktreeRoot, rel)); err != nil {
			t.Fatalf("runtime index %s unavailable: %v", rel, err)
		}
	}
	worktreeDraft := storage.ObjectDraftPlaneRoot(worktreeRoot)
	info, err := fileutil.Lstat(worktreeDraft)
	if err != nil {
		t.Fatalf("isolated draft plane missing: %v", err)
	}
	if info.Mode()&fileutil.ModeSymlink != 0 {
		t.Fatal("worktree draft plane must not be a symlink to studio object_drafts")
	}
	if !info.IsDir() {
		t.Fatalf("worktree draft plane is %v, want directory", info.Mode())
	}
	if _, err := fileutil.Stat(filepath.Join(worktreeDraft, objects.KindAgentTask, taskID+".yaml")); !fileutil.IsNotExist(err) {
		t.Fatalf("studio draft task leaked into worktree: %v", err)
	}
}

func TestOrchestrationExecutorDirtIgnoresDraftPlane(t *testing.T) {
	t.Parallel()
	drafts := paths.ObjectDraftsRel()
	if got := orchestrationExecutorDirt("?? " + drafts); got != "" {
		t.Fatalf("symlink porcelain = %q, want empty", got)
	}
	if got := orchestrationExecutorDirt("?? " + drafts + "/\n M result.txt"); !strings.Contains(got, "result.txt") || strings.Contains(got, paths.ObjectDraftsDir) {
		t.Fatalf("kept dirt = %q, want result.txt only", got)
	}
}

func findRepoRoot(t *testing.T) string {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			return repoRoot
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatal("could not locate repo root containing go.mod")
		}
		repoRoot = parent
	}
}

// Not parallel: PrepareIsolatedTempProject binds ZQK_TEST_ROOT with t.Setenv.
//
// This test used to build its root by symlinking the real repo's .zqk into a t.TempDir. The draft
// plane lives under .zqk/object_drafts, so the symlink meant every run created a permanent
// agent_task in the developer's own project state — nine had accumulated before anyone looked.
// Seeding the schema plane copies what the test needs to read instead of aliasing what it writes.
func TestWaitForAgentTaskReadableDraftPlane(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "agent_draft_plane",
		SeedSchemaPlane: true,
	})
	root, fs := proj.Root, proj.FileStorage

	taskID := fmt.Sprintf("ATK-DRAFT-READABLE-%d", time.Now().UnixNano())
	task := map[string]any{
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyTitle:              "Draft Readable Test",
		objects.FieldKeyStatus:             objects.ObjectStatusProposed,
		objects.FieldKeyAssigneePersonaRef: "PER-CODER",
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	if err := fs.Create(ctx, secCtx, task); err != nil {
		t.Fatalf("fs.Create draft task: %v", err)
	}

	// Verify task file exists on draft plane
	draftPath := storage.ObjectDraftPlanePath(root, objects.KindAgentTask, taskID)
	if _, err := fileutil.Stat(draftPath); err != nil {
		t.Errorf("expected draft task file %s to exist: %v", draftPath, err)
	}

	// waitForAgentTaskReadable must succeed for draft plane tasks (dual-read)
	if err := waitForAgentTaskReadable(ctx, fs, secCtx, taskID); err != nil {
		t.Fatalf("waitForAgentTaskReadable failed on draft-plane task: %v", err)
	}
}

func TestOrchestrationTaskOpts_ClassifiesWorkClass(t *testing.T) {
	t.Parallel()

	state := &orchestratorState{
		title:  "Plan Title",
		planID: "PRI-1",
	}

	codingOpts := orchestrationTaskOpts(state, "worker", "PER-CODER", "coding", "Implement pkg/storage feature")
	if !codingOpts.IncludeTDD || !codingOpts.IncludeObserver {
		t.Fatalf("coding task must have IncludeTDD=true and IncludeObserver=true: got tdd=%v, observer=%v",
			codingOpts.IncludeTDD, codingOpts.IncludeObserver)
	}

	docsOpts := orchestrationTaskOpts(state, "worker", "PER-EVAL", "docs_eval", "CEF evaluate docs/quality/cef-runs/2026-09-04")
	if docsOpts.IncludeTDD || docsOpts.IncludeObserver {
		t.Fatalf("docs_eval task must have IncludeTDD=false and IncludeObserver=false: got tdd=%v, observer=%v",
			docsOpts.IncludeTDD, docsOpts.IncludeObserver)
	}
}

func TestResolveOrchestrationTimeout(t *testing.T) {
	t.Parallel()

	// 1. Explicit opts.Timeout takes precedence
	optsTimeout := 2 * time.Hour
	got := resolveOrchestrationTimeout(optsTimeout, nil)
	if got != 2*time.Hour {
		t.Fatalf("resolveOrchestrationTimeout with opts.Timeout got %v, want %v", got, 2*time.Hour)
	}

	// 2. Command flag takes precedence when opts.Timeout is 0
	cmd := NewOrchestrateCmd()
	if err := cmd.Flags().Set(cli.FlagTimeout, "3h"); err != nil {
		t.Fatalf("failed to set timeout flag: %v", err)
	}
	got = resolveOrchestrationTimeout(0, cmd)
	if got != 3*time.Hour {
		t.Fatalf("resolveOrchestrationTimeout with cmd flag got %v, want %v", got, 3*time.Hour)
	}

	// 3. Fallback to default orchestrationTimeout (4 hours)
	emptyCmd := NewOrchestrateCmd()
	got = resolveOrchestrationTimeout(0, emptyCmd)
	if got != 4*time.Hour {
		t.Fatalf("resolveOrchestrationTimeout fallback got %v, want %v", got, 4*time.Hour)
	}

	// 4. Fallback when cmd is nil
	got = resolveOrchestrationTimeout(0, nil)
	if got != 4*time.Hour {
		t.Fatalf("resolveOrchestrationTimeout nil cmd fallback got %v, want %v", got, 4*time.Hour)
	}
}

func TestNewOrchestrateCmd_TimeoutFlagRegistered(t *testing.T) {
	t.Parallel()

	cmd := NewOrchestrateCmd()
	flag := cmd.Flags().Lookup(cli.FlagTimeout)
	if flag == nil {
		t.Fatalf("expected %q flag to be registered on NewOrchestrateCmd", cli.FlagTimeout)
	}

	if err := cmd.ParseFlags([]string{"--timeout", "90m"}); err != nil {
		t.Fatalf("failed to parse --timeout flag: %v", err)
	}

	parsedTimeout := cli.GetTimeout(cmd)
	if parsedTimeout != 90*time.Minute {
		t.Fatalf("cli.GetTimeout(cmd) = %v, want %v", parsedTimeout, 90*time.Minute)
	}
}

func TestBuildOrchestrationExecutorArgs(t *testing.T) {
	t.Parallel()

	// With timeout
	args := buildOrchestrationExecutorArgs("ATK-123", "do work", 4*time.Hour)
	expected := []string{"agent", "execute", "--task-id", "ATK-123", "--prompt", "do work", "--timeout", "4h0m0s"}
	if len(args) != len(expected) {
		t.Fatalf("buildOrchestrationExecutorArgs len = %d, want %d: %v", len(args), len(expected), args)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], expected[i])
		}
	}

	// Without timeout
	argsNoTimeout := buildOrchestrationExecutorArgs("ATK-456", "prompt", 0)
	if len(argsNoTimeout) != 6 {
		t.Fatalf("buildOrchestrationExecutorArgs len = %d, want 6: %v", len(argsNoTimeout), argsNoTimeout)
	}
}

func TestOrchestrationExecutorChildEnv_bindsSeatedKernelNotWorktree(t *testing.T) {
	t.Parallel()

	kernel := "/Users/lanceettl/zqk-public-candidate"
	worktree := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "repo", "ATK-1789868091203586000-7b8ea42c")
	parent := []string{
		"PATH=/usr/bin",
		"ZQK_API_KEY=ACC-PARENT",
		"ZQK_PROJECT_ROOT=" + worktree,
	}
	seatKey := authcred.DefaultSwarmWorkerAccount
	zqkBin := filepath.Join(kernel, "bin", "zqk")

	out := orchestrationExecutorChildEnv(parent, kernel, seatKey, zqkBin)

	if containsEnvLine(out, "ZQK_PROJECT_ROOT="+worktree) {
		t.Fatalf("child ZQK_PROJECT_ROOT must not be the ATK worktree: %v", out)
	}
	if !containsEnvLine(out, "ZQK_PROJECT_ROOT="+kernel) {
		t.Fatalf("child ZQK_PROJECT_ROOT must be seated kernel %q, got %v", kernel, out)
	}
	if containsEnvLine(out, "ZQK_API_KEY=ACC-PARENT") {
		t.Fatalf("parent API key leaked: %v", out)
	}
	if !containsEnvLine(out, "ZQK_API_KEY="+seatKey) {
		t.Fatalf("missing seat API key: %v", out)
	}
	if !containsEnvLine(out, zqkenv.Bin().Name()+"="+zqkBin) {
		t.Fatalf("missing zqk bin: %v", out)
	}
}

func TestOrchestrateRun_executorDoesNotBindProjectRootToWorktree(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("orchestrate_run.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if strings.Contains(src, "WithSeatAPIKeyEnv(os.Environ(), seatKey, worktreePath)") {
		t.Fatal("orchestrate_run.go still injects ZQK_PROJECT_ROOT=worktreePath; ATK execute then misses the seated account index")
	}
}

func containsEnvLine(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
