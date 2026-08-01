package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewTrayCommandBuilder creates a new tray command
func NewTrayCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for tray")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
