package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalKindFieldsCommandBuilder creates a new internal_kind_fields command
func NewInternalKindFieldsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("fields")
	builder.WithShort("List available fields for an internal object kind")
	help := clipkg.DynamicHelpBuilder("List available fields for an internal object kind")
	help.WithDescriptionLines("List available fields for a specific internal object kind.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Usage: internal <kind> fields [flags]")
	help.AddExample("List fields for a specific kind", "%s internal object_spec fields")
	help.AddExample("Root: list kinds (use internal fields)", "%s internal fields --list-kinds")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
