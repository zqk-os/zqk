package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectDemoteCommandBuilder creates a new object_demote command
func NewObjectDemoteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("demote")
	builder.WithShort("Demote one or more objects to a lower lifecycle status")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
