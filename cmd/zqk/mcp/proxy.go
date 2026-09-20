package mcp

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/spf13/cobra"
)

func NewProxyCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcProxyCommandBuilder()
	cmd.RunE = runProxy
	return cmd
}

func runProxy(cmd *cobra.Command, _ []string) error {
	var flags clipkg.FlagBag
	addr := resolveTCPFlag(flags.String(cmd, "tcp"))
	if err := flags.Err(); err != nil {
		return err
	}

	logger := logging.GetLoggerFromProfile(cli.GetContext(cmd).Profile)
	logging.Fluent(logger).Info("MCP proxy started").String("addr", addr).Log()

	proxy := mcp.NewProxyDaemon(addr, logger)

	parent := cmd.Context()
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return proxy.Start(ctx)
}
