package mutate_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/mutate"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMutateCmd_Help(t *testing.T) {
	cmd := mutate.NewMutateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "ZQL")
}

func TestMutateCmd_DryRunFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	scriptFile := filepath.Join(tmpDir, "test.zql")
	script := `
		BEGIN;
		LET $p = UPSERT priority_plan {
			title: "Dry Run Plan",
			priority_tier: "P1",
			status: "in_progress"
		};
		COMMIT;
	`
	err := fileutil.WriteFile(scriptFile, []byte(script), 0644)
	require.NoError(t, err)

	cmd := mutate.NewMutateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"-f", scriptFile, "--dry-run", "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"transaction_id"`)
	require.Contains(t, buf.String(), `"dry_run"`)
}

func TestMutateCmd_DirectString(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := mutate.NewMutateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"BEGIN; LET $p = UPSERT priority_plan { title: 'Direct Plan', priority_tier: 'P1', status: 'in_progress' }; COMMIT;", "--dry-run", "--format", "table"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Transaction")
	require.Contains(t, buf.String(), "DRY-RUN VALIDATED")
}

func TestMutateCmd_BreakGlassValidation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	t.Run("missing_reason_rejected", func(t *testing.T) {
		cmd := mutate.NewMutateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"BEGIN; UPSERT priority_plan { title: 'Test' }; COMMIT;", "--break-glass", "--dry-run"})
		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "--break-glass requires explicit non-empty justification")
	})

	t.Run("short_reason_rejected", func(t *testing.T) {
		cmd := mutate.NewMutateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"BEGIN; UPSERT priority_plan { title: 'Test' }; COMMIT;", "--break-glass", "--break-glass-reason", "short", "--dry-run"})
		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "justification reason too short")
	})
}

func TestMutateCmd_SystemFieldRejectionAndOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	scriptWithSystemField := `
		BEGIN;
		UPSERT backlog_item {
			title: 'Attempting to forge created_at',
			created_at: '2020-01-01T00:00:00Z',
			priority_tier: 'P1',
			status: 'planned'
		};
		COMMIT;
	`

	t.Run("fails_closed_without_break_glass", func(t *testing.T) {
		cmd := mutate.NewMutateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{scriptWithSystemField, "--dry-run"})
		err := cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "system_managed_field")
	})

	t.Run("succeeds_with_break_glass_and_valid_reason", func(t *testing.T) {
		cmd := mutate.NewMutateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{
			scriptWithSystemField,
			"--dry-run",
			"--break-glass",
			"--break-glass-reason", "emergency data repair approved by lead architect",
		})
		err := cmd.Execute()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "DRY-RUN VALIDATED")
	})
}

func findModuleRoot() (string, error) {
	dir, err := fileutil.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fileutil.ErrNotExist
		}
		dir = parent
	}
}

func TestMutateCmd_NonDryRun_CreateAndUpdatePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil)) })
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	repoRoot, err := findModuleRoot()
	require.NoError(t, err)
	require.NoError(t, testenvroot.BootstrapRoot(tmpDir, repoRoot))
	require.NoError(t, testenvroot.CopyLifecyclesFromProject(tmpDir, repoRoot))
	require.NoError(t, testenvroot.CopyObjectSpecsFromProject(tmpDir, repoRoot))

	// Step 1: Create a new priority_plan object via ZQL without dry-run
	cmdCreate := mutate.NewMutateCmd()
	var createBuf bytes.Buffer
	cmdCreate.SetOut(&createBuf)
	cmdCreate.SetArgs([]string{
		"BEGIN; LET $p = UPSERT priority_plan { id: 'PRI-TEST-PERSIST-001', title: 'Persisted Plan', priority_tier: 'P1', status: 'originated' }; COMMIT;",
		"--format", "table",
	})
	err = cmdCreate.Execute()
	require.NoError(t, err)
	require.Contains(t, createBuf.String(), "COMMITTED")

	// Step 2: Update the existing priority_plan object via ZQL without dry-run
	cmdUpdate := mutate.NewMutateCmd()
	var updateBuf bytes.Buffer
	cmdUpdate.SetOut(&updateBuf)
	cmdUpdate.SetArgs([]string{
		"BEGIN; UPSERT priority_plan { id: 'PRI-TEST-PERSIST-001', title: 'Updated Plan Title' }; COMMIT;",
		"--format", "table",
	})
	err = cmdUpdate.Execute()
	require.NoError(t, err)
	require.Contains(t, updateBuf.String(), "COMMITTED")
}
