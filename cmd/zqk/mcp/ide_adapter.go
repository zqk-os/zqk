package mcp

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/mcp/ideadapter"
	"github.com/spf13/cobra"
)

func NewIDEAdapterCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcIDEAdapterCommandBuilder()
	cmd.RunE = runIDEAdapter
	return cmd
}

// NewCursorAdapterCmd wires the Cursor-preferred alias to the same ide-adapter
// runtime. Help/examples advertise cursor-adapter; without this AddCommand the
// binary treats `mcp cursor-adapter --tcp` as parent flags → unknown --tcp and
// Cursor MCP stays red. TRACK: BLI-MCP-CURSOR-ADAPTER-SYMLINK-001
func NewCursorAdapterCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpSvcCursorAdapterCommandBuilder()
	cmd.RunE = runIDEAdapter
	return cmd
}

func runIDEAdapter(cmd *cobra.Command, _ []string) error {
	var flags clipkg.FlagBag
	addr := resolveTCPFlag(flags.String(cmd, "tcp"))
	if err := flags.Err(); err != nil {
		return err
	}

	cliCtx := cli.GetContext(cmd)
	logger := logging.GetLoggerFromProfile(cliCtx.Profile)
	projectRoot := cli.ResolveProjectRoot(".")

	cfg := ideadapter.LoadConfig(projectRoot)
	if addr != "" {
		cfg.DaemonTCP = addr
	}

	if _, linkErr := mcp.EnsureMCPRoleSymlink(projectRoot, mcp.MCPRoleIDEAdapter, os.Args[0]); linkErr != nil {
		logging.Fluent(logger).Warn("MCP ide-adapter role symlink unavailable").
			WithError(linkErr).
			Log()
	}

	ideadapter.SetDiagLogRoot(projectRoot)
	logging.Fluent(logger).Info("MCP ide-adapter started").
		String("addr", cfg.DaemonTCP).
		Log()

	adapter := ideadapter.New(cfg, logger)

	// Do not derive from cmd.Context(): IDE cancels it after tools/call, which
	// used to exit the adapter process and mark MCP red. Lifetime is stdin EOF;
	// SIGINT/SIGTERM close stdin to unblock the read loop. TRACK:
	// BLI-REDACTED — hourglass/context-refresh soft drain.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) // Background: request-or-shutdown derived
	defer stop()
	goroutinelabels.NewGoroutine("mcp_ide_adapter_stdin_close", "close stdin on SIGINT/SIGTERM soft drain").StartSimple(func() {
		<-ctx.Done()
		_ = os.Stdin.Close()
	})

	return adapter.Run(ctx)
}
