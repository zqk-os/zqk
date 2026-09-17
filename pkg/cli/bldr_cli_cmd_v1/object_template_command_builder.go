package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectTemplateCommandBuilder creates a new object_template command
func NewObjectTemplateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("template")
	builder.WithShort("Generate a blank template for creating an object")
	help := clipkg.DynamicHelpBuilder("Generate a blank template for creating an object")
	help.WithDescriptionLines("Generate a blank YAML template for creating an object of the specified kind.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The template includes all required fields with placeholder values and optional fields")
	help.WithDescriptionLines("with their default values (if any). This is useful for creating objects manually or")
	help.WithDescriptionLines("as a starting point for automation.")
	help.AddExample("Generate template for backlog item", "%s template backlog_item")
	help.AddExample("Generate template and save to file", "%s template backlog_item --output item.yaml")
	help.AddExample("Generate template without optional fields", "%s template backlog_item --include-optional=false")
	help.AddExample("Generate template without comments", "%s template backlog_item --include-comments=false")
	help.ExcludeFlag("format")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("include-optional", "", true, "Include optional fields in template (default: true)")
	builder.AddBoolFlag("include-comments", "", true, "Include field descriptions as comments (default: true)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
