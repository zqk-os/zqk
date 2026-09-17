package agentidle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestFileStore_Accumulate(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "idle.json")

	store, err := NewFileStore(storePath)
	require.NoError(t, err)

	err = store.Accumulate("agent-1", "task-a", 5*time.Second)
	require.NoError(t, err)

	err = store.Accumulate("agent-1", "task-a", 3*time.Second)
	require.NoError(t, err)

	err = store.Accumulate("agent-2", "task-b", 10*time.Second)
	require.NoError(t, err)

	idleTime, err := store.GetIdleTime("agent-1", "task-a")
	require.NoError(t, err)
	assert.Equal(t, 8*time.Second, idleTime)

	idleTime, err = store.GetIdleTime("agent-2", "task-b")
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, idleTime)
}

func TestFileStore_RestartSafe(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "idle.json")

	store1, err := NewFileStore(storePath)
	require.NoError(t, err)
	err = store1.Accumulate("agent-1", "task-a", 15*time.Second)
	require.NoError(t, err)

	// Create a new store instance pointing to the same file (simulating restart)
	store2, err := NewFileStore(storePath)
	require.NoError(t, err)

	idleTime, err := store2.GetIdleTime("agent-1", "task-a")
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, idleTime)
}

func TestFileStore_PermissionsAndSchema(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "idle.json")

	store, err := NewFileStore(storePath)
	require.NoError(t, err)

	err = store.Accumulate("agent-1", "task-a", 10*time.Second)
	require.NoError(t, err)

	info, err := fileutil.Stat(storePath)
	require.NoError(t, err)

	// Check if permissions are exactly 0600
	assert.Equal(t, fileutil.FileMode(0600), info.Mode().Perm())

	data, err := fileutil.ReadFile(storePath)
	require.NoError(t, err)

	// Check if schema_version exists
	assert.Contains(t, string(data), `"schema_version": "2.0.0"`)
}
// tdd refresh
