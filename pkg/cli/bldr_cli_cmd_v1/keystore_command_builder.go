package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewKeystoreCommandBuilder creates a new keystore command
func NewKeystoreCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for keystore")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
