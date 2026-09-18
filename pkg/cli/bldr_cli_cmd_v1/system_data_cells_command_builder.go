package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemDataCellsCommandBuilder creates a new system_data_cells command
func NewSystemDataCellsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for data-cells")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
