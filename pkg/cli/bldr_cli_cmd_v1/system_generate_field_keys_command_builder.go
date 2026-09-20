package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateFieldKeysCommandBuilder creates a new system_generate_field_keys command
func NewSystemGenerateFieldKeysCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-field-keys")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
