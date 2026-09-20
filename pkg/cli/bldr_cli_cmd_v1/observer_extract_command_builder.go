package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObserverExtractCommandBuilder creates a new observer_extract command
func NewObserverExtractCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for extract")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
