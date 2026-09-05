package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewQuickBundleCommandBuilder creates a new quick_bundle command
func NewQuickBundleCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("bundle")
	builder.WithShort("Create a backlog item, criteria, and agent tasks from a bundle file")
	help := clipkg.DynamicHelpBuilder("Create a backlog item, criteria, and agent tasks from a bundle file")
	help.WithDescriptionLines("Accepts a JSON/YAML file defining a backlog_item, criteria, and agent_tasks.")
	help.WithDescriptionLines("Auto-links them via backlog_item_refs and related_object_refs.")
	help.AddExample("Create from YAML", "%s quick bundle --file bundle.yaml")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("file", "b", "", "Path to bundle YAML or JSON file")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
