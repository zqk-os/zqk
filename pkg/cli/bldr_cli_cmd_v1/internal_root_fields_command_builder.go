package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalRootFieldsCommandBuilder creates a new internal_root_fields command
func NewInternalRootFieldsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("fields")
	builder.WithShort("List internal object kinds or explore fields for a kind")
	help := clipkg.DynamicHelpBuilder("List internal object kinds or explore fields for a kind")
	help.WithDescriptionLines("Root-level internal fields command. Use --list-kinds to enumerate kinds, or use")
	help.WithDescriptionLines("internal <kind> fields for a specific kind.")
	help.AddExample("List all kinds", "%s internal fields --list-kinds")
	help.AddExample("List kinds as JSON", "%s internal fields --list-kinds --format json")
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
