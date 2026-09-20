package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewQuickCommandBuilder creates a new quick command
func NewQuickCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for quick")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
