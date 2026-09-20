package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGitCommandBuilder creates a new system_git command
func NewSystemGitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("git")
	builder.WithShort("Generated spec for git")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
