package kernel

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernel/steward"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	"github.com/zqk-os/zqk/pkg/storage"
)

func newStewardCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKernelStewardCommandBuilder()
	cmd.AddCommand(newStewardDaemonCmd())
	cmd.AddCommand(newStewardSweepCmd())
	return cmd
}

type systemHygieneProvider struct {
	projectRoot string
}

func (p *systemHygieneProvider) ReapStaleLocks(ctx context.Context) (int, error) {
	cnt, _, err := resourcehygiene.ReapStaleLocks(p.projectRoot, resourcehygiene.DefaultLockStaleAge, false)
	return cnt, err
}

func (p *systemHygieneProvider) ReapTempFiles(ctx context.Context) (int, error) {
	cnt, _, _, err := resourcehygiene.ReapOrphanedTempFiles(p.projectRoot, 30*time.Minute, false)
	return cnt, err
}

func (p *systemHygieneProvider) CompactWAL(ctx context.Context) error {
	return storage.CompactWAL(p.projectRoot)
}

type kernelPlanProvider struct {
	sp storage.ObjectStorageProvider
}

func (p *kernelPlanProvider) ListCandidatePlans(ctx context.Context) ([]steward.PriorityPlanSummary, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	res, err := p.sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: objects.KindPriorityPlan,
	})
	if err != nil {
		return nil, err
	}
	var out []steward.PriorityPlanSummary
	for _, obj := range res.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		blis, _ := obj[objects.FieldKeyBacklogItemRefs].([]any)
		out = append(out, steward.PriorityPlanSummary{
			ID:          id,
			Status:      status,
			ShovelReady: len(blis) > 0 && (status == "planned" || status == "active"),
		})
	}
	return out, nil
}

func newStewardDaemonCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKernelStewardDaemonCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		intervalSec, _ := cmd.Flags().GetInt("interval")
		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			projectRoot = paths.ResolveProjectRoot(".")
		}
		if projectRoot == "" {
			return errfmt.Errorf("project root not found")
		}

		daemonLock, err := singleton.AcquireDaemonLock(projectRoot, "steward")
		if err != nil {
			return errfmt.Errorf("failed to acquire steward daemon lock: %w", err)
		}
		defer daemonLock.Release()

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("Kernel steward daemon starting").
			String("project_root", projectRoot).
			Int("interval_seconds", intervalSec).
			Log()

		hygiene := &systemHygieneProvider{projectRoot: projectRoot}
		plans := &kernelPlanProvider{sp: proc.Storage()}
		monitor := steward.NewRunwayMonitor(steward.MinimumShovelReadyRunway)
		daemon := steward.NewDaemon(hygiene, plans, monitor)

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if intervalSec <= 0 {
			intervalSec = 30
		}
		ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
		defer ticker.Stop()

		// Run initial sweep on boot
		sweep, err := daemon.ExecuteSweep(ctx)
		if err != nil {
			logging.Fluent(logger).Warn("Initial kernel steward sweep error").WithError(err).Log()
		} else {
			logSweepSummary(logger, sweep)
		}

		for {
			select {
			case <-ctx.Done():
				logging.Fluent(logger).Info("Kernel steward daemon shutting down").Log()
				return nil
			case <-ticker.C:
				sweep, err := daemon.ExecuteSweep(ctx)
				if err != nil {
					logging.Fluent(logger).Warn("Kernel steward sweep error").WithError(err).Log()
				} else {
					logSweepSummary(logger, sweep)
				}
			}
		}
	})
	return cmd
}

func newStewardSweepCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKernelStewardSweepCommandBuilder()
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			projectRoot = paths.ResolveProjectRoot(".")
		}
		if projectRoot == "" {
			return errfmt.Errorf("project root not found")
		}

		hygiene := &systemHygieneProvider{projectRoot: projectRoot}
		plans := &kernelPlanProvider{sp: proc.Storage()}
		monitor := steward.NewRunwayMonitor(steward.MinimumShovelReadyRunway)
		daemon := steward.NewDaemon(hygiene, plans, monitor)

		sweep, err := daemon.ExecuteSweep(cmd.Context())
		if err != nil {
			return errfmt.Newf("execute kernel steward sweep").Wrap(err)
		}

		return cli.FormatOutput(cmd, sweep)
	})
	return cmd
}

func logSweepSummary(logger logging.Logger, sweep *steward.SweepResult) {
	if sweep == nil {
		return
	}
	entry := logging.Fluent(logger).Info("Kernel steward sweep complete").
		String("sweep_id", sweep.SweepID).
		Int("locks_reaped", sweep.LocksReaped).
		Int("temp_files_reaped", sweep.TempFilesReaped).
		Bool("wal_compacted", sweep.WALCompacted).
		Int("shovel_ready_buffer", sweep.Runway.ShovelReadyBuffer).
		Int("target_buffer", sweep.Runway.TargetBuffer).
		Bool("starvation_risk", sweep.Runway.IsStarvationRisk).
		String("duration", sweep.Duration.String())
	if len(sweep.SignalsGenerated) > 0 {
		entry.Int("signals_generated", len(sweep.SignalsGenerated))
	}
	entry.Log()
}
