package mutate_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/mutate"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
)

const (
	testValidScript = "BEGIN; LET $p = UPSERT priority_plan { title: 'Stdin Plan', priority_tier: 'P1', status: 'in_progress' }; COMMIT;"
	testDeletePlan  = "BEGIN; DELETE priority_plan 'PRI-DELETE-001'; COMMIT;"
	testCreatePlan  = "BEGIN; UPSERT priority_plan { id: 'PRI-DELETE-001', title: 'To Delete', priority_tier: 'P1', status: 'originated' }; COMMIT;"
)

func setupMutateTestProject(t *testing.T) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
		if cleanupErr != nil {
			t.Logf("cleanup teardown notice: %v", cleanupErr)
		}
	})
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	repoRoot, err := findModuleRoot()
	require.NoError(t, err)
	bootstrapErr := testenvroot.BootstrapRoot(tmpDir, repoRoot)
	require.NoError(t, bootstrapErr)
	copyLifecycleErr := testenvroot.CopyLifecyclesFromProject(tmpDir, repoRoot)
	require.NoError(t, copyLifecycleErr)
	copySpecErr := testenvroot.CopyObjectSpecsFromProject(tmpDir, repoRoot)
	require.NoError(t, copySpecErr)

	return tmpDir, repoRoot
}

func executeMutateScript(t *testing.T, stdinContent string, args ...string) (string, error) {
	t.Helper()
	cmd := mutate.NewMutateCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	if stdinContent != "" {
		cmd.SetIn(strings.NewReader(stdinContent))
	}
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()
	return outBuf.String(), cmdErr
}

func TestMutateCmd_StdinInput(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
		if cleanupErr != nil {
			t.Logf("cleanup teardown notice: %v", cleanupErr)
		}
	})
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	t.Run("stdin_via_file_dash", func(t *testing.T) {
		out, err := executeMutateScript(t, testValidScript, "-f", "-", "--dry-run", "--format", "json")
		require.NoError(t, err)
		assert.Contains(t, out, `"transaction_id"`)
	})

	t.Run("stdin_via_arg_dash", func(t *testing.T) {
		out, err := executeMutateScript(t, testValidScript, "-", "--dry-run", "--format", "json")
		require.NoError(t, err)
		assert.Contains(t, out, `"transaction_id"`)
	})
}

func TestMutateCmd_MissingAndInvalidScripts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
		if cleanupErr != nil {
			t.Logf("cleanup teardown notice: %v", cleanupErr)
		}
	})
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	t.Run("no_script_provided", func(t *testing.T) {
		_, err := executeMutateScript(t, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutation script required as argument or via -f/--file")
	})

	t.Run("missing_file_error", func(t *testing.T) {
		_, err := executeMutateScript(t, "", "-f", "nonexistent_file_path_12345.zql")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read mutation script file")
	})

	t.Run("syntax_error", func(t *testing.T) {
		_, err := executeMutateScript(t, "", "INVALID SYNTAX IN SCRIPT")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ZQL syntax error")
	})
}

func TestMutateCmd_DeletePersistenceAndTableFormat(t *testing.T) {
	_, _ = setupMutateTestProject(t)

	// Step 1: Create object
	createOut, createErr := executeMutateScript(t, "", testCreatePlan, "--format", "table")
	require.NoError(t, createErr)
	assert.Contains(t, createOut, "COMMITTED")

	// Step 2: Delete object via ZQL
	deleteOut, deleteErr := executeMutateScript(t, "", testDeletePlan, "--format", "table")
	require.NoError(t, deleteErr)
	assert.Contains(t, deleteOut, "COMMITTED")
	assert.Contains(t, deleteOut, "DELETE")
}
