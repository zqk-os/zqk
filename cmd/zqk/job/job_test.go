package job

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJobCmd_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewJobCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "job", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["list"], "job must have list subcommand")
	assert.True(t, subNames["trigger"], "job must have trigger subcommand")
	assert.True(t, subNames["history"], "job must have history subcommand")
	assert.True(t, subNames["config"], "job must have config subcommand")
}
