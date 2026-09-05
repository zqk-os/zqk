package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewKeystoreRotateCommandBuilder creates a new keystore_rotate command
func NewKeystoreRotateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for rotate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
