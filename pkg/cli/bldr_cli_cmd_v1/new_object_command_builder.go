package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewNewObjectCommandBuilder creates a new new_object command
func NewNewObjectCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("object")
	builder.WithShort("Write a draft YAML template for an object kind")
	help := clipkg.DynamicHelpBuilder("Write a draft YAML template for an object kind")
	help.WithDescriptionLines("Generates YAML from the object spec (required fields and common optional fields).")
	help.WithDescriptionLines("Next: zqk object create <kind> (after a default-path draft, --file is optional).")
	help.AddExample("Draft template for a backlog item (writes to .zqk/drafts/ by default)", "%s new object backlog_item")
	help.AddExample("Write draft to stdout", "%s new object goal --output -")
	help.AddExample("Explicit output path", "%s new object requirement -o /tmp/req-draft.yaml")
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
