package system

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewStateCommitCmd(t *testing.T) {
	t.Parallel()
	cmd := NewStateCommitCmd()
	if cmd == nil {
		t.Fatal("NewStateCommitCmd() returned nil")
	}

	if cmd.Use != "state-commit" {
		t.Errorf("Expected command use to be 'state-commit', got '%s'", cmd.Use)
	}

	if cmd.Flags().Lookup("snapshot-file") == nil {
		t.Error("Command should have --snapshot-file flag")
	}
}

func TestStateCommit_OperationalDynamicProof_Execute(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755))

	snapshotFile := filepath.Join(tmpDir, "output-state.csnap")
	backupDir := filepath.Join(tmpDir, "backups")

	cmd := NewStateCommitCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{
		"--snapshot-file", snapshotFile,
		"--backup-dir", backupDir,
	})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "State committed to")

	// Verify file was written
	require.True(t, fileutil.Exists(snapshotFile))
	cs, err := storage.ReadCompressedSnapshot(snapshotFile)
	require.NoError(t, err)
	assert.NotNil(t, cs)

	// Second execution: triggers archive of prior tip
	var buf2 bytes.Buffer
	cmd2 := NewStateCommitCmd()
	cmd2.SilenceErrors = true
	cmd2.SilenceUsage = true
	cmd2.SetOut(&buf2)
	cmd2.SetErr(&buf2)
	cli.SetContext(cmd2, cli.ContextForProjectRoot(tmpDir))
	cmd2.SetArgs([]string{
		"--snapshot-file", snapshotFile,
		"--backup-dir", backupDir,
		"--backup-keep", "2",
	})

	err2 := cmd2.Execute()
	require.NoError(t, err2)
	output2 := buf2.String()
	assert.Contains(t, output2, "Archived prior tip")
}
