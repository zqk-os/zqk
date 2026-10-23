package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAuditEventBuffer_InitializationAccessors(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.EnsureDir(tmpDir))

	secCtx := pkgctx.NewSystemSecurityContext()
	rules := DefaultAggregationRules()
	buf := NewAuditEventBuffer(tmpDir, secCtx, rules)
	require.NotNil(t, buf)
	t.Cleanup(func() {
		require.NoError(t, buf.Shutdown())
	})

	require.Equal(t, tmpDir, buf.GetProjectRoot())
	newRoot := filepath.Join(tmpDir, "new-root")
	buf.SetProjectRoot(newRoot)
	require.Equal(t, newRoot, buf.GetProjectRoot())

	buf.SetSecurityContext(secCtx)
	require.True(t, buf.IsEnabled())
	buf.SetEnabled(false)
	require.False(t, buf.IsEnabled())
	buf.SetEnabled(true)
	require.True(t, buf.IsEnabled())

	// Setters under non-contended state
	buf.SetFileStorage(nil)
	buf.SetTSDBProvider(nil)

	ch := make(chan FlushProgress, 5)
	buf.SetFlushProgressChannel(ch)

	buf.SetFlushErrorCallback(func(key string, err error) {})
}

func TestAuditEventBuffer_GetContextLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.EnsureDir(tmpDir))

	buf := NewAuditEventBuffer(tmpDir, pkgctx.NewSystemSecurityContext(), DefaultAggregationRules())
	require.NotNil(t, buf)
	t.Cleanup(func() {
		require.NoError(t, buf.Shutdown())
	})

	ctx, cancel := buf.getContext()
	require.NotNil(t, ctx)
	require.NotNil(t, cancel)
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.True(t, time.Until(deadline) > 0)
}

func TestAuditEventBuffer_FlushAndTearDownGlobalHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.EnsureDir(tmpDir))
	secCtx := pkgctx.NewSystemSecurityContext()

	require.NoError(t, InitializeGlobalBufferWithConfig(tmpDir, secCtx))
	globalBuf := GetGlobalAuditEventBuffer()
	require.NotNil(t, globalBuf)

	// Flush for matching root
	FlushGlobalAuditBufferForProjectRoot(tmpDir)

	// Flush for non-matching root (no-op)
	FlushGlobalAuditBufferForProjectRoot("/nonexistent/path")

	// TearDown for matching root
	resetDir := filepath.Join(t.TempDir(), "reset-root")
	require.NoError(t, TearDownGlobalAuditBufferForTestProjectRoot(tmpDir, resetDir, secCtx))

	// TearDown for non-matching root (no-op)
	require.NoError(t, TearDownGlobalAuditBufferForTestProjectRoot("/nonexistent/path", resetDir, secCtx))

	// Shutdown all tracked buffers
	ShutdownAllAuditBuffersForTesting()
}
