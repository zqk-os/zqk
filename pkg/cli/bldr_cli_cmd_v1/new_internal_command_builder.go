package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewNewInternalCommandBuilder creates a new new_internal command
func NewNewInternalCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("internal")
	builder.WithShort("Write a draft YAML template for an internal object kind")
	help := clipkg.DynamicHelpBuilder("Write a draft YAML template for an internal object kind")
	help.WithDescriptionLines("Same YAML generation as `new object`; apply with internal create when ready.")
	help.WithDescriptionLines("Next: zqk internal create <kind> (after a default-path draft, --file is optional).")
	help.AddExample("Draft template for object_spec", "%s new internal object_spec")
	help.AddExample("Stdout", "%s new internal object_spec -o -")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddStringFlag("output", "o", "", "Output file path, or '-' for stdout (default: .zqk/drafts/<kind>-<timestamp>.yaml)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
