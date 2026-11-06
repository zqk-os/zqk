package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureTestIdentityCacheHandler_RestoresHandlerAfterNil(t *testing.T) {
	prev := GetCacheOperationHandler()
	t.Cleanup(func() {
		SetCacheOperationHandler(prev)
	})

	// First ensure sets default handler
	EnsureTestIdentityCacheHandler()
	require.NotNil(t, GetCacheOperationHandler())

	// Simulate a test clearing the handler on cleanup or during test
	SetCacheOperationHandler(nil)
	require.Nil(t, GetCacheOperationHandler())

	// EnsureTestIdentityCacheHandler must restore it whenever it is nil
	EnsureTestIdentityCacheHandler()
	require.NotNil(t, GetCacheOperationHandler())
}
