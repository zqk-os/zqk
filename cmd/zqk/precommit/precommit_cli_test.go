package precommit

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/precommit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func TestNewPreCommitCmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewPreCommitCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "pre-commit", cmd.Name())

	expected := []string{
		"aggregate",
		"write-result",
		"write-result-from-last",
		"status",
		"clear",
		"lint-report",
		"policy-report",
		"integrity-report",
	}

	subs := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subs {
		subNames[sub.Name()] = true
	}

	for _, name := range expected {
		assert.True(t, subNames[name], "pre-commit command must contain subcommand: %s", name)
	}
}

func TestPreCommit_ResolveProjectRoot(t *testing.T) {
	dir := t.TempDir()

	// 1. With explicit --project-root flag
	cmd := NewAggregateCmd()
	require.NoError(t, cmd.ParseFlags([]string{"--project-root", dir}))
	root, err := resolvePreCommitProjectRoot(cmd)
	require.NoError(t, err)
	assert.Equal(t, dir, root)

	// 2. With fallback to current project root when flag is omitted
	cmd2 := NewAggregateCmd()
	cmd2.SetArgs([]string{})
	root2, err := resolvePreCommitProjectRoot(cmd2)
	require.NoError(t, err)
	assert.NotEmpty(t, root2)
}

func TestPreCommit_ResolveCategory_MissingCategory(t *testing.T) {
	dir := t.TempDir()
	cmd := NewWriteResultCmd()
	cmd.SetArgs([]string{"--project-root", dir})

	_, _, err := resolvePreCommitCategory(cmd, "test usage")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--category is required")
}

func TestPreCommit_AggregateAndClear(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", dir)

	// Write a category result first
	res := precommit.CategoryResult{
		OK:        true,
		Summary:   "all checks passed",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	err := precommit.WriteCategory(dir, "lint", res)
	require.NoError(t, err)

	// Run aggregate
	aggCmd := NewAggregateCmd()
	out := bytes.NewBuffer(nil)
	aggCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	aggCmd.SetOut(out)
	aggCmd.SetErr(out)
	aggCmd.SetArgs([]string{"--project-root", dir})

	err = aggCmd.Execute()
	require.NoError(t, err)

	// Verify aggregated file exists
	agg, err := precommit.ReadAggregated(dir)
	require.NoError(t, err)
	assert.False(t, agg.Block)
	assert.Contains(t, agg.Categories, "lint")

	// Run clear
	clearCmd := NewClearCmd()
	clearOut := bytes.NewBuffer(nil)
	clearCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), clearOut))
	clearCmd.SetOut(clearOut)
	clearCmd.SetErr(clearOut)
	clearCmd.SetArgs([]string{"--project-root", dir})

	err = clearCmd.Execute()
	require.NoError(t, err)

	// Status after clear reflects unblocked commit state
	statusCmd := NewStatusCmd()
	statusOut := bytes.NewBuffer(nil)
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), statusOut))
	statusCmd.SetOut(statusOut)
	statusCmd.SetErr(statusOut)
	statusCmd.SetArgs([]string{"--project-root", dir})

	err = statusCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, statusOut.String(), "Block:        false")
	assert.Contains(t, statusOut.String(), "Commit would be allowed.")
}

func TestPreCommit_WriteResult(t *testing.T) {
	dir := t.TempDir()

	cmd := NewWriteResultCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{
		"--project-root", dir,
		"--category", "policy",
		"--ok=false",
		"--summary", "policy check violation",
		"--blocking=true",
		"--details", "rule violation 1",
		"--details", "rule violation 2",
	})

	err := cmd.Execute()
	require.NoError(t, err)

	// Verify category file written
	cat, err := precommit.ReadCategory(dir, "policy")
	require.NoError(t, err)
	assert.False(t, cat.OK)
	assert.Equal(t, "policy check violation", cat.Summary)
	assert.True(t, cat.Blocking)
	assert.Len(t, cat.Details, 2)
}

func TestPreCommit_WriteResultFromLast(t *testing.T) {
	dir := t.TempDir()

	// 1. When staging file doesn't exist
	cmdFail := NewWriteResultFromLastCmd()
	outFail := bytes.NewBuffer(nil)
	cmdFail.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outFail))
	cmdFail.SetOut(outFail)
	cmdFail.SetErr(outFail)
	cmdFail.SetArgs([]string{"--project-root", dir, "--category", "lint"})

	err := cmdFail.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read last result for lint")

	// 2. When staging file exists
	lastRes := precommit.CategoryResult{
		OK:        false,
		Summary:   "3 lint issues found",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	data, err := json.Marshal(lastRes)
	require.NoError(t, err)
	require.NoError(t, fileutil.MkdirAll(precommit.CategoryDir(dir), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(precommit.LastResultPath(dir, "lint"), data, paths.FilePerm600))

	cmdSuccess := NewWriteResultFromLastCmd()
	outSuccess := bytes.NewBuffer(nil)
	cmdSuccess.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outSuccess))
	cmdSuccess.SetOut(outSuccess)
	cmdSuccess.SetErr(outSuccess)
	cmdSuccess.SetArgs([]string{"--project-root", dir, "--category", "lint"})

	err = cmdSuccess.Execute()
	require.NoError(t, err)

	// Verify aggregated was run and blocks
	agg, err := precommit.ReadAggregated(dir)
	require.NoError(t, err)
	assert.True(t, agg.Block)
	assert.Contains(t, agg.Categories, "lint")
}

