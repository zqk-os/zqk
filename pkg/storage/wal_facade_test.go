package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestWALFacade_NewObjectWAL(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := storage.NewObjectWAL(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, wal)
	defer wal.Close()

	walPath := storage.GetWALPath(tmpDir)
	require.NotEmpty(t, walPath)
	require.Equal(t, filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir, "object.wal"), walPath)
}
