package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewTraySignCommandBuilder creates a new tray_sign command
func NewTraySignCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for sign")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
