package scheduler

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/strutil"
)

// stopScheduler stops the scheduler daemon.
// Idempotent: if the daemon is not running, returns success (already stopped).
// For an external daemon we send SIGTERM first, then emit coordinator events, so we never block
// on storage/coordinator while the daemon holds project-root resources (which caused "command timed out").
// If cmd is non-nil, success/status messages are written via cli.WriteOutput so the user sees feedback (MCP/test routing).
func stopScheduler(ctx *cli.Context, cmd *cobra.Command) error {
	// Outer pipeline wall-clock must cover inner --wait/--max-wait polling plus coordinator/storage tail work;
	// otherwise runSchedulerControlWithTimeout caps at schedulerControlTimeout (20s) and aborts mid-wait.
	wall := schedulerControlTimeout
	if cmd != nil {
		forceKill, errForce := cmd.Flags().GetBool(schedulerFlagForce)
		if errForce != nil {
			return errfmt.Newf("failed to parse 'force' flag").Wrap(errForce)
		}
		waitForExit, errWait := cmd.Flags().GetBool(schedulerFlagWait)
		if errWait != nil {
			return errfmt.Newf("failed to parse 'wait' flag").Wrap(errWait)
		}
		maxWait, errMaxWait := cmd.Flags().GetDuration("max-wait")
		if errMaxWait != nil {
			return errfmt.Newf("failed to get 'max-wait' flag").Wrap(errMaxWait)
		}
		if maxWait <= 0 {
			maxWait = schedulerpkg.StopSchedulerShutdownWait
		}
		if waitForExit && !forceKill {
			const stopWaitTailSlack = 20 * time.Second
			w := maxWait + schedulerCoordinatorTimeout + stopWaitTailSlack
			if w < schedulerControlTimeout {
				w = schedulerControlTimeout
			}
			wall = w
		}
	}

	return runSchedulerControlWithTimeoutDur("scheduler stop", wall, func() error {
		status, err := getSchedulerStatus(ctx)
		if err != nil {
			return err
		}
		if !status.Running {
			logger := logging.GetLoggerFromProfile(strutil.OrDefault(ctx.Profile, schedulerProfileSystem))
			logging.Fluent(logger).Info("Scheduler daemon is not running (already stopped)").Log()
			if cmd != nil {
				if err := cli.WriteOutput(cmd, []byte("Scheduler daemon is not running (already stopped).\n")); err != nil {
					schedulerpkg.SLog(logger).Debug("Failed to write stopped status to output").WithError(err).Log()
				}
			}
			// Still write no-auto-restart so ensure-scheduler-running.sh (cron) does not restart until user runs start
			projectRoot := resolveSchedulerCLIProjectRoot(ctx)
			if projectRoot != emptyValue {
				if err := schedulerpkg.WriteNoAutoRestartFile(projectRoot); err != nil {
					schedulerpkg.SLog(logger).Debug("Failed to write no-auto-restart file during idempotent stop").WithError(err).Log()
				}
			}
			return nil
		}

		projectRoot := resolveSchedulerCLIProjectRoot(ctx)
		profile, err := validateSchedulerProjectRootAndBrand(ctx, projectRoot)
		if err != nil {
			return err
		}

		if status.InProcess {
			sched := schedulerpkg.GetGlobalScheduler()
			if sched != nil {
				logger := logging.GetLoggerFromProfile(profile)
				storageProvider := getStorageProviderForCoordinator(projectRoot)
				stopFields := buildConfigAtShutdownFields(projectRoot)
				emitSchedulerStopEventViaCoordinator(
					pkgctx.NewSystemContext(),
					projectRoot,
					storageProvider,
					"Stopping scheduler daemon (in this process)",
					0,
					profile,
					stopFields,
				)
				sched.Stop()
				if err := schedulerpkg.WriteNoAutoRestartFile(projectRoot); err != nil {
					schedulerpkg.SLog(logger).Debug("Failed to write no-auto-restart file after in-process stop").WithError(err).Log()
				}
				return nil
			}
		}

		forceKill := false
		waitForExit := false
		maxWait := schedulerpkg.StopSchedulerShutdownWait
		if cmd != nil {
			var errForce, errWait, errMaxWait error
			forceKill, errForce = cmd.Flags().GetBool(schedulerFlagForce)
			if errForce != nil {
				return errfmt.Newf("failed to parse 'force' flag").Wrap(errForce)
			}
			waitForExit, errWait = cmd.Flags().GetBool(schedulerFlagWait)
			if errWait != nil {
				return errfmt.Newf("failed to parse 'wait' flag").Wrap(errWait)
			}
			maxWait, errMaxWait = cmd.Flags().GetDuration("max-wait")
			if errMaxWait != nil {
				return errfmt.Newf("failed to get 'max-wait' flag").Wrap(errMaxWait)
			}
			if maxWait <= 0 {
				maxWait = schedulerpkg.StopSchedulerShutdownWait
			}
		}
		// Only print "Scheduler stop completed." when shutdown is verified (avoids mismatch with scheduler status).
		reportStopCompleted := false

		// Send SIGTERM immediately so the daemon gets the signal before we touch storage.
		// Creating storage or emitting events can block while the daemon holds project-root resources.
		if forceKill {
			if err := schedulerpkg.ForceKillSchedulerByPID(status.ProjectRoot); err != nil {
				return errfmt.Newf("failed to force kill scheduler daemon").Wrap(err)
			}
			reportStopCompleted = true
			if cmd != nil {
				if err := cli.WriteOutput(cmd, []byte(fmt.Sprintf(schedulerStopForceMsgFmt, status.ProcessID))); err != nil {
					return err
				}
			}
		} else if waitForExit {
			// SIGTERM once, then poll until exit or maxWait (same budget as StopSchedulerByPIDWithWait).
			pid, sigErr := schedulerpkg.SignalSchedulerByPID(status.ProjectRoot, syscall.SIGTERM)
			if sigErr != nil {
				return errfmt.Errorf(schedulerErrStopDaemonFmt, sigErr)
			}
			if cmd != nil {
				if err := cli.WriteOutput(cmd, []byte(fmt.Sprintf(schedulerStopMessageFmt, pid, maxWait))); err != nil {
					return err
				}
			}
			if err := schedulerpkg.WaitForSchedulerDaemonExit(status.ProjectRoot, pid, maxWait); err != nil {
				logger := logging.GetLoggerFromProfile(strutil.OrDefault(ctx.Profile, schedulerProfileSystem))
				if errWrite := schedulerpkg.WriteNoAutoRestartFile(projectRoot); errWrite != nil {
					schedulerpkg.SLog(logger).Debug("Failed to write no-auto-restart file after wait timeout").WithError(errWrite).Log()
				}
				if cmd != nil {
					if errOutput := cli.WriteOutput(cmd, []byte(fmt.Sprintf(
						"Daemon (PID %d) still running after %v wait (SIGTERM). PID file was kept so you can run stop again.\n"+paths.RewriteCanonicalCLIInvocations("Next: zqk scheduler stop --wait --max-wait <longer>, or zqk scheduler stop --force if stuck. Common causes: storage/hash drain, in-flight jobs, metrics flush.\n"),
						pid, maxWait))); errOutput != nil {
						schedulerpkg.SLog(logger).Debug("Failed to write stop timeout warning").WithError(errOutput).Log()
					}
				}
				return err
			}
			reportStopCompleted = true
		} else {
			pid, sigErr := schedulerpkg.SignalSchedulerByPID(status.ProjectRoot, syscall.SIGTERM)
			if sigErr != nil {
				return errfmt.Errorf(schedulerErrStopDaemonFmt, sigErr)
			}
			if cmd != nil {
				if err := cli.WriteOutput(cmd, []byte(fmt.Sprintf(schedulerStopSigtermMsgFmt, pid))); err != nil {
					return err
				}
				if err := cli.WriteOutput(cmd, []byte(paths.RewriteCanonicalCLIInvocations("Shutdown is in progress; check: zqk scheduler status. To wait: zqk scheduler stop --wait. To force: zqk scheduler stop --force.\n"))); err != nil {
					return err
				}
			}
		}

		// Best-effort: emit stop event and log (may block briefly now that daemon is shutting down).
		// Use timeout to prevent hanging if storage initialization is slow
		stopCtx, stopCancel := context.WithTimeout(pkgctx.NewSystemContext(), schedulerCoordinatorTimeout)
		defer stopCancel()

		// Get storage provider with timeout to avoid blocking on slow initialization
		storageProviderCh := make(chan storagepkg.ObjectStorageProvider, 1)
		goroutinelabels.NewGoroutine("scheduler_core", "get storage provider").
			StartSimple(func() {
				storageProviderCh <- getStorageProviderForCoordinator(projectRoot)
			})
		var storageProvider storagepkg.ObjectStorageProvider
		select {
		case storageProvider = <-storageProviderCh:
		case <-stopCtx.Done():
			// Timeout - continue without storage provider
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Storage provider initialization timed out during scheduler stop, continuing without event emission").Log()
		}

		if storageProvider != nil {
			stopFields := buildConfigAtShutdownFields(projectRoot)
			emitSchedulerStopEventViaCoordinator(
				stopCtx,
				projectRoot,
				storageProvider,
				"Stopping scheduler daemon",
				status.ProcessID,
				profile,
				stopFields,
			)
		}
		if err := schedulerpkg.WriteNoAutoRestartFile(projectRoot); err != nil {
			logger := logging.GetLoggerFromProfile(strutil.OrDefault(ctx.Profile, schedulerProfileSystem))
			schedulerpkg.SLog(logger).Debug("Failed to write no-auto-restart file after daemon stop").WithError(err).Log()
		}
		logger := logging.GetLoggerFromProfile(strutil.OrDefault(ctx.Profile, schedulerProfileSystem))
		if reportStopCompleted {
			schedulerpkg.SLog(logger).Info("Scheduler daemon stop requested (wait/force path completed)").
				Int("pid", status.ProcessID).
				Bool("force", forceKill).
				Bool("wait", waitForExit).
				Log()
			if cmd != nil {
				if err := cli.WriteOutput(cmd, []byte("Scheduler stop completed.\n")); err != nil {
					schedulerpkg.SLog(logger).Debug("Failed to write stop completion message").WithError(err).Log()
				}
			}
		} else if !forceKill && !waitForExit {
			schedulerpkg.SLog(logger).Info("Scheduler daemon stop initiated (async shutdown)").
				Int("pid", status.ProcessID).
				Log()
		}
		return nil
	})
}

// getStorageProviderForCoordinator returns a storage provider for coordinator events (best-effort).
// Uses global cache so we don't create another storage instance.
func getStorageProviderForCoordinator(projectRoot string) storagepkg.ObjectStorageProvider {
	if projectRoot == emptyValue {
		return nil
	}
	storageCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()
	provider, err := storagepkg.GetGlobalStorageProviderCache().GetOrCreate(storageCtx, projectRoot)
	if err != nil {
		return nil
	}
	return provider
}
