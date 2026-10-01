package mcp

import (
	"os"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func NewSuperviseCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSuperviseCommandBuilder()
	cmd.RunE = runSupervise
	return cmd
}

func runEnsureOnce(projectRoot, tcpAddr string) {
	cmd := execwrap.Command(resolveMCPDaemonBinPath(projectRoot), "mcp", "ensure", "--tcp", tcpAddr, "--timeout", "0")
	cmd.Dir = projectRoot
	_ = cmd.Run()
}

func runSupervise(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		tcpAddr := resolveTCPFlag(flags.String(cmd, "tcp"))
		isStatus := flags.Bool(cmd, "status")
		isStop := flags.Bool(cmd, "stop")
		isLoop := flags.Bool(cmd, "loop")
		if err := flags.Err(); err != nil {
			return err
		}

		port := tcpPort(tcpAddr)
		projectRoot := projectRootOrResolve(proc.ProjectRoot())
		pidFile := mcpDaemonPIDPath(projectRoot, port)
		supPidFile := mcpSupervisePIDPath(projectRoot, port)
		logger := logging.GetLoggerFromProfile(proc.Context().Profile)

		if isStatus {
			return cli.FormatOutput(cmd, superviseStatusPayload(tcpAddr, port, supPidFile))
		}

		if isStop {
			if pid, ok := getSupervisePID(supPidFile); ok {
				killPIDBestEffort(pid)
				_ = fileutil.Remove(supPidFile)
			}
			if pid, ok := readPIDFile(pidFile); ok {
				killPIDBestEffort(pid)
			}
			if pid, ok := getDaemonPIDFromPort(port); ok {
				killPIDBestEffort(pid)
			}
			logging.Fluent(logger).Info("mcp-supervise stopped").String("addr", tcpAddr).Log()
			return nil
		}

		logFile := mcpSuperviseLogPath(projectRoot)
		_ = fileutil.EnsureDir(filepath.Dir(logFile))
		exe := resolveMCPDaemonBinPath(projectRoot)

		// --loop must run before the "already running" check: the parent writes this
		// child's PID to the supervise pidfile before the child enters the loop, so a
		// premature check would self-exit as "already running".
		// TRACK: mcp supervise Setsid child longevity
		if isLoop {
			_ = writePIDFile(supPidFile, os.Getpid())
			for {
				ensureCmd := execwrap.Command(exe, "mcp", "ensure", "--tcp", tcpAddr, "--timeout", "0")
				ensureCmd.Dir = projectRoot
				logF, err := fileutil.OpenAppend(logFile)
				if err == nil {
					ensureCmd.Stdout = logF
					ensureCmd.Stderr = logF
				}
				_ = ensureCmd.Run()
				if logF != nil {
					_ = logF.Close()
				}
				select {
				case <-proc.OperationContext().Done():
					return nil
				case <-time.After(5 * time.Second):
				}
			}
		}

		if pid, ok := getSupervisePID(supPidFile); ok {
			logging.Fluent(logger).Info("mcp-supervise already running").Int("pid", pid).Log()
			runEnsureOnce(projectRoot, tcpAddr)
			return nil
		}

		// --timeout 0: default CLI auto-timeout must not kill the Setsid child.
		// TRACK: same class as mcp daemon spawn in ensure.go.
		supCmd := execwrap.Command(exe, "mcp", "supervise", "--tcp", tcpAddr, "--loop", "--timeout", "0")
		supCmd.Dir = projectRoot
		setDetach(supCmd)
		if logF, err := fileutil.OpenAppend(logFile); err == nil {
			supCmd.Stdout = logF
			supCmd.Stderr = logF
		}

		if err := supCmd.Start(); err != nil {
			return errfmt.Newf("failed to start supervisor").Wrap(err)
		}

		logging.Fluent(logger).Info("mcp-supervise started").Int("pid", supCmd.Process.Pid).Log()
		// Persist the child pid immediately so --status is useful before the loop writes it.
		_ = writePIDFile(supPidFile, supCmd.Process.Pid)

		select {
		case <-proc.OperationContext().Done():
			return proc.OperationContext().Err()
		case <-time.After(300 * time.Millisecond):
		}
		runEnsureOnce(projectRoot, tcpAddr)

		return cli.FormatOutput(cmd, superviseStatusPayload(tcpAddr, port, supPidFile))
	})(cmd, nil)
}
