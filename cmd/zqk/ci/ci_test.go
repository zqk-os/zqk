package ci

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupTestGitRepo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s (%v)", strings.Join(args, " "), out, err)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "ci@localhost")
	runGit("config", "user.name", "ci")

	// Create .zqk directory so it's a valid project root
	if err := fileutil.MkdirAll(filepath.Join(repo, paths.ProjectDataDir), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}

	if err := fileutil.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/ci\n\ngo 1.22\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "go.mod")
	runGit("commit", "-q", "-m", "initial commit")

	headCmd := testkit.ManagedCommand(t, t.Context(), "git", "rev-parse", "HEAD")
	headCmd.Dir = repo
	out, err := headCmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	headSHA := strings.TrimSpace(string(out))
	return repo, headSHA
}

func TestNewCICmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewCICmd()
	require.NotNil(t, cmd, "NewCICmd must return non-nil command")
	assert.Equal(t, "ci", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["checkout"], "ci command must have checkout subcommand")
	assert.True(t, subNames["run"], "ci command must have run subcommand")
	assert.True(t, subNames["status"], "ci command must have status subcommand")
	assert.True(t, subNames["demote"], "ci command must have demote subcommand")
	assert.True(t, subNames["init"], "ci command must have init subcommand")
}

func TestCICmd_Status_Unset(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	cmd := NewCICmd()
	cmd.SetArgs([]string{"status"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "Local CI status")
	assert.Contains(t, got, "SOURCE_SHA=(not set")
	assert.Contains(t, got, "workdir=(missing)")
}

func TestCICmd_CheckoutAndStatusAndDemote_Lifecycle(t *testing.T) {
	repo, headSHA := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	// 1. Checkout
	checkoutCmd := NewCICmd()
	checkoutCmd.SetArgs([]string{"checkout", "--sha", headSHA, "--no-archive"})
	outCheckout := bytes.NewBuffer(nil)
	checkoutCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outCheckout))
	checkoutCmd.SetOut(outCheckout)
	checkoutCmd.SetErr(outCheckout)

	err := checkoutCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outCheckout.String(), "local-ci checkout:")
	assert.Contains(t, outCheckout.String(), headSHA)

	// 2. Status populated
	statusCmd := NewCICmd()
	statusCmd.SetArgs([]string{"status"})
	outStatus := bytes.NewBuffer(nil)
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outStatus))
	statusCmd.SetOut(outStatus)
	statusCmd.SetErr(outStatus)

	err = statusCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outStatus.String(), headSHA)
	assert.Contains(t, outStatus.String(), "workdir=")

	// 3. Demote
	demoteCmd := NewCICmd()
	demoteCmd.SetArgs([]string{"demote"})
	outDemote := bytes.NewBuffer(nil)
	demoteCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outDemote))
	demoteCmd.SetOut(outDemote)
	demoteCmd.SetErr(outDemote)

	err = demoteCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outDemote.String(), "local-ci demote: worktrees and pointer files removed")

	// 4. Status after demote is back to unset
	statusCmd2 := NewCICmd()
	statusCmd2.SetArgs([]string{"status"})
	outStatus2 := bytes.NewBuffer(nil)
	statusCmd2.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outStatus2))
	statusCmd2.SetOut(outStatus2)
	statusCmd2.SetErr(outStatus2)

	err = statusCmd2.Execute()
	require.NoError(t, err)
	assert.Contains(t, outStatus2.String(), "SOURCE_SHA=(not set")
}

func TestCICmd_Run_CheckoutOnly(t *testing.T) {
	repo, headSHA := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	runCmd := NewCICmd()
	runCmd.SetArgs([]string{"run", "--sha", headSHA, "--checkout-only", "--no-archive", "--allow-dirty"})
	out := bytes.NewBuffer(nil)
	runCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	runCmd.SetOut(out)
	runCmd.SetErr(out)

	err := runCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "local-ci checkout:")
}

func TestCICmd_Checkout_FailureCases(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	// Invalid SHA
	cmd := NewCICmd()
	cmd.SetArgs([]string{"checkout", "--sha", "deadbeef00000000000000000000000000000000"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-ci checkout failed")
}

func TestCICmd_Run_CheckoutFailure(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	// Run with invalid SHA fails at checkout stage
	runCmd := NewCICmd()
	runCmd.SetArgs([]string{"run", "--sha", "invalid-sha-12345"})
	out := bytes.NewBuffer(nil)
	runCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	runCmd.SetOut(out)
	runCmd.SetErr(out)

	err := runCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-ci checkout failed")
}

func TestCICmd_RunTestCasesAfterCheckout_MissingGoMod(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", repo)
	// Create .zqk so project root resolves, but don't create workdir/go.mod
	if err := fileutil.MkdirAll(filepath.Join(repo, paths.ProjectDataDir), paths.FilePerm755); err != nil {
		t.Fatal(err)
	}

	cmd := newRunCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := runTestCasesAfterCheckout(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-ci workdir missing go.mod")
}

func TestCICmd_MissingProjectRoot_Errors(t *testing.T) {
	worktreeRoot := filepath.Join(t.TempDir(), ".zqk", "worktrees", "isolated-seat")
	require.NoError(t, fileutil.MkdirAll(worktreeRoot, paths.DirPerm755))
	t.Setenv("ZQK_PROJECT_ROOT", worktreeRoot)

	checkoutCmd := newCheckoutCmd()
	err := runLocalCICheckout(checkoutCmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	statusCmd := newStatusCmd()
	err = runStatus(statusCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	demoteCmd := newDemoteCmd()
	err = runDemote(demoteCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	runCmd := newRunCmd()
	err = runTestCasesAfterCheckout(runCmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	initCmd := newInitCmd()
	err = runCIInit(initCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")
}

func TestCICmd_Init_DryRun(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	cmd := NewCICmd()
	cmd.SetArgs([]string{"init", "--dry-run"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "ci init (dry-run): would scaffold")

	// Ensure files were NOT created
	wfPath := filepath.Join(repo, ".github", "workflows", "zqk-audit.yml")
	assert.False(t, fileutil.Exists(wfPath))
}

func TestCICmd_Init_ScaffoldLifecycle(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	cmd := NewCICmd()
	cmd.SetArgs([]string{"init"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "ci init: created")

	wfPath := filepath.Join(repo, ".github", "workflows", "zqk-audit.yml")
	formPath := filepath.Join(repo, "Formula", "zqk.rb")
	assert.True(t, fileutil.Exists(wfPath))
	assert.True(t, fileutil.Exists(formPath))

	// Re-running without --force should fail
	cmd2 := NewCICmd()
	cmd2.SetArgs([]string{"init"})
	out2 := bytes.NewBuffer(nil)
	cmd2.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out2))
	cmd2.SetOut(out2)
	cmd2.SetErr(out2)

	err = cmd2.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Re-running with --force should succeed
	cmd3 := NewCICmd()
	cmd3.SetArgs([]string{"init", "--force"})
	out3 := bytes.NewBuffer(nil)
	cmd3.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out3))
	cmd3.SetOut(out3)
	cmd3.SetErr(out3)

	err = cmd3.Execute()
	require.NoError(t, err)
	assert.Contains(t, out3.String(), "ci init: created")
}

