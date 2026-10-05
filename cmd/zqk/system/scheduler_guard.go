package system

import (
	"fmt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

type schedulerCommandRequirement = schedulerpkg.CommandRequirement

const (
	schedulerRequirementNone     = schedulerpkg.RequirementNone
	schedulerRequirementOptional = schedulerpkg.RequirementOptional
	schedulerRequirementRequired = schedulerpkg.RequirementRequired
)

var (
	schedulerRunningForRoot             = schedulerpkg.IsSchedulerRunningForRoot
	schedulerRunningChecker             = schedulerRunningForRoot
	resolveProjectRootForSchedulerGuard = cli.ResolveProjectRoot
	evaluateSchedulerGuard              = schedulerpkg.EvaluateSchedulerGuard
)

func requirementForSystemCommand(name string) schedulerCommandRequirement {
	switch name {
	case "aggregate-audit", "retention-tolerance", "compact-stream-state", "compact-wal", "cleanup-duplicates", "cleanup-quarantine", "auto-fix-process-pending":
		return schedulerRequirementRequired
	case "retention-status", "health-data", "auto-fix-batch":
		return schedulerRequirementOptional
	default:
		return schedulerRequirementNone
	}
}

func runSystemSchedulerGuard(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.Name() == "system" {
		return nil
	}
	if err := cli.ValidateAnnotatedKind(cli.KindAnnotKeysSystem, cmd, args, nil); err != nil {
		return err
	}
	req := requirementForSystemCommand(cmd.Name())
	if req == schedulerRequirementNone {
		return nil
	}
	projectRoot, running, allowDegraded := cli.EvaluateCommandSchedulerState(cmd, resolveProjectRootForSchedulerGuard, schedulerRunningChecker)
	block, warn := evaluateSchedulerGuard(req, running, allowDegraded)
	if !block && !warn {
		return nil
	}
	ctx := cli.GetContext(cmd)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if ctx != nil {
		logger = logging.GetLoggerFromProfile(ctx.Profile)
	}
	if block {
		if cli.Confirm("Scheduler daemon is down. Attempt auto-restart? [y/N]") {
			if err := cli.AutoRestartDaemon(projectRoot); err != nil {
				return errfmt.Errorf("failed to auto-restart daemon: %v", err)
			}
			// Allow the command to proceed after restart
			return nil
		}
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("scheduler daemon is not running; command %q requires scheduler-backed maintenance/caches. Start it with 'zqk scheduler start' or re-run with --allow-degraded (see docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md)", cmd.Name())))
	}
	logging.Fluent(logger).Warn("Running command while scheduler daemon is not running").
		Command(cmd.Name()).
		String("impact", "maintenance/caches may be stale and results can be partial").
		String("action", paths.RewriteCanonicalCLIInvocations("Start scheduler with: zqk scheduler start (details: docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md)")).
		Log()
	return nil
}
