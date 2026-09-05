package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObserverPopulateCommandBuilder creates a new observer_populate command
func NewObserverPopulateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for populate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
