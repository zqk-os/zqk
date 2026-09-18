package core_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/storage/core"
	"github.com/stretchr/testify/require"
)

func TestCoreMemoryEngine(t *testing.T) {
	eng := core.NewMemoryEngine("policy")
	require.Equal(t, "policy", eng.Kind())
	require.NoError(t, eng.Close())
}
