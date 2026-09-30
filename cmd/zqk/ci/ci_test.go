package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCICmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewCICmd()
	require.NotNil(t, cmd, "NewCICmd must return non-nil command")
	assert.Equal(t, "ci", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["checkout"], "ci command must have checkout subcommand")
	assert.True(t, subNames["run"], "ci command must have run subcommand")
	assert.True(t, subNames["status"], "ci command must have status subcommand")
	assert.True(t, subNames["demote"], "ci command must have demote subcommand")
}
