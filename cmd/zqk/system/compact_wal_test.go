package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewCompactWALCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := NewCompactWALCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "compact-wal", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	forceFlag := cmd.Flags().Lookup("force")
	require.NotNil(t, forceFlag, "expected --force flag on compact-wal command")
	assert.Equal(t, "false", forceFlag.DefValue)
}

func TestCompactWAL_Integration(t *testing.T) {
	projectRoot := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), projectRoot)
	if _, err := setupSystemTestEnvironmentRoot(t, projectRoot); err != nil {
		t.Fatalf("setupSystemTestEnvironmentRoot: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(projectRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, projectRoot, fileStorage)

	// Write an object to trigger WAL entry
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		"id":    "REQ-COMPACT-WAL-001",
		"kind":  "requirement",
		"title": "Compact WAL Test Object",
	}
	err = fileStorage.Create(context.Background(), secCtx, obj)
	require.NoError(t, err)

	// Verify CompactWAL executes without error
	err = storage.CompactWAL(projectRoot)
	assert.NoError(t, err)
}
