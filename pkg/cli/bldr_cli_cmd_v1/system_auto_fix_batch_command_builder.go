package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAutoFixBatchCommandBuilder creates a new system_auto_fix_batch command
func NewSystemAutoFixBatchCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for auto-fix-batch")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
