package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestFileObjectStorage_NewHashRegistry_CancelledContextReturnsNonNil(t *testing.T) {
	t.Setenv(zqkenv.StreamStorageEnabled().Name(), "false")

	projectRoot := t.TempDir()
	fos, err := NewFileObjectStorageForTest(projectRoot)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fos.Shutdown(context.Background())
	})

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	kindDir := t.TempDir()
	reg := fos.newHashRegistry(cancelledCtx, "audit_event", kindDir)
	require.NotNil(t, reg, "newHashRegistry must not return nil when request context is cancelled")
	require.NoError(t, reg.Load(), "Load() on returned HashRegistry must succeed without panicking")
}
