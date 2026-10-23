package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAuditEventBufferRegistry_GetGlobalBufferRegistry(t *testing.T) {
	reg1 := GetGlobalBufferRegistry()
	require.NotNil(t, reg1)

	reg2 := GetGlobalBufferRegistry()
	require.Same(t, reg1, reg2)
}

func TestAuditEventBufferRegistry_GetOrCreate_ReusesExistingBuffer(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.EnsureDir(tmpDir))

	reg := GetGlobalBufferRegistry()
	secCtx := pkgctx.NewSystemSecurityContext()

	buf1 := reg.GetOrCreate(tmpDir, secCtx)
	require.NotNil(t, buf1)
	t.Cleanup(func() {
		reg.Unregister(tmpDir)
	})

	buf2 := reg.GetOrCreate(tmpDir, secCtx)
	require.Same(t, buf1, buf2)
	require.Equal(t, tmpDir, buf1.GetProjectRoot())
}

func TestAuditEventBufferRegistry_Unregister_FlushesAndRemoves(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.EnsureDir(tmpDir))

	reg := GetGlobalBufferRegistry()
	secCtx := pkgctx.NewSystemSecurityContext()

	buf1 := reg.GetOrCreate(tmpDir, secCtx)
	require.NotNil(t, buf1)

	reg.Unregister(tmpDir)

	// After unregister, GetOrCreate should create a distinct new buffer instance
	buf2 := reg.GetOrCreate(tmpDir, secCtx)
	require.NotNil(t, buf2)
	require.NotSame(t, buf1, buf2)
	t.Cleanup(func() {
		reg.Unregister(tmpDir)
	})
}

func TestAuditEventBufferRegistry_MultipleProjectRoots(t *testing.T) {
	tmpRoot := t.TempDir()
	dirA := filepath.Join(tmpRoot, "projA")
	dirB := filepath.Join(tmpRoot, "projB")
	require.NoError(t, fileutil.EnsureDir(dirA))
	require.NoError(t, fileutil.EnsureDir(dirB))

	reg := GetGlobalBufferRegistry()
	secCtx := pkgctx.NewSystemSecurityContext()

	bufA := reg.GetOrCreate(dirA, secCtx)
	bufB := reg.GetOrCreate(dirB, secCtx)
	require.NotNil(t, bufA)
	require.NotNil(t, bufB)
	require.NotSame(t, bufA, bufB)

	require.Equal(t, dirA, bufA.GetProjectRoot())
	require.Equal(t, dirB, bufB.GetProjectRoot())

	reg.Unregister(dirA)
	reg.Unregister(dirB)
}
