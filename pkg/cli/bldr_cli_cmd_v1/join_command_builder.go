package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewJoinCommandBuilder creates a new join command
func NewJoinCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("join")
	builder.WithShort("Join a node or agent into an existing federation or mesh network")
	help := clipkg.DynamicHelpBuilder("Join a node or agent into an existing federation or mesh network")
	help.WithDescriptionLines(
		"Connect and register the local node or agent workspace into an existing",
		"federated swarm or distributed mesh network.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
