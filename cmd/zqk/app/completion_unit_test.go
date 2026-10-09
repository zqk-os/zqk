package app

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionBuilder_UnitDirect(t *testing.T) {
	shells := DefaultShells()
	require.NotEmpty(t, shells)

	argsBuilder := NewCompletionArgsBuilder(shells)
	validArgs := argsBuilder.ValidArgs()
	assert.Contains(t, validArgs, "bash")
	assert.Contains(t, validArgs, "zsh")
	assert.Contains(t, validArgs, "fish")

	shellMap := argsBuilder.ShellByName()
	assert.NotNil(t, shellMap["bash"])
	assert.NotNil(t, shellMap["zsh"])
	assert.NotNil(t, shellMap["fish"])

	rootCmd := &cobra.Command{Use: "zqk"}
	b := &CompletionBuilder{
		CommandName: "zqk",
		Root:        rootCmd,
		Shells:      shells,
	}
	cmd := b.Build()
	require.NotNil(t, cmd)
	assert.Equal(t, "completion [bash|zsh|fish]", cmd.Use)
	rootCmd.AddCommand(cmd)

	// Test each shell's GenCompletion function directly
	for _, s := range shells {
		var buf bytes.Buffer
		err := s.GenCompletion(rootCmd, &buf)
		assert.NoError(t, err)
		assert.NotEmpty(t, buf.String())
	}

	// Test run of completion command
	var outBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetArgs([]string{"completion", "bash"})
	err := rootCmd.Execute()
	assert.NoError(t, err)

	outBuf.Reset()
	rootCmd.SetArgs([]string{"completion", "zsh"})
	err = rootCmd.Execute()
	assert.NoError(t, err)

	outBuf.Reset()
	rootCmd.SetArgs([]string{"completion", "fish"})
	err = rootCmd.Execute()
	assert.NoError(t, err)

	// Test invalid shell arg validation
	assert.Error(t, cmd.ValidateArgs([]string{"unsupported_shell"}))
}
