package mcp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func NewDaemonCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcDaemonCommandBuilder()
	cmd.RunE = runDaemon
	return cmd
}

func runDaemon(cmd *cobra.Command, args []string) error {
	var flags clipkg.FlagBag
	addr := strings.TrimSpace(flags.String(cmd, "tcp"))
	port := flags.Int(cmd, "port")
	if err := flags.Err(); err != nil {
		return err
	}
	if port > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	} else {
		addr = resolveTCPFlag(addr)
	}

	// Reuse serve path with TCP enabled so proxy↔daemon share one MCP server implementation.
	tcpAddr = addr

	logger := logging.GetLoggerFromProfile(cli.GetContext(cmd).Profile)
	logging.Fluent(logger).Info("MCP daemon starting on TCP").Addr(addr).Log()

	// Do not derive from cmd.Context(): agent/IDE shells often cancel it when stdin
	// closes or the parent request ends, which exited ServeTCP immediately (no listen).
	// Lifetime is SIGINT/SIGTERM. TRACK: BLI-1784969955962654000-dc689643 — hourglass.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) // Background: request-or-shutdown derived
	defer stop()
	cmd.SetContext(ctx)

	// Scrub subprocess-only env that IDE/ensure shells sometimes leak into the daemon.
	// ZQK_PARENT_PID would start a parent-death watcher that os.Exit(0)s the daemon;
	// ZQK_MCP_ACCOUNT_ID on the server process pollutes in-process CLI. Mark ourselves
	// as parent-zqk so nested ExecuteContext skips the child idle watchdog.
	_ = zqkenv.ParentPID().Unset()
	_ = zqkenv.MCPAccountID().Unset()
	_ = zqkenv.IsParentZqk().Set("1")

	return runServe(cmd, args)
}
