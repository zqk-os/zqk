package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAlignCommandBuilder creates a new system_align command
func NewSystemAlignCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for align")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
