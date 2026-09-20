package system

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/relay"
	"github.com/spf13/cobra"
)

// NewRelayCmd creates the relay command that spins up the Sovereign Relay HTTP/WebSocket server.
func NewRelayCmd() *cobra.Command {
	var listenAddr string

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRelayCommandBuilder(), &cobra.Command{
		Use:   "relay",
		Short: "Start the Sovereign Relay NAT traversal server",
		Long: `Starts the ZQK Sovereign Relay server. 
This command is intended to be run on a public-facing cloud instance (e.g. behind Cloudflare).
It acts as a secure, end-to-end encrypted 'dumb pipe' router, mapping incoming webhook traffic
from external APIs to persistent WebSockets held by local ZQK developer daemons.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRelayServer(listenAddr)
		},
	})

	cmd.Flags().StringVar(&listenAddr, "listen", ":8443", "Address and port to listen on")

	return cmd
}

func runRelayServer(addr string) error {
	srv := relay.NewServer(addr)
	return srv.Start()
}
