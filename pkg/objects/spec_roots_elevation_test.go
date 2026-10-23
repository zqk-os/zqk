package objects

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAddSpecRoot_DuplicatesAndEmpty(t *testing.T) {
	initialCount := len(ExtraSpecRoots())
	AddSpecRoot("")
	assert.Len(t, ExtraSpecRoots(), initialCount)

	tempDir := t.TempDir()
	AddSpecRoot(tempDir)
	defer RemoveSpecRoot(tempDir)

	assert.Len(t, ExtraSpecRoots(), initialCount+1)
	// Adding again must be idempotent
	AddSpecRoot(tempDir)
	assert.Len(t, ExtraSpecRoots(), initialCount+1)
}

func TestAddLifecycleRoot_DuplicatesAndEmpty(t *testing.T) {
	initialCount := len(ExtraLifecycleRoots())
	AddLifecycleRoot("")
	assert.Len(t, ExtraLifecycleRoots(), initialCount)

	tempDir := t.TempDir()
	AddLifecycleRoot(tempDir)
	defer RemoveLifecycleRoot(tempDir)

	assert.Len(t, ExtraLifecycleRoots(), initialCount+1)
	AddLifecycleRoot(tempDir)
	assert.Len(t, ExtraLifecycleRoots(), initialCount+1)
}

func TestSpecIndexKeyAndStamp(t *testing.T) {
	tempDir := t.TempDir()
	key1 := specIndexKey(tempDir, nil)
	assert.Equal(t, tempDir, key1)

	key2 := specIndexKey(tempDir, []string{"/extra/specs"})
	assert.Contains(t, key2, tempDir)
	assert.Contains(t, key2, "/extra/specs")

	stamp := specIndexStamp(tempDir, []string{tempDir})
	assert.GreaterOrEqual(t, uint64(stamp), uint64(0))
}

func TestMergeSpecIndexes_Precedence(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, "kernel")
	extraDir := filepath.Join(tempDir, "pack")
	require.NoError(t, fileutil.EnsureDir(specsDir))
	require.NoError(t, fileutil.EnsureDir(extraDir))

	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(specsDir, "bli.yaml"), []byte("kind: bli\n")))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(extraDir, "bli.yaml"), []byte("kind: bli_pack\n")))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(extraDir, "pack_only.yaml"), []byte("kind: pack_only\n")))

	merged := mergeSpecIndexes(specsDir, []string{extraDir})
	assert.Equal(t, filepath.Join(specsDir, "bli.yaml"), merged["bli.yaml"])
	assert.Equal(t, filepath.Join(extraDir, "pack_only.yaml"), merged["pack_only.yaml"])
}

func TestFindRegisteredSpecFile_Errors(t *testing.T) {
	tempDir := t.TempDir()
	_, err := FindRegisteredSpecFile(tempDir, "")
	assert.Error(t, err)

	_, err = FindRegisteredSpecFile(tempDir, "non_existent_kind_xyz_99")
	assert.Error(t, err)
}

func TestPathHelpers(t *testing.T) {
	t.Parallel()
	assert.True(t, isPathUnderDir("/a/b", "/a/b/c/d.yaml"))
	assert.False(t, isPathUnderDir("/a/b", "/a/x/d.yaml"))
	assert.True(t, isPathUnderDir("/a/b", "/a/b"))

	abs, ok := cleanAbsPath(".")
	assert.True(t, ok)
	assert.NotEmpty(t, abs)

	assert.False(t, pathInExtraSpecRoot("/non/existent/path/spec.yaml"))
}
