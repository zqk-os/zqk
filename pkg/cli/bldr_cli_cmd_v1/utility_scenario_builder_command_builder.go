package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewUtilityScenarioBuilderCommandBuilder creates a new utility_scenario_builder command
func NewUtilityScenarioBuilderCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for scenario-builder")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
