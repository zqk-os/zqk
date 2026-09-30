package grep

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGrepCmd(t *testing.T) {
	t.Parallel()

	cmd := NewGrepCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "grep [query] [path]", cmd.Use)
	assert.Contains(t, cmd.Aliases, "zgrep")
	assert.NotNil(t, cmd.Flags().Lookup("ignore-case"))
	assert.NotNil(t, cmd.Flags().Lookup("ast"))
	assert.NotNil(t, cmd.Flags().Lookup("max-tokens"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))
}
