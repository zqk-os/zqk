package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemUpdateMutationMetricsCommandBuilder creates a new system_update_mutation_metrics command
func NewSystemUpdateMutationMetricsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for update-mutation-metrics")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
