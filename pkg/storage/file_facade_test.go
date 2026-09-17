package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestFileFacade_NewFileLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock, err := storage.NewFileLock(lockPath)
	require.NoError(t, err)
	require.NotNil(t, lock)

	strat := storage.NewAutoCleanupStrategy()
	require.NotNil(t, strat)

	metrics := storage.GetFileLockMetrics()
	require.NotNil(t, metrics)
}
