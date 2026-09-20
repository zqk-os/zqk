package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemValidateScenarioCommandBuilder creates a new system_validate_scenario command
func NewSystemValidateScenarioCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate-scenario")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
