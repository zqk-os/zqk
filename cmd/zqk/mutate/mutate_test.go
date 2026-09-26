package mutate_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/mutate"
	"github.com/zqk-os/zqk/pkg/storage"
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

