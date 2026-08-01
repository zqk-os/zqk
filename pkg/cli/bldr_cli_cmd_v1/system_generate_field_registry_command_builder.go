package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateFieldRegistryCommandBuilder creates a new system_generate_field_registry command
func NewSystemGenerateFieldRegistryCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-field-registry")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
