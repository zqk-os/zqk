package mutate_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/mutate"
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
