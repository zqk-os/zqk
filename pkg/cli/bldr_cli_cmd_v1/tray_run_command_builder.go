package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewTrayRunCommandBuilder creates a new tray_run command
func NewTrayRunCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for run")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
