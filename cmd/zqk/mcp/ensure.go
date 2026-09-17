package mcp

import (
	"net"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	mcppkg "github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/spf13/cobra"
)

func NewEnsureCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpEnsureCommandBuilder()
	cmd.RunE = runEnsure
	return cmd
}

func runEnsure(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		tcpAddr := resolveTCPFlag(flags.String(cmd, "tcp"))
		if err := flags.Err(); err != nil {
			return err
		}
		port := tcpPort(tcpAddr)
		projectRoot := projectRootOrResolve(proc.ProjectRoot())
		pidFile := mcpDaemonPIDPath(projectRoot, port)
		logger := logging.GetLoggerFromProfile(proc.Context().Profile)

		binPath := resolveMCPDaemonBinPath(projectRoot)
		// Always refresh IDE role symlinks (daemon + ide-adapter), even when
		// the daemon is already up — rebuilds otherwise leave mcp.json's
		// zqk-mcp-ide-adapter missing and IDE shows a red MCP connector.
		if linkErr := mcppkg.EnsureMCPIDERoleSymlinks(projectRoot, binPath); linkErr != nil {
			logging.Fluent(logger).Warn("mcp IDE role symlinks unavailable").
				WithError(linkErr).
				String("bin", binPath).
				Log()
		}
		// Cursor Customize (~/.cursor/mcp.json) + project .cursor/mcp.json.
		// TRACK: BLI-MCP-CURSOR-ADAPTER-SYMLINK-001 — empty global mcpServers after UI move.
		if instErr := mcppkg.AutoInstall(projectRoot, logger); instErr != nil {
			logging.Fluent(logger).Warn("mcp.json AutoInstall incomplete").
				WithError(instErr).
				Log()
		}

		if pid, ok := getDaemonPIDFromPort(port); ok {
			_ = writePIDFile(pidFile, pid)
			logging.Fluent(logger).Info("mcp-daemon already listening").
				String("addr", tcpAddr).
				Int("pid", pid).
				Log()
			return nil
		}

		// Prefer role symlink so ps shows zqk-mcp-daemon, not bare zqk.
		daemonBin := binPath
		if rolePath := mcppkg.MCPRoleBinPath(projectRoot, mcppkg.MCPRoleDaemon); fileutil.IsRegularFile(rolePath) {
			daemonBin = rolePath
		}
		logFile := mcpDaemonLogPath(projectRoot, port)
		_ = fileutil.EnsureDir(filepath.Dir(logFile))

		logging.Fluent(logger).Info("mcp-daemon starting").
			String("bin", daemonBin).
			String("addr", tcpAddr).
			Log()

		daemonCmd := execwrap.Command(daemonBin, "mcp", "daemon", "--tcp", tcpAddr, "--timeout", "0")
		daemonCmd.Dir = projectRoot
		setDetach(daemonCmd)

		logF, err := fileutil.OpenAppend(logFile)
		if err == nil {
			daemonCmd.Stdout = logF
			daemonCmd.Stderr = logF
		}

		if err := daemonCmd.Start(); err != nil {
			return errfmt.Newf("failed to start daemon").Wrap(err)
		}

		_ = writePIDFile(pidFile, daemonCmd.Process.Pid)
		logging.Fluent(logger).Info("mcp-daemon started").
			Int("pid", daemonCmd.Process.Pid).
			String("addr", tcpAddr).
			Log()

		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			conn, dialErr := net.DialTimeout("tcp", tcpAddr, 300*time.Millisecond)
			if dialErr == nil {
				_ = conn.Close()
				logging.Fluent(logger).Info("mcp-daemon listening").String("addr", tcpAddr).Log()
				return nil
			}
			time.Sleep(150 * time.Millisecond)
		}

		return errfmt.Errorf("mcp-daemon: timed out waiting for listen on %s", tcpAddr)
	})(cmd, nil)
}
