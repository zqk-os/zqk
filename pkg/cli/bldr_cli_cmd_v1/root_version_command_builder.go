package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewRootVersionCommandBuilder creates a new root_version command
func NewRootVersionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("version")
	builder.WithShort("Show ZQK version information")
	help := clipkg.DynamicHelpBuilder("Show ZQK version information")
	help.WithDescriptionLines("Display version information including executable name, build datetime, and git commit.")
	help.WithDescriptionLines("Same behavior as `utility version`; provided at the top level for discoverability and scripting.")
	help.AddExample("Show version (top-level)", "%s version")
	help.AddExample("Same output via utility group", "%s utility version")
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
