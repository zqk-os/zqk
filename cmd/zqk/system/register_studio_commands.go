package system

import "github.com/spf13/cobra"

// registerStudioCommands registers studio-only system subcommands.
// In the open-core community candidate, this file is replaced with a no-op stub.
func registerStudioCommands(systemCmd *cobra.Command) {
	systemCmd.AddCommand(NewEvolveCmd())
	systemCmd.AddCommand(NewMaterializeAgentChatChannelCmd())
	systemCmd.AddCommand(NewAmbientDaemonCmd())
}
