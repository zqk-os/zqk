package ci

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/localci"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCICmd_Checkout_SkippedBranch(t *testing.T) {
	repo, headSHA := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	// 1. Initial checkout promotes SHA to workdir
	cmd1 := NewCICmd()
	cmd1.SetArgs([]string{"checkout", "--sha", headSHA, "--no-archive"})
	out1 := bytes.NewBuffer(nil)
	cmd1.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out1))
	cmd1.SetOut(out1)
	cmd1.SetErr(out1)

	err := cmd1.Execute()
	require.NoError(t, err)
	assert.Contains(t, out1.String(), "local-ci checkout:")
	assert.NotContains(t, out1.String(), "skipped:")

	// 2. Second checkout of the exact same SHA triggers res.Skipped branch
	cmd2 := NewCICmd()
	cmd2.SetArgs([]string{"checkout", "--sha", headSHA, "--no-archive"})
	out2 := bytes.NewBuffer(nil)
	cmd2.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out2))
	cmd2.SetOut(out2)
	cmd2.SetErr(out2)

	err = cmd2.Execute()
	require.NoError(t, err)
	assert.Contains(t, out2.String(), "local-ci checkout skipped:")
	assert.Contains(t, out2.String(), "already checked out at")
}

func TestCICmd_Run_FullTestCasesPipeline(t *testing.T) {
	repo, headSHA := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)
	t.Cleanup(func() {
		err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(repo, nil))
		require.NoError(t, err)
	})

	// Execute `zqk ci run` without --checkout-only to exercise runTestCasesAfterCheckout
	runCmd := NewCICmd()
	runCmd.SetArgs([]string{"run", "--sha", headSHA, "--no-archive", "--allow-dirty"})
	out := bytes.NewBuffer(nil)
	runCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	runCmd.SetOut(out)
	runCmd.SetErr(out)

	err := runCmd.Execute()
	require.NoError(t, err)
	got := out.String()
	assert.Contains(t, got, "local-ci checkout:")
}

func TestCICmd_Status_GoModIsDirectory(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	base := localci.BaseDir(repo, "")
	err := fileutil.MkdirAll(base, paths.DirPerm755)
	require.NoError(t, err)

	shaPath := filepath.Join(base, paths.LocalCISourceSHAFile)
	err = fileutil.WriteFile(shaPath, []byte("11223344556677889900aabbccddeeff11223344\n"), paths.FilePerm644)
	require.NoError(t, err)

	// Create workdir and make go.mod a DIRECTORY instead of a regular file
	workdir := filepath.Join(base, paths.LocalCIWorkdirName)
	err = fileutil.MkdirAll(filepath.Join(workdir, "go.mod"), paths.DirPerm755)
	require.NoError(t, err)

	statusCmd := NewCICmd()
	statusCmd.SetArgs([]string{"status"})
	out := bytes.NewBuffer(nil)
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	statusCmd.SetOut(out)
	statusCmd.SetErr(out)

	err = statusCmd.Execute()
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "Local CI status")
	assert.Contains(t, got, "SOURCE_SHA=11223344556677889900aabbccddeeff11223344")
	assert.Contains(t, got, "workdir=(missing)")
}

func TestCICmd_Demote_ContextCancellation(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	demoteCmd := NewCICmd()
	demoteCmd.SetArgs([]string{"demote"})
	out := bytes.NewBuffer(nil)
	demoteCmd.SetOut(out)
	demoteCmd.SetErr(out)

	err := demoteCmd.ExecuteContext(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-ci demote failed")
}

func TestCICmd_Checkout_ContextCancellation(t *testing.T) {
	repo, headSHA := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	checkoutCmd := NewCICmd()
	checkoutCmd.SetArgs([]string{"checkout", "--sha", headSHA, "--no-archive"})
	out := bytes.NewBuffer(nil)
	checkoutCmd.SetOut(out)
	checkoutCmd.SetErr(out)

	err := checkoutCmd.ExecuteContext(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-ci checkout failed")
}

func TestCICmd_Subcommands_InvalidFlags(t *testing.T) {
	repo, _ := setupTestGitRepo(t)
	t.Setenv("ZQK_PROJECT_ROOT", repo)

	cases := [][]string{
		{"checkout", "--nonexistent-checkout-flag"},
		{"run", "--nonexistent-run-flag"},
		{"status", "--nonexistent-status-flag"},
		{"demote", "--nonexistent-demote-flag"},
	}

	for _, args := range cases {
		cmd := NewCICmd()
		cmd.SetArgs(args)
		out := bytes.NewBuffer(nil)
		cmd.SetOut(out)
		cmd.SetErr(out)

		err := cmd.Execute()
		require.Error(t, err, "expected error for args %v", args)
		assert.Contains(t, err.Error(), "unknown flag")
	}
}
