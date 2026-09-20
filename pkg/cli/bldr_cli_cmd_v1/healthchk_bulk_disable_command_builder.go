package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthchkBulkDisableCommandBuilder creates a new healthchk_bulk_disable command
func NewHealthchkBulkDisableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("disable")
	builder.WithShort("disable command")
	help := clipkg.DynamicHelpBuilder("disable command")
	help.WithDescriptionLines("disable command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
