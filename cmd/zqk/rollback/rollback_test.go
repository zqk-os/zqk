package rollback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRollbackCmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewRollbackCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "rollback", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["list"], "rollback must have list subcommand")
	assert.True(t, subNames["apply"], "rollback must have apply subcommand")
	assert.True(t, subNames["reconstruct"], "rollback must have reconstruct subcommand")
	assert.True(t, subNames["retain"], "rollback must have retain subcommand")
}
