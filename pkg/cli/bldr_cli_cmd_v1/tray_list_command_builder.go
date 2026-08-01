package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewTrayListCommandBuilder creates a new tray_list command
func NewTrayListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
