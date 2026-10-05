package mcp

import (
	"net"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	mcppkg "github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func NewEnsureCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMcpEnsureCommandBuilder()
	cmd.RunE = runEnsure
	return cmd
}

func runEnsure(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		rawTCP := flags.String(cmd, "tcp")
		if err := flags.Err(); err != nil {
			return err
		}
		target := resolveMCPDaemonTarget(proc, rawTCP)
		binPath := resolveMCPDaemonBinPath(target.projectRoot)
		// Always refresh IDE role symlinks (daemon + ide-adapter), even when
		// the daemon is already up — rebuilds otherwise leave mcp.json's
		// zqk-mcp-ide-adapter missing and IDE shows a red MCP connector.
		if linkErr := mcppkg.EnsureMCPIDERoleSymlinks(target.projectRoot, binPath); linkErr != nil {
			logging.Fluent(target.logger).Warn("mcp IDE role symlinks unavailable").
				WithError(linkErr).
				String("bin", binPath).
				Log()
		}
		// Cursor Customize (~/.cursor/mcp.json) + project .cursor/mcp.json.
		// empty global mcpServers after UI move.
		if instErr := mcppkg.AutoInstall(target.projectRoot, target.logger); instErr != nil {
			logging.Fluent(target.logger).Warn("mcp.json AutoInstall incomplete").
				WithError(instErr).
				Log()
		}

		if pid, ok := getDaemonPIDFromPort(target.port); ok {
			_ = writePIDFile(target.pidFile, pid)
			logging.Fluent(target.logger).Info("mcp-daemon already listening").
				String("addr", target.addr).
				Int("pid", pid).
				Log()
			return nil
		}

		// Prefer role symlink so ps shows zqk-mcp-daemon, not bare zqk.
		daemonBin := binPath
		if rolePath := mcppkg.MCPRoleBinPath(target.projectRoot, mcppkg.MCPRoleDaemon); fileutil.IsRegularFile(rolePath) {
			daemonBin = rolePath
		}
		logFile := mcpDaemonLogPath(target.projectRoot, target.port)
		_ = fileutil.EnsureDir(filepath.Dir(logFile))

		logging.Fluent(target.logger).Info("mcp-daemon starting").
			String("bin", daemonBin).
			String("addr", target.addr).
			Log()

		daemonCmd := execwrap.Command(daemonBin, "mcp", "daemon", "--tcp", target.addr, "--timeout", "0")
		daemonCmd.Dir = target.projectRoot
		setDetach(daemonCmd)

		logF, err := fileutil.OpenAppend(logFile)
		if err == nil {
			daemonCmd.Stdout = logF
			daemonCmd.Stderr = logF
		}

		if err := daemonCmd.Start(); err != nil {
			return errfmt.Newf("failed to start daemon").Wrap(err)
		}

		_ = writePIDFile(target.pidFile, daemonCmd.Process.Pid)
		logging.Fluent(target.logger).Info("mcp-daemon started").
			Int("pid", daemonCmd.Process.Pid).
			String("addr", target.addr).
			Log()

		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			conn, dialErr := net.DialTimeout("tcp", target.addr, 300*time.Millisecond)
			if dialErr == nil {
				_ = conn.Close()
				logging.Fluent(target.logger).Info("mcp-daemon listening").String("addr", target.addr).Log()
				return nil
			}
			time.Sleep(150 * time.Millisecond)
		}

		return errfmt.Errorf("mcp-daemon: timed out waiting for listen on %s", target.addr)
	})(cmd, nil)
}
