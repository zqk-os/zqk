package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemViewCommandBuilder creates a new system_view command
func NewSystemViewCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for view")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
