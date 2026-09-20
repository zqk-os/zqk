package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectPromoteCommandBuilder creates a new object_promote command
func NewObjectPromoteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("promote")
	builder.WithShort("Promote one or more objects to the highest valid lifecycle status they satisfy")
	help := clipkg.DynamicHelpBuilder("Promote one or more objects to the highest valid lifecycle status they satisfy")
	help.WithDescriptionLines("Promote one or more objects one lifecycle hop when preconditions pass.")
	help.WithDescriptionLines("IDs may be positional args, comma-separated (ID1,ID2), and/or --ids.")
	help.AddExample("Promote one object", "%s object promote ATK-001")
	help.AddExample("Promote many (comma-separated)", "%s object promote ATK-001,ATK-002,ATK-003")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	cmd.Use = "promote <id>[,id...] [id...]"
	cmd.Flags().String("ids", "", "Comma-separated list of object IDs (same as positional multi-ID / comma lists)")
	cmd.Args = cobra.MinimumNArgs(0)
	return cmd
}
