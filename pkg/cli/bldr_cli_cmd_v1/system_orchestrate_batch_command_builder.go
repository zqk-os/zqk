package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemOrchestrateBatchCommandBuilder creates a new system_orchestrate_batch command
func NewSystemOrchestrateBatchCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generates the ontological cascade (Workstream, Requirement, Criteria, TestCase) for a batch of items")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
