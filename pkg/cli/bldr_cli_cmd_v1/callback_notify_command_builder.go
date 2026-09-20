package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewCallbackNotifyCommandBuilder creates a new callback_notify command
func NewCallbackNotifyCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for notify")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
