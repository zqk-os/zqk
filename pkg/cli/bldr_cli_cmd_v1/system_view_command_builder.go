package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemViewCommandBuilder creates a new system_view command
func NewSystemViewCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for view")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
