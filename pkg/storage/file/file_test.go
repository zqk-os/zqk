package file_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/storage/file"
)

func TestFileStoreReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := file.NewStore(tmpDir)
	require.NoError(t, err)

	err = store.Write("testkind/obj1.yaml", []byte("id: obj1\nkind: testkind\n"))
	require.NoError(t, err)

	data, err := store.Read("testkind/obj1.yaml")
	require.NoError(t, err)
	require.Equal(t, "id: obj1\nkind: testkind\n", string(data))
}
