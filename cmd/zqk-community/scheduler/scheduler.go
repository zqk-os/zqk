// Package scheduler is the community-only start/stop/status surface.
// Studio cmd/zqk/scheduler retains the full scheduler command tree.
// TRACK: TDE-1789699310016987000-5703b344
package scheduler

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewSchedulerCmd returns start/stop/status for the community binary.
func NewSchedulerCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerCommandBuilder()
	cli.RequireSession(cmd, false)
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newStatusCmd())
	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerStartCommandBuilder()
	cli.RequireSchedulerCheck(cmd, false)
	// TRACK: TDE-1789699310016987000-5703b344 — remove when scheduler start flags live in command DNA.
	cmd.Flags().Bool("background", true, "Run in background and return immediately (default)")
	cmd.Flags().Bool("foreground", false, "Run in foreground (attach to terminal)")
	cmd.RunE = runStart
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerStopCommandBuilder()
	cli.RequireSchedulerCheck(cmd, false)
	cmd.RunE = runStop
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerStatusCommandBuilder()
	cli.RequireSchedulerCheck(cmd, false)
	cli.RequireSession(cmd, false)
	cmd.RunE = runStatus
	return cmd
}

func runStart(cmd *cobra.Command, _ []string) error {
	root := cli.ResolveProjectRoot(".")
	logger := logging.GetLoggerFromProfile("")
	running, pid, err := schedulerpkg.IsSchedulerRunning(root)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	if running {
		return cli.Guard(cmd).Err(errfmt.Errorf("scheduler daemon is already running (PID: %d)", pid)).Return()
	}
	foreground, _ := cmd.Flags().GetBool("foreground")
	if foreground {
		return runForeground(cmd, root, logger)
	}
	exe, err := fileutil.Executable()
	if err != nil || exe == "" {
		return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve this binary for detached start")).Return()
	}
	child := execwrap.Command(exe, "scheduler", "start", "--foreground")
	child.Dir = root
	child.Env = os.Environ()
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to start scheduler daemon: %w").Return()
	}
	logging.Fluent(logger).Info("Scheduler daemon started").Int("pid", child.Process.Pid).Log()
	return nil
}

func runForeground(cmd *cobra.Command, root string, logger logging.Logger) error {
	systemCtx := pkgctx.NewSystemContext()
	storageCtx, cancel := context.WithTimeout(systemCtx, 30*time.Second)
	defer cancel()
	factory, err := storagepkg.NewStorageFactory(storageCtx, root)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to create storage factory: %w").Return()
	}
	var provider storagepkg.ObjectStorageProvider
	if factory != nil {
		provider = factory.GetStorage()
	}
	sched := schedulerpkg.NewSchedulerWithProjectRoot(
		provider,
		objects.GetGlobalSpecLoader(),
		objects.GetGlobalLifecycleLoader(),
		root,
		system.NewObjectIDCacheBuilderForScheduler(),
	)
	sched.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	if err := schedulerpkg.WritePIDFile(root); err != nil {
		logging.Fluent(logger).Warn("Failed to write scheduler PID file").WithError(err).Log()
	}
	logging.Fluent(logger).Info("Starting scheduler daemon in foreground").ProjectRoot(root).Log()
	if err := sched.Start(pkgctx.NewSystemContext()); err != nil {
		_ = schedulerpkg.RemovePIDFile(root)
		return cli.Guard(cmd).Err(err).Wrapf("scheduler start failed: %w").Return()
	}
	return nil
}

func runStop(cmd *cobra.Command, _ []string) error {
	root := cli.ResolveProjectRoot(".")
	logger := logging.GetLoggerFromProfile("")
	if err := schedulerpkg.StopSchedulerByPID(root); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to stop scheduler daemon: %w").Return()
	}
	logging.Fluent(logger).Info("Scheduler daemon stop signaled").Log()
	return nil
}

func runStatus(cmd *cobra.Command, _ []string) error {
	root := cli.ResolveProjectRoot(".")
	running, pid, err := schedulerpkg.IsSchedulerRunning(root)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	status := "not running"
	if running {
		status = "running"
	}
	payload := map[string]any{
		objects.FieldKeyStatus: status,
		"pid":                  pid,
		"project_root":         root,
	}
	return cli.FormatOutput(cmd, payload)
}
