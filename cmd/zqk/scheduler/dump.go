package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

// NewDumpCmd creates the command to capture a process dump from the running scheduler daemon.
func NewDumpCmd() *cobra.Command {
	dumpCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerDumpCommandBuilder(), &cobra.Command{
		Use:   "dump",
		Short: "Capture process dump from the running scheduler daemon",
		Long: "Sends SIGUSR1 to the scheduler daemon to capture goroutine dump, heap profile, " +
			"and thread info to .zqk/scheduler/diagnostics/. Use this when threads or goroutines are escalating.",
	})
	cli.AddCommonFlags(dumpCmd)
	dumpCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx := cli.GetContext(cmd)
		if ctx == nil {
			return errfmt.Errorf("failed to get context")
		}
		return runDump(ctx, cmd)
	}
	return dumpCmd
}

func runDump(ctx *cli.Context, cmd *cobra.Command) error {
	if runtime.GOOS == "windows" {
		return errfmt.Errorf("scheduler dump is not supported on Windows (SIGUSR1 not available)")
	}

	// Use same status logic as "scheduler status" so both commands agree (same project root and running check).
	status, err := getSchedulerStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Running {
		return errfmt.Errorf("scheduler daemon is not running; start it with 'zqk scheduler start' first")
	}

	pid := status.ProcessID
	if status.InProcess {
		pid = os.Getpid()
	}
	if pid <= 0 {
		return errfmt.Errorf("scheduler daemon is running but could not determine PID")
	}

	if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
		return errfmt.Errorf("failed to send SIGUSR1 to daemon (PID %d): %w", pid, err)
	}

	diagnosticsDir := filepath.Join(status.ProjectRoot, paths.ProjectDataDir, paths.SchedulerDir, "diagnostics")
	var b strings.Builder
	fmt.Fprintf(&b, "Process dump requested (SIGUSR1 sent to PID %d).\n", pid)
	fmt.Fprintf(&b, "Daemon will write to: %s\n", diagnosticsDir)
	fmt.Fprintf(&b, "Check diagnostics.jsonl for scheduler_command operation=dump status=started (immediate) and status=complete or status=error. Capture can take a minute with many goroutines.\n")

	return cli.WriteOutput(cmd, []byte(b.String()))
}
