package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewCiDemoteCommandBuilder creates a new ci_demote command
func NewCiDemoteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("demote")
	builder.WithShort("Tear down the Local CI workdir and reset state")
	help := clipkg.DynamicHelpBuilder("Tear down the Local CI workdir and reset state")
	help.WithDescriptionLines("Removes the active Local CI worktree and associated state files.")
	help.AddExample("Tear down local CI", "%s ci demote")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
