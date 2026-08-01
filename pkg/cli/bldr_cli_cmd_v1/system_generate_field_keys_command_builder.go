package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateFieldKeysCommandBuilder creates a new system_generate_field_keys command
func NewSystemGenerateFieldKeysCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-field-keys")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
