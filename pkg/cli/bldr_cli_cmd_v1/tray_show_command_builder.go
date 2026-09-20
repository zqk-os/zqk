package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewTrayShowCommandBuilder creates a new tray_show command
func NewTrayShowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for show")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
