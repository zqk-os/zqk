package observer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewObserverCmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewObserverCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "observer", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["extract"], "observer must have extract subcommand")
	assert.True(t, subNames["populate"], "observer must have populate subcommand")
}
