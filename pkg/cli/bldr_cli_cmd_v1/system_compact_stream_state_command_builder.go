package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCompactStreamStateCommandBuilder creates a new system_compact_stream_state command
func NewSystemCompactStreamStateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for compact-stream-state")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
