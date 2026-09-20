package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemVetConfigCommandBuilder creates a new system_vet_config command
func NewSystemVetConfigCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for vet-config")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
