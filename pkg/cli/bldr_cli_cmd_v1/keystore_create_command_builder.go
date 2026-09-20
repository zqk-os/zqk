package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewKeystoreCreateCommandBuilder creates a new keystore_create command
func NewKeystoreCreateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for create")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
