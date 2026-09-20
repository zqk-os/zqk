package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObserverCommandBuilder creates a new observer command
func NewObserverCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for observer")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
