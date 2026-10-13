package system

import (
	"bytes"
	"context"
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
	tmpDir := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	factory, err := storage.NewStorageFactory(t.Context(), tmpDir)
	if err != nil {
		t.Fatalf("failed to create StorageFactory: %v", err)
	}
	t.Cleanup(func() {
		if err := factory.Shutdown(context.Background()); err != nil {
			t.Logf("factory shutdown: %v", err)
		}
	})

	provider := factory.GetStorage()
	fileStorage := storage.UnwrapToFileObjectStorage(provider)
	if fileStorage == nil {
		t.Fatalf("UnwrapToFileObjectStorage returned nil for factory default storage")
	}

	if fileStorage.GetProjectRoot() != tmpDir {
		t.Errorf("expected project root %s, got %s", tmpDir, fileStorage.GetProjectRoot())
	}
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
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	cmd := NewStateRestoreCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--input", filepath.Join(tmpDir, "missing_snapshot.csnap")})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestStateRestore_OperationalDynamicProof_RestoreSnapshot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755))

	objs := []map[string]any{
		{
			"id":          "GOAL-RESTORE-1",
			"kind":        "goal",
			"title":       "Restored Goal",
			"status":      "open",
			"description": "A goal restored from compressed snapshot",
		},
	}

	logger := logging.GetLoggerFromProfile("debug")
	now := time.Now().UTC()
	cs, err := storage.CreateCompressedSnapshot(objs, now, logger)
	require.NoError(t, err)

	snapshotPath := filepath.Join(tmpDir, "restore_test.csnap")
	require.NoError(t, storage.WriteCompressedSnapshot(cs, snapshotPath))

	cmd := NewStateRestoreCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--input", snapshotPath})

	err = cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Restore complete")
	assert.Contains(t, output, "1 objects written")
}

func TestStateRestore_NegativeAdversarialBoundary_PruneWithoutConfirmPrune(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755))

	objs := []map[string]any{
		{"id": "GOAL-RESTORE-2", "kind": "goal", "title": "Goal 2"},
	}
	logger := logging.GetLoggerFromProfile("debug")
	cs, err := storage.CreateCompressedSnapshot(objs, time.Now().UTC(), logger)
	require.NoError(t, err)

	snapshotPath := filepath.Join(tmpDir, "restore_prune_test.csnap")
	require.NoError(t, storage.WriteCompressedSnapshot(cs, snapshotPath))

	cmd := NewStateRestoreCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--input", snapshotPath, "--prune"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--prune requires --confirm-prune")
}

func TestStateRestore_OperationalDynamicProof_PruneWithConfirmPrune(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	processDir := filepath.Join(tmpDir, paths.ProcessDir)
	require.NoError(t, fileutil.MkdirAll(processDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755))

	orphanDir := filepath.Join(processDir, "custom")
	require.NoError(t, fileutil.MkdirAll(orphanDir, paths.DirPerm755))
	orphanFile := filepath.Join(orphanDir, "orphan.yaml")
	require.NoError(t, fileutil.WriteFile(orphanFile, []byte("title: orphan"), paths.FilePerm644))

	objs := []map[string]any{
		{"id": "GOAL-RESTORE-3", "kind": "goal", "title": "Goal 3"},
	}
	logger := logging.GetLoggerFromProfile("debug")
	cs, err := storage.CreateCompressedSnapshot(objs, time.Now().UTC(), logger)
	require.NoError(t, err)

	snapshotPath := filepath.Join(tmpDir, "restore_prune_confirm_test.csnap")
	require.NoError(t, storage.WriteCompressedSnapshot(cs, snapshotPath))

	cmd := NewStateRestoreCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--input", snapshotPath, "--prune", "--confirm-prune"})

	err = cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Restore complete")
	assert.False(t, fileutil.Exists(orphanFile))
}

