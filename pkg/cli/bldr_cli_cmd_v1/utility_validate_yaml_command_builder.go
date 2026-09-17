package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewUtilityValidateYamlCommandBuilder creates a new utility_validate_yaml command
func NewUtilityValidateYamlCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate-yaml")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
