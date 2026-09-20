package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewQuickCommandBuilder creates a new quick command
func NewQuickCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for quick")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
