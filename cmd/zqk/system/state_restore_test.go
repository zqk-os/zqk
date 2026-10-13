package system

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStateRestoreStorageUnwrap(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	fileStorage := storage.UnwrapToFileObjectStorage(proj.FileStorage)
	if fileStorage == nil {
		t.Fatalf("UnwrapToFileObjectStorage returned nil for isolated temp project storage")
	}

	if fileStorage.GetProjectRoot() != proj.Root {
		t.Errorf("expected project root %s, got %s", proj.Root, fileStorage.GetProjectRoot())
	}
}

func setupRestoreTest(t *testing.T) string {
	t.Helper()
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755))
	return tmpDir
}

func executeRestoreCmd(projectRoot string, args ...string) (string, error) {
	cmd := NewStateRestoreCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func createTestRestoreCSnap(t *testing.T, dir, filename string, objs []map[string]any) string {
	t.Helper()
	logger := logging.GetLoggerFromProfile("debug")
	now := time.Now().UTC()
	cs, err := storage.CreateCompressedSnapshot(objs, now, logger)
	require.NoError(t, err)

	snapshotPath := filepath.Join(dir, filename)
	require.NoError(t, storage.WriteCompressedSnapshot(cs, snapshotPath))
	return snapshotPath
}

func TestNewStateRestoreCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewStateRestoreCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "state-restore", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.NotNil(t, cmd.Flags().Lookup("input"))
	assert.NotNil(t, cmd.Flags().Lookup("prune"))
	assert.NotNil(t, cmd.Flags().Lookup("confirm-prune"))
}

func TestStateRestore_NegativeAdversarialBoundary_MissingInput(t *testing.T) {
	tmpDir := setupRestoreTest(t)

	_, err := executeRestoreCmd(tmpDir, "--input", filepath.Join(tmpDir, "missing_snapshot.csnap"))
	require.Error(t, err)
}

func TestStateRestore_OperationalDynamicProof_RestoreSnapshot(t *testing.T) {
	tmpDir := setupRestoreTest(t)

	objs := []map[string]any{
		{
			"id":          "GOAL-RESTORE-1",
			"kind":        "goal",
			"title":       "Restored Goal",
			"status":      "open",
			"description": "A goal restored from compressed snapshot",
		},
	}

	snapshotPath := createTestRestoreCSnap(t, tmpDir, "restore_test.csnap", objs)

	output, err := executeRestoreCmd(tmpDir, "--input", snapshotPath)
	require.NoError(t, err)

	assert.Contains(t, output, "Restore complete")
	assert.Contains(t, output, "1 objects written")
}

func TestStateRestore_NegativeAdversarialBoundary_PruneWithoutConfirmPrune(t *testing.T) {
	tmpDir := setupRestoreTest(t)

	objs := []map[string]any{
		{"id": "GOAL-RESTORE-2", "kind": "goal", "title": "Goal 2"},
	}

	snapshotPath := createTestRestoreCSnap(t, tmpDir, "restore_prune_test.csnap", objs)

	_, err := executeRestoreCmd(tmpDir, "--input", snapshotPath, "--prune")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--prune requires --confirm-prune")
}

func TestStateRestore_OperationalDynamicProof_PruneWithConfirmPrune(t *testing.T) {
	tmpDir := setupRestoreTest(t)

	orphanDir := filepath.Join(tmpDir, paths.ProcessDir, "custom")
	require.NoError(t, fileutil.MkdirAll(orphanDir, paths.DirPerm755))
	orphanFile := filepath.Join(orphanDir, "orphan.yaml")
	require.NoError(t, fileutil.WriteFile(orphanFile, []byte("title: orphan"), paths.FilePerm644))

	objs := []map[string]any{
		{"id": "GOAL-RESTORE-3", "kind": "goal", "title": "Goal 3"},
	}

	snapshotPath := createTestRestoreCSnap(t, tmpDir, "restore_prune_confirm_test.csnap", objs)

	output, err := executeRestoreCmd(tmpDir, "--input", snapshotPath, "--prune", "--confirm-prune")
	require.NoError(t, err)

	assert.Contains(t, output, "Restore complete")
	assert.False(t, fileutil.Exists(orphanFile))
}
