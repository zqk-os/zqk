package swarm

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupTestProjectRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	zqkDir := filepath.Join(dir, paths.ProjectDataDir)
	err := fileutil.MkdirAll(zqkDir, paths.DirPerm755)
	require.NoError(t, err)
	t.Setenv("ZQK_PROJECT_ROOT", dir)
	return dir
}

func TestSwarmDispatch_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewSwarmCmd()
	require.NotNil(t, cmd)

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["dispatch"], "swarm command must have dispatch subcommand")
}

func TestSwarmDispatch_DryRun(t *testing.T) {
	setupTestProjectRoot(t)

	cmd := NewDispatchCmd()
	cmd.SetArgs([]string{"--dry-run", "--token-budget", "30000", "--max-parallel", "3"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "SWARM DISPATCH")
	assert.Contains(t, got, "Dry-run validated successfully")
	assert.Contains(t, got, "10000 tokens/worker")
}

func TestSwarmDispatch_LiveExecution(t *testing.T) {
	setupTestProjectRoot(t)

	cmd := NewDispatchCmd()
	cmd.SetArgs([]string{"--token-budget", "45000", "--max-parallel", "3", "--entrypoint", "sprint-gate"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "Swarm runner active")
	assert.Contains(t, got, "AST_INVARIANTS=VALIDATED")
}

func TestSwarmDispatch_InvalidTokenBudget(t *testing.T) {
	setupTestProjectRoot(t)

	cmd := NewDispatchCmd()
	cmd.SetArgs([]string{"--token-budget", "0"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing or non-positive token budget")
}
