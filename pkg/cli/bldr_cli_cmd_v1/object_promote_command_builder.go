package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectPromoteCommandBuilder creates a new object_promote command
func NewObjectPromoteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("promote")
	builder.WithShort("Promote one or more objects to the highest valid lifecycle status they satisfy")
	help := clipkg.DynamicHelpBuilder("Promote one or more objects to the highest valid lifecycle status they satisfy")
	help.WithDescriptionLines("Promote one or more objects to the highest valid lifecycle status they satisfy")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
