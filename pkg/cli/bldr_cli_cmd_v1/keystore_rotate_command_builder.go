package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewKeystoreRotateCommandBuilder creates a new keystore_rotate command
func NewKeystoreRotateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for rotate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
