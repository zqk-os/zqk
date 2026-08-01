package mcp

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

var daemonTcpAddr string

func NewDaemonCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMcpDaemonCommandBuilder(), &cobra.Command{
		Use:   "daemon",
		Short: "Start the MCP background daemon",
	})
	cmd.RunE = runDaemon
	cli.RequireSession(cmd, false)
	return cmd
}

func runDaemon(cmd *cobra.Command, args []string) error {
	daemonTcpAddr, _ := cmd.Flags().GetString("tcp")
	if port, err := cmd.Flags().GetInt("port"); err == nil && port > 0 {
		daemonTcpAddr = fmt.Sprintf("127.0.0.1:%d", port)
	}
	if daemonTcpAddr == "" {
		daemonTcpAddr = "127.0.0.1:8443"
	}

	// Reuse serve path with TCP enabled so proxy↔daemon share one MCP server implementation.
	tcpAddr = daemonTcpAddr

	logger := logging.GetLoggerFromProfile(cli.GetContext(cmd).Profile)
	logging.Fluent(logger).Info("MCP daemon starting on TCP").String("addr", daemonTcpAddr).Log()

	parent := cmd.Context()
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd.SetContext(ctx)

	return runServe(cmd, args)
}
