package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemVetConfigCommandBuilder creates a new system_vet_config command
func NewSystemVetConfigCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for vet-config")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
