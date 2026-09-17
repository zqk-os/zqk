package storage_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestGraphFacade_NewGraphLock(t *testing.T) {
	mockProv := provider.NewMockGraphProvider()
	pool, err := mockProv.CreatePool(t.Context(), provider.ConnectionConfig{MaxConns: 2})
	require.NoError(t, err)

	lock := storage.NewGraphLock(pool, "res1", "owner1")
	require.NotNil(t, lock)
}
