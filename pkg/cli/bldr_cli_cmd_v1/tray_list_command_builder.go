package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewTrayListCommandBuilder creates a new tray_list command
func NewTrayListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
