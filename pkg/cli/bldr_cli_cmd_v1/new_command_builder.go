package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewNewCommandBuilder creates a new new command
func NewNewCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("new")
	builder.WithShort("Write draft YAML templates for object create and scenario bundles")
	help := clipkg.DynamicHelpBuilder("Write draft YAML templates for object create and scenario bundles")
	help.WithDescriptionLines("Materialize editable drafts derived from object specs (cliexamples) or a minimal scenario bundle.")
	help.WithDescriptionLines("For a new kind definition file (object_specs YAML), use: zqk new object-spec <ontology> [--extends base_object]")
	help.WithDescriptionLines("Edit instance drafts, then persist with:")
	help.WithDescriptionLines("  zqk object create <kind>   (after a default-path draft, --file is optional)")
	help.WithDescriptionLines("  zqk internal create <kind>   (privileged kinds; same last-draft pointer)")
	help.WithDescriptionLines("  zqk-scenario bundle apply -f <draft> -R .")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Default output path when --output is omitted: .zqk/drafts/<name>-<timestamp>.yaml")
	help.WithDescriptionLines("Use --output - for stdout.")
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
