package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemRepairCasCorruptionCommandBuilder creates a new system_repair_cas_corruption command
func NewSystemRepairCasCorruptionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for repair-cas-corruption")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
