package app

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewCompletionCmd returns the root-level completion command for generating shell completion scripts.
// Supports bash, zsh, and fish. Output is written to stdout for sourcing (e.g. zqk completion bash > /path/to/completions).
func NewCompletionCmd(root *cobra.Command) *cobra.Command {
	cmd := NewCompletionBuilder(paths.CLICommandName, root).
		WithShells(DefaultShells()).
		Build()
	cli.RequireSession(cmd, false) // completion does not need session or storage
	return cmd
}
