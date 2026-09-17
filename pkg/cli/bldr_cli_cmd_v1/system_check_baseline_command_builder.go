package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCheckBaselineCommandBuilder creates a new system_check_baseline command
func NewSystemCheckBaselineCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for check-baseline")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
