package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStreamGcCommandBuilder creates a new system_stream_gc command
func NewSystemStreamGcCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for stream-gc")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
