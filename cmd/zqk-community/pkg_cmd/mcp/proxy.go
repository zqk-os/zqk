package mcp

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/spf13/cobra"
)

var proxyTcpAddr string

func NewProxyCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMcpProxyCommandBuilder(), &cobra.Command{
		Use:   "proxy",
		Short: "Start the MCP proxy shim",
	})
	cmd.RunE = runProxy
	cmd.Flags().StringVar(&proxyTcpAddr, "tcp", "127.0.0.1:8443", "TCP address of the underlying MCP daemon")
	cli.RequireSession(cmd, false)
	return cmd
}

func runProxy(cmd *cobra.Command, args []string) error {
	logger := logging.GetLoggerFromProfile(cli.GetContext(cmd).Profile)
	logging.Fluent(logger).Info("MCP proxy started").Log()

	proxy := mcp.NewProxyDaemon(proxyTcpAddr, logger)

	parent := cmd.Context()
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return proxy.Start(ctx)
}
