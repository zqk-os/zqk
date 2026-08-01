package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGitCommandBuilder creates a new system_git command
func NewSystemGitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for git")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
