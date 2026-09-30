package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMeshLeaseCommandBuilder creates a new mesh_lease command
func NewMeshLeaseCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("lease")
	builder.WithShort("Acquire or release execution leases on shared swarm workstreams")
	help := clipkg.DynamicHelpBuilder("Acquire or release execution leases on shared swarm workstreams")
	help.WithDescriptionLines("Coordinates mutual exclusion and resource reservation for distributed tasks across concurrent agent seats.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
