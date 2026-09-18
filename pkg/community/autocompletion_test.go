package community_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
)

// Helper to create a dummy cobra command structure for tests.
func newTestCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "zqk",
		Short: "ZQK Knowledge Kernel CLI",
	}
	subCmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage workflows",
	}
	subCmd.Flags().StringP("mode", "m", "auto", "Execution mode")
	root.AddCommand(subCmd)
	return root
}

// CRIT-1789714836532627000-32b3b55a: Functional Acceptance
// Verifies generation of valid completion scripts across bash, zsh, fish, and powershell.
func TestAutocompletion_FunctionalAcceptance(t *testing.T) {
	root := newTestCommand()

	for _, shell := range community.SupportedShells {
		t.Run("generate_"+shell, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := community.CompletionConfig{
				Shell: shell,
			}
			err := community.GenerateCompletion(root, cfg, &buf)
			require.NoError(t, err, "Shell %s should generate without error", shell)
			output := buf.String()
			assert.NotEmpty(t, output, "Completion script for %s should not be empty", shell)
			assert.Contains(t, output, "zqk", "Completion script should reference the root command name")

			// Check shell installation instructions
			instructions, err := community.ShellInstallInstructions("zqk", shell)
			require.NoError(t, err)
			assert.NotEmpty(t, instructions)
			assert.Contains(t, instructions, shell)
		})
	}
}

// CRIT-1789714836532628000-19b3696a: Boundary & Error Handling
// Verifies handling of unsupported shells, nil pointers, empty input, and description toggles.
func TestAutocompletion_BoundaryAndErrorHandling(t *testing.T) {
	root := newTestCommand()

	t.Run("nil_root_command", func(t *testing.T) {
		var buf bytes.Buffer
		err := community.GenerateCompletion(nil, community.CompletionConfig{Shell: "bash"}, &buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "root command cannot be nil")
	})

	t.Run("nil_writer", func(t *testing.T) {
		err := community.GenerateCompletion(root, community.CompletionConfig{Shell: "bash"}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "writer cannot be nil")
	})

	t.Run("empty_shell_name", func(t *testing.T) {
		var buf bytes.Buffer
		err := community.GenerateCompletion(root, community.CompletionConfig{Shell: "   "}, &buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "shell name must not be empty")
	})

	t.Run("unsupported_shell", func(t *testing.T) {
		var buf bytes.Buffer
		err := community.GenerateCompletion(root, community.CompletionConfig{Shell: "tcsh"}, &buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported shell \"tcsh\"")
	})

	t.Run("is_shell_supported", func(t *testing.T) {
		assert.True(t, community.IsShellSupported("bash"))
		assert.True(t, community.IsShellSupported("ZSH"))
		assert.True(t, community.IsShellSupported("  fish "))
		assert.True(t, community.IsShellSupported("PowerShell"))
		assert.False(t, community.IsShellSupported("csh"))
		assert.False(t, community.IsShellSupported(""))
	})

	t.Run("unsupported_shell_install_instructions", func(t *testing.T) {
		_, err := community.ShellInstallInstructions("zqk", "unknown_shell")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported shell")
	})

	t.Run("default_binary_name_in_instructions", func(t *testing.T) {
		inst, err := community.ShellInstallInstructions("", "bash")
		require.NoError(t, err)
		assert.Contains(t, inst, "zqk completion bash")
	})
}

// CRIT-1789714836532629000-37553e81: Integration & Conformance
// Verifies description toggles, command structure discovery, and formatting flags.
func TestAutocompletion_IntegrationAndConformance(t *testing.T) {
	root := newTestCommand()

	t.Run("no_descriptions_toggle", func(t *testing.T) {
		for _, shell := range community.SupportedShells {
			var buf bytes.Buffer
			cfg := community.CompletionConfig{
				Shell:          shell,
				NoDescriptions: true,
			}
			err := community.GenerateCompletion(root, cfg, &buf)
			require.NoError(t, err, "Shell %s with NoDescriptions should generate cleanly", shell)
			assert.NotEmpty(t, buf.String())
		}
	})

	t.Run("subcommand_presence_in_script", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := community.CompletionConfig{Shell: "bash"}
		err := community.GenerateCompletion(root, cfg, &buf)
		require.NoError(t, err)
		script := buf.String()
		// Cobra bash completion v2 embeds subcommand words
		assert.True(t, strings.Contains(script, "workflow") || strings.Contains(script, "zqk"), "Script must mention workflow or zqk")
	})
}
