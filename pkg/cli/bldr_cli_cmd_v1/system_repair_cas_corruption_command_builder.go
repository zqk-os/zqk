package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRepairCasCorruptionCommandBuilder creates a new system_repair_cas_corruption command
func NewSystemRepairCasCorruptionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for repair-cas-corruption")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
