package system

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/stretchr/testify/require"
)

func TestComputeTestCaseHash(t *testing.T) {
	tempRoot := t.TempDir()

	// Create dummy artifacts
	artifact1 := filepath.Join(tempRoot, "dummy1.txt")
	err := fileutil.WriteStandardFile(artifact1, []byte("hello world 1"))
	require.NoError(t, err)

	artifact2 := filepath.Join(tempRoot, "dummy2.txt")
	err = fileutil.WriteStandardFile(artifact2, []byte("hello world 2"))
	require.NoError(t, err)

	hash, err := computeTestCaseHash(tempRoot, []string{"dummy2.txt", "dummy1.txt"})
	require.NoError(t, err)

	// compute expected hash manually to verify identical logic
	hasher := sha256.New()

	h1 := sha256.New()
	h1.Write([]byte("hello world 1"))
	h1Str := hex.EncodeToString(h1.Sum(nil))

	h2 := sha256.New()
	h2.Write([]byte("hello world 2"))
	h2Str := hex.EncodeToString(h2.Sum(nil))

	// Should be sorted alphabetically by artifact path
	hasher.Write([]byte("dummy1.txt:" + h1Str + "\n"))
	hasher.Write([]byte("dummy2.txt:" + h2Str + "\n"))

	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	require.Equal(t, expectedHash, hash)
}
