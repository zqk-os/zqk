package semantic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSemanticCmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewSemanticCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "semantic", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["assess"], "semantic must have assess subcommand")
	assert.True(t, subNames["recommend"], "semantic must have recommend subcommand")
	assert.True(t, subNames["infer"], "semantic must have infer subcommand")
}
