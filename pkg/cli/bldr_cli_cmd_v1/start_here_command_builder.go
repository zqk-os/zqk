package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewStartHereCommandBuilder creates a new start_here command
func NewStartHereCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("start-here")
	builder.WithShort("Start Here tutorial for new users and agents")
	help := clipkg.DynamicHelpBuilder("Start Here tutorial for new users and agents")
	help.WithDescriptionLines("Shows the Start Here tutorial steps for Community Edition / Sovereign OS onboarding.")
	help.AddExample("Run the Start Here tutorial", "%s system start-here")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
