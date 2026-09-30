package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthchkBulkDisableCommandBuilder creates a new healthchk_bulk_disable command
func NewHealthchkBulkDisableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("disable")
	builder.WithShort("Bulk disable health check probes matching criteria")
	help := clipkg.DynamicHelpBuilder("Bulk disable health check probes matching criteria")
	help.WithDescriptionLines(
		"Disable multiple system health check probes in bulk based on filter criteria,",
		"subsystem tags, or specified probe identifiers.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
