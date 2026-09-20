package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObserverPopulateCommandBuilder creates a new observer_populate command
func NewObserverPopulateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for populate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
