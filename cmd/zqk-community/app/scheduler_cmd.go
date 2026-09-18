package app

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewCommunitySchedulerCmd is the community-bounded daemon surface (start/stop/status).
// Studio's cmd/zqk/scheduler tree is not compiled here (it imports cmd/zqk/agent).
func NewCommunitySchedulerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduler",
		Short: "Background job daemon (start, stop, status)",
		Long:  "Runs retention, audit aggregation, and one-shot jobs already written to the kernel. Init seeds those jobs in-process. First-run object CRUD does not require the daemon.",
	}
	cmd.AddCommand(newCommunitySchedulerStartCmd())
	cmd.AddCommand(newCommunitySchedulerStopCmd())
	cmd.AddCommand(newCommunitySchedulerStatusCmd())
	return cmd
}

func newCommunitySchedulerStartCmd() *cobra.Command {
	var foreground bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the community scheduler daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			projectRoot := cli.ResolveProjectRoot(".")
			if projectRoot == EmptyValue {
				return errfmt.Errorf("project root not found; run from a directory with .zqk/ after %s system init", brand.ExecutableName())
			}
			running, pid, err := scheduler.IsSchedulerRunning(projectRoot)
			if err == nil && running {
				logging.Fluent(logger).Info("Scheduler already running").
					Int("pid", pid).
					String("project_root", projectRoot).
					Log()
				return nil
			}
			if !foreground {
				return startCommunitySchedulerDetached(projectRoot, logger)
			}
			return startCommunitySchedulerForeground(cmd, projectRoot, logger)
		},
	}
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run in this terminal (used by detached start)")
	return cmd
}

func startCommunitySchedulerDetached(projectRoot string, logger logging.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return errfmt.Newf("resolve %s binary", brand.ExecutableName()).Wrap(err)
	}
	child := exec.Command(exe, "scheduler", "start", "--foreground")
	child.Dir = projectRoot
	child.Env = append(os.Environ(), zqkenv.ProjectRoot().Name()+"="+projectRoot)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	child.Stdout = nil
	child.Stderr = nil
	if err := child.Start(); err != nil {
		return errfmt.Newf("start detached scheduler").Wrap(err)
	}
	logging.Fluent(logger).Info("Scheduler daemon starting").
		Int("pid", child.Process.Pid).
		String("project_root", projectRoot).
		String("note", brand.ExecutableName()+" scheduler status").
		Log()
	_ = child.Process.Release()
	return nil
}

func startCommunitySchedulerForeground(cmd *cobra.Command, projectRoot string, logger logging.Logger) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	storageFactory, err := storagepkg.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("storage factory").Wrap(err)
	}
	var provider storagepkg.ObjectStorageProvider
	if storageFactory != nil {
		provider = storageFactory.GetStorage()
	}
	sched := scheduler.NewSchedulerWithProjectRoot(
		provider,
		objects.GetGlobalSpecLoader(),
		objects.GetGlobalLifecycleLoader(),
		projectRoot,
		system.NewObjectIDCacheBuilderForScheduler(),
	)
	sched.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	if err := scheduler.WritePIDFile(projectRoot); err != nil {
		logging.Fluent(logger).Warn("Could not write scheduler PID file").WithError(err).Log()
	}
	logging.Fluent(logger).Info("Scheduler running in foreground").
		String("project_root", projectRoot).
		Log()
	return sched.Start(ctx)
}

func newCommunitySchedulerStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the community scheduler daemon",
		RunE: func(_ *cobra.Command, _ []string) error {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			projectRoot := cli.ResolveProjectRoot(".")
			if projectRoot == EmptyValue {
				return errfmt.Errorf("project root not found")
			}
			if err := scheduler.StopSchedulerByPID(projectRoot); err != nil {
				return err
			}
			logging.Fluent(logger).Info("Scheduler stop signaled").
				String("project_root", projectRoot).
				Log()
			return nil
		},
	}
}

func newCommunitySchedulerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the community scheduler daemon is running",
		RunE: func(_ *cobra.Command, _ []string) error {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			projectRoot := cli.ResolveProjectRoot(".")
			if projectRoot == EmptyValue {
				return errfmt.Errorf("project root not found")
			}
			running, pid, err := scheduler.IsSchedulerRunning(projectRoot)
			if err != nil {
				return err
			}
			status := "not running"
			if running {
				status = "running"
			}
			logging.Fluent(logger).Info("Scheduler status").
				String("status", status).
				Int("pid", pid).
				String("project_root", projectRoot).
				Log()
			return nil
		},
	}
}
