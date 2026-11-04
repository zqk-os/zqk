package precommit

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/precommit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func setupUnresolvableRoot(t *testing.T) {
	t.Helper()
	fakeWorktree := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "nonexistent-repo", "task-isolate")
	t.Setenv("ZQK_PROJECT_ROOT", fakeWorktree)
	t.Setenv("ZQK_TEST_ROOT", "")
}

func execPreCommitCmd(ctx context.Context, cmd *cobra.Command, args []string) (string, error) {
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, out))
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestElevation_ProjectRoot_UnresolvableError(t *testing.T) {
	setupUnresolvableRoot(t)

	// Verify resolvePreCommitProjectRoot returns error directly
	cmd := NewAggregateCmd()
	cmd.SetArgs([]string{})
	_, err := resolvePreCommitProjectRoot(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	// Verify subcommands fail with unresolvable project root
	cmds := []*cobra.Command{
		NewAggregateCmd(),
		NewClearCmd(),
		NewIntegrityReportCmd(),
		NewLintReportCmd(),
		NewPolicyReportCmd(),
		NewStatusCmd(),
		NewWriteResultCmd(),
		NewWriteResultFromLastCmd(),
	}

	for _, c := range cmds {
		_, execErr := execPreCommitCmd(context.Background(), c, []string{})
		require.Error(t, execErr, "command %s must fail on unresolvable root", c.Name())
		assert.Contains(t, execErr.Error(), "project root not found")
	}
}

func TestElevation_Reports_DirectoryReadError(t *testing.T) {
	dir := t.TempDir()

	// 1. Lint report directory error
	lintPath := precommit.LintOutputPath(dir)
	require.NoError(t, fileutil.MkdirAll(lintPath, paths.DirPerm755))
	_, errLint := execPreCommitCmd(context.Background(), NewLintReportCmd(), []string{"--project-root", dir})
	require.Error(t, errLint)
	assert.Contains(t, errLint.Error(), "read lint output")

	// 2. Policy report directory error
	policyPath := precommit.PolicyOutputPath(dir)
	require.NoError(t, fileutil.MkdirAll(policyPath, paths.DirPerm755))
	_, errPolicy := execPreCommitCmd(context.Background(), NewPolicyReportCmd(), []string{"--project-root", dir})
	require.Error(t, errPolicy)
	assert.Contains(t, errPolicy.Error(), "read policy output")

	// 3. Integrity report directory error
	integrityPath := precommit.IntegrityOutputPath(dir)
	require.NoError(t, fileutil.MkdirAll(integrityPath, paths.DirPerm755))
	_, errIntegrity := execPreCommitCmd(context.Background(), NewIntegrityReportCmd(), []string{"--project-root", dir})
	require.Error(t, errIntegrity)
	assert.Contains(t, errIntegrity.Error(), "read integrity output")
}

func TestElevation_Status_CorruptedAggregatedFile(t *testing.T) {
	dir := t.TempDir()
	resultsPath := precommit.AggregatedPath(dir)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(resultsPath), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(resultsPath, []byte("{invalid-json-payload"), paths.FilePerm644))

	_, err := execPreCommitCmd(context.Background(), NewStatusCmd(), []string{"--project-root", dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read results")
}

func TestElevation_Status_StatPermissionDenied(t *testing.T) {
	dir := t.TempDir()
	precommitDir := precommit.CategoryDir(dir)
	require.NoError(t, fileutil.MkdirAll(precommitDir, paths.DirPerm755))
	require.NoError(t, fileutil.Chmod(precommitDir, 0000))
	t.Cleanup(func() {
		require.NoError(t, fileutil.Chmod(precommitDir, paths.DirPerm755))
	})

	_, err := execPreCommitCmd(context.Background(), NewStatusCmd(), []string{"--project-root", dir})
	require.Error(t, err)
}

func TestElevation_WriteResult_CategoryWriteFailure(t *testing.T) {
	dir := t.TempDir()
	catPath := precommit.CategoryPath(dir, "lint")
	require.NoError(t, fileutil.MkdirAll(catPath, paths.DirPerm755))

	cmd := NewWriteResultCmd()
	args := []string{
		"--project-root", dir,
		"--category", "lint",
		"--ok=true",
		"--summary", "all pass",
	}
	_, err := execPreCommitCmd(context.Background(), cmd, args)
	require.Error(t, err)
}

func TestElevation_WriteResultFromLast_WriteCategoryFailure(t *testing.T) {
	dir := t.TempDir()
	lastRes := precommit.CategoryResult{
		OK:        true,
		Summary:   "ok summary",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	data, err := json.Marshal(lastRes)
	require.NoError(t, err)
	require.NoError(t, fileutil.MkdirAll(precommit.CategoryDir(dir), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(precommit.LastResultPath(dir, "lint"), data, paths.FilePerm600))

	// Collide category target as directory
	require.NoError(t, fileutil.MkdirAll(precommit.CategoryPath(dir, "lint"), paths.DirPerm755))

	cmd := NewWriteResultFromLastCmd()
	_, execErr := execPreCommitCmd(context.Background(), cmd, []string{"--project-root", dir, "--category", "lint"})
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "write category")
}

func TestElevation_WriteResultFromLast_AggregateFailure(t *testing.T) {
	dir := t.TempDir()
	lastRes := precommit.CategoryResult{
		OK:        true,
		Summary:   "ok summary",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	data, err := json.Marshal(lastRes)
	require.NoError(t, err)
	require.NoError(t, fileutil.MkdirAll(precommit.CategoryDir(dir), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(precommit.LastResultPath(dir, "lint"), data, paths.FilePerm600))

	// Collide results.json target as directory
	require.NoError(t, fileutil.MkdirAll(precommit.AggregatedPath(dir), paths.DirPerm755))

	cmd := NewWriteResultFromLastCmd()
	_, execErr := execPreCommitCmd(context.Background(), cmd, []string{"--project-root", dir, "--category", "lint"})
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "aggregate")
}

func TestElevation_Aggregate_Failure(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(precommit.AggregatedPath(dir), paths.DirPerm755))

	cmd := NewAggregateCmd()
	_, err := execPreCommitCmd(context.Background(), cmd, []string{"--project-root", dir})
	require.Error(t, err)
}

func TestElevation_Clear_Failure(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(precommit.AggregatedPath(dir), paths.DirPerm755))

	cmd := NewClearCmd()
	_, err := execPreCommitCmd(context.Background(), cmd, []string{"--project-root", dir})
	require.Error(t, err)
}

func TestElevation_Status_RollupDetailsAndAction(t *testing.T) {
	dir := t.TempDir()

	resIntegrity := precommit.CategoryResult{
		OK:        false,
		Summary:   "broken invariants",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	require.NoError(t, precommit.WriteCategory(dir, "integrity", resIntegrity))

	resDocman := precommit.CategoryResult{
		OK:        false,
		Summary:   "missing docstrings",
		Blocking:  true,
		UpdatedAt: zqktime.NowRFC3339UTC(),
	}
	require.NoError(t, precommit.WriteCategory(dir, "docman", resDocman))

	agg, err := precommit.Aggregate(dir)
	require.NoError(t, err)
	assert.True(t, agg.Block)

	out, statusErr := execPreCommitCmd(context.Background(), NewStatusCmd(), []string{"--project-root", dir})
	require.NoError(t, statusErr)
	assert.Contains(t, out, "Block:        true")
	assert.Contains(t, out, "Blocking categories:")
	assert.Contains(t, out, "integrity: broken invariants")
	assert.Contains(t, out, "docman: missing docstrings")
	assert.Contains(t, out, "Required action: Fix the issues above, then either:")
}

func TestElevation_FlagValidation_UnknownFlags(t *testing.T) {
	dir := t.TempDir()
	subcmds := []*cobra.Command{
		NewAggregateCmd(),
		NewClearCmd(),
		NewIntegrityReportCmd(),
		NewLintReportCmd(),
		NewPolicyReportCmd(),
		NewStatusCmd(),
		NewWriteResultCmd(),
		NewWriteResultFromLastCmd(),
	}

	for _, sc := range subcmds {
		_, err := execPreCommitCmd(context.Background(), sc, []string{"--project-root", dir, "--nonexistent-flag"})
		require.Error(t, err, "subcommand %s should reject unknown flag", sc.Name())
	}
}

func TestElevation_FormatOutput_StructuredAggregated(t *testing.T) {
	agg := &precommit.AggregatedResult{
		UpdatedAt: zqktime.NowRFC3339UTC(),
		Block:     false,
		Categories: map[string]precommit.CategoryResult{
			"lint": {
				OK:        true,
				Summary:   "clean",
				Blocking:  true,
				UpdatedAt: zqktime.NowRFC3339UTC(),
			},
		},
	}

	cmd := NewStatusCmd()
	outBuf := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outBuf))

	errTable := cli.FormatOutputAs(cmd, cli.FormatTable, agg)
	require.NoError(t, errTable)
	assert.NotEmpty(t, outBuf.String())

	outJSONBuf := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outJSONBuf))
	errJSON := cli.FormatOutputAs(cmd, cli.FormatJSON, agg)
	require.NoError(t, errJSON)
	assert.Contains(t, outJSONBuf.String(), `"block": false`)

	outYAMLBuf := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), outYAMLBuf))
	errYAML := cli.FormatOutputAs(cmd, cli.FormatYAML, agg)
	require.NoError(t, errYAML)
	assert.Contains(t, outYAMLBuf.String(), "block: false")
}
