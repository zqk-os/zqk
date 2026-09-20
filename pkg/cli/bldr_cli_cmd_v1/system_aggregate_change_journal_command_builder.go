package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAggregateChangeJournalCommandBuilder creates a new system_aggregate_change_journal command
func NewSystemAggregateChangeJournalCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for aggregate-change-journal")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
