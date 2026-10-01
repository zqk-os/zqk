package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUICmd(t *testing.T) {
	cmd := NewUICmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "ui", cmd.Name())
	assert.Contains(t, cmd.Aliases, "dashboard")
	assert.Contains(t, cmd.Aliases, "console")

	tabFlag := cmd.Flag("tab")
	require.NotNil(t, tabFlag)
	assert.Equal(t, "seismograph", tabFlag.DefValue)
}