func TestPreCommit_Status_BlockingAndAllowed(t *testing.T) {
	dir := t.TempDir()

	// 1. Status with blocking failure
	resBlock := precommit.CategoryResult{
		OK:        false,
		Summary:   "failing architecture invariant",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	err := precommit.WriteCategory(dir, "integrity", resBlock)
	require.NoError(t, err)
	_, err = precommit.Aggregate(dir)
	require.NoError(t, err)

	statusCmd := NewStatusCmd()
	statusOut := bytes.NewBuffer(nil)
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), statusOut))
	statusCmd.SetOut(statusOut)
	statusCmd.SetErr(statusOut)
	statusCmd.SetArgs([]string{"--project-root", dir})

	err = statusCmd.Execute()
	require.NoError(t, err)
	outputStr := statusOut.String()
	assert.Contains(t, outputStr, "Block:        true")
	assert.Contains(t, outputStr, "Blocking categories:")
	assert.Contains(t, outputStr, "integrity: failing architecture invariant")

	// 2. Status with passing verdict
	resPass := precommit.CategoryResult{
		OK:        true,
		Summary:   "all checks clean",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	err = precommit.WriteCategory(dir, "integrity", resPass)
	require.NoError(t, err)
	_, err = precommit.Aggregate(dir)
	require.NoError(t, err)

	statusCmd2 := NewStatusCmd()
	statusOut2 := bytes.NewBuffer(nil)
	statusCmd2.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), statusOut2))
	statusCmd2.SetOut(statusOut2)
	statusCmd2.SetErr(statusOut2)
	statusCmd2.SetArgs([]string{"--project-root", dir})

	err = statusCmd2.Execute()
	require.NoError(t, err)
	outputStr2 := statusOut2.String()
	assert.Contains(t, outputStr2, "Block:        false")
	assert.Contains(t, outputStr2, "Commit would be allowed.")
}

func TestPreCommit_Reports_MissingAndPresent(t *testing.T) {
	dir := t.TempDir()

	// 1. Lint report missing & present
	lintCmd := NewLintReportCmd()
	outLintMissing := bytes.NewBuffer(nil)
	lintCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outLintMissing))
	lintCmd.SetOut(outLintMissing)
	lintCmd.SetErr(outLintMissing)
	lintCmd.SetArgs([]string{"--project-root", dir})
	err := lintCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outLintMissing.String(), "No lint output yet")

	lintPath := precommit.LintOutputPath(dir)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(lintPath), paths.FilePerm755))
	require.NoError(t, fileutil.WriteFile(lintPath, []byte("pkg/foo.go:10: lint error"), paths.FilePerm644))

	lintCmdPresent := NewLintReportCmd()
	outLintPresent := bytes.NewBuffer(nil)
	lintCmdPresent.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outLintPresent))
	lintCmdPresent.SetOut(outLintPresent)
	lintCmdPresent.SetErr(outLintPresent)
	lintCmdPresent.SetArgs([]string{"--project-root", dir})
	err = lintCmdPresent.Execute()
	require.NoError(t, err)
	assert.Contains(t, outLintPresent.String(), "pkg/foo.go:10: lint error")

	// 2. Policy report missing & present
	policyCmd := NewPolicyReportCmd()
	outPolicyMissing := bytes.NewBuffer(nil)
	policyCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outPolicyMissing))
	policyCmd.SetOut(outPolicyMissing)
	policyCmd.SetErr(outPolicyMissing)
	policyCmd.SetArgs([]string{"--project-root", dir})
	err = policyCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outPolicyMissing.String(), "No policy output yet")

	policyPath := precommit.PolicyOutputPath(dir)
	require.NoError(t, fileutil.WriteFile(policyPath, []byte("pkg/bar.go: violation of POL-001"), paths.FilePerm644))

	policyCmdPresent := NewPolicyReportCmd()
	outPolicyPresent := bytes.NewBuffer(nil)
	policyCmdPresent.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outPolicyPresent))
	policyCmdPresent.SetOut(outPolicyPresent)
	policyCmdPresent.SetErr(outPolicyPresent)
	policyCmdPresent.SetArgs([]string{"--project-root", dir})
	err = policyCmdPresent.Execute()
	require.NoError(t, err)
	assert.Contains(t, outPolicyPresent.String(), "pkg/bar.go: violation of POL-001")

	// 3. Integrity report missing & present
	integrityCmd := NewIntegrityReportCmd()
	outIntegrityMissing := bytes.NewBuffer(nil)
	integrityCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outIntegrityMissing))
	integrityCmd.SetOut(outIntegrityMissing)
	integrityCmd.SetErr(outIntegrityMissing)
	integrityCmd.SetArgs([]string{"--project-root", dir})
	err = integrityCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outIntegrityMissing.String(), "No integrity output yet")

	integrityPath := precommit.IntegrityOutputPath(dir)
	require.NoError(t, fileutil.WriteFile(integrityPath, []byte("hash mismatch on CAS object"), paths.FilePerm644))

	integrityCmdPresent := NewIntegrityReportCmd()
	outIntegrityPresent := bytes.NewBuffer(nil)
	integrityCmdPresent.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outIntegrityPresent))
	integrityCmdPresent.SetOut(outIntegrityPresent)
	integrityCmdPresent.SetErr(outIntegrityPresent)
	integrityCmdPresent.SetArgs([]string{"--project-root", dir})
	err = integrityCmdPresent.Execute()
	require.NoError(t, err)
	assert.Contains(t, outIntegrityPresent.String(), "hash mismatch on CAS object")
}
