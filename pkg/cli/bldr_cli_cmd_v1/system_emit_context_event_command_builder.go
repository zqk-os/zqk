package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemEmitContextEventCommandBuilder creates a new system_emit_context_event command
func NewSystemEmitContextEventCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for emit-context-event")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
