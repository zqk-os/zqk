package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewVdsCommandBuilder creates a new vds command
func NewVdsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("vds")
	builder.WithShort("Verifiable Decomposition Spine — gates agents must respect")
	help := clipkg.DynamicHelpBuilder("Verifiable Decomposition Spine — gates agents must respect")
	help.WithDescriptionLines("Project-agnostic culture tool for POL-WORKFLOW-VDS: checklist stages, scaffold chunks,")
	help.WithDescriptionLines("and evaluate dsl_checks with a fluent pass/fail report.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Subcommands: checklist, evaluate, project, init.")
	help.WithDescriptionLines("project uses customization vendor_providers (--provider / --all / --list); no vendor-named subcommands.")
	help.AddExample("Culture + stages", "%s workflow vds checklist --format json")
	help.AddExample("Evaluate chunks", "%s workflow vds evaluate --format json")
	help.AddExample("Agent markdown brief", "%s workflow vds evaluate --format agent-prompt")
	help.AddExample("Project configured provider", "%s workflow vds project --provider ide --write")
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
