package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectDemoteCommandBuilder creates a new object_demote command
func NewObjectDemoteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("demote")
	builder.WithShort("Demote one or more objects to a lower lifecycle status")
	help := clipkg.DynamicHelpBuilder("Demote one or more objects to a lower lifecycle status")
	help.WithDescriptionLines("IDs may be positional args, comma-separated (ID1,ID2), and/or --ids.")
	help.AddExample("Demote many (comma-separated)", "%s object demote ATK-001,ATK-002")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	cmd.Use = "demote <id>[,id...] [id...]"
	cmd.Flags().String("ids", "", "Comma-separated list of object IDs (same as positional multi-ID / comma lists)")
	cmd.Args = cobra.MinimumNArgs(0)
	return cmd
}
