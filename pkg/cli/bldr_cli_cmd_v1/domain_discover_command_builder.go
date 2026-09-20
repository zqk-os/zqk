package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewDomainDiscoverCommandBuilder creates a new domain_discover command
func NewDomainDiscoverCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("discover")
	builder.WithShort("Discover registered domain ontologies")
	help := clipkg.DynamicHelpBuilder("Discover registered domain ontologies")
	help.WithDescriptionLines("List domain_registry objects (registered domain ontologies) from storage.")
	help.WithDescriptionLines("Use to see which domains are registered and available for domain objects.")
	help.AddExample("List registered domains", "%s domain discover")
	help.AddExample("Discover with scan", "%s domain discover --scan")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("scan", "", false, "Scan project for domain_registry objects (same as default list)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
