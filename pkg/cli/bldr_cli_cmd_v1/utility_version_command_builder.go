package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewUtilityVersionCommandBuilder creates a new utility_version command
func NewUtilityVersionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("version")
	builder.WithShort("Show ZQK version information")
	help := clipkg.DynamicHelpBuilder("Show ZQK version information")
	help.WithDescriptionLines("Display version information including executable name, build datetime, and git commit.")
	help.AddExample("Show version (top-level alias)", "%s version")
	help.AddExample("Show version under utility", "%s utility version")
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
