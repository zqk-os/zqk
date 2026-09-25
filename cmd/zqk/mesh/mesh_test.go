package mesh

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMesh(t *testing.T) {
	cmd := NewMeshCmd()
	require.NotNil(t, cmd, "NewMeshCmd must return non-nil command")

	assert.Equal(t, "mesh", cmd.Use)
	assert.NotEmpty(t, cmd.Short, "mesh command must provide a short description")

	// Verify required subcommands are mounted
	subcommands := cmd.Commands()
	subcmdNames := make(map[string]bool)
	for _, sc := range subcommands {
		subcmdNames[sc.Name()] = true
	}

	expectedSubcmds := []string{"market", "advertise", "lease"}
	for _, expected := range expectedSubcmds {
		assert.True(t, subcmdNames[expected], "mesh command must register subcommand %q", expected)
	}

	// Verify execution with help flag outputs usage
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "mesh")
}
