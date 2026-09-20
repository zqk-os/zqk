package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCheckAsyncBaselineCommandBuilder creates a new system_check_async_baseline command
func NewSystemCheckAsyncBaselineCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for check-async-baseline")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
