package system

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/spf13/cobra"
)

var schedulerRunningChecker = schedulerRunningForRoot
var resolveProjectRootForSchedulerGuard = cli.ResolveProjectRoot

type schedulerCommandRequirement int

const (
	schedulerRequirementNone schedulerCommandRequirement = iota
	schedulerRequirementOptional
	schedulerRequirementRequired
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

func evaluateSchedulerGuard(requirement schedulerCommandRequirement, running, allowDegraded bool) (block bool, warn bool) {
	if running || requirement == schedulerRequirementNone {
		return false, false
	}
	if requirement == schedulerRequirementRequired && !allowDegraded {
		return true, false
	}
	return false, true
}

func schedulerRunningForRoot(projectRoot string) bool {
	if projectRoot == emptyValue {
		return true
	}
	sched, ok := schedulerpkg.GetGlobalSchedulerIfAvailable()
	if ok && sched != nil && sched.IsRunning() {
		return true
	}
	running, _, err := schedulerpkg.IsSchedulerRunning(projectRoot)
	return err == nil && running
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
	projectRoot := resolveProjectRootForSchedulerGuard(".")
	running := schedulerRunningChecker(projectRoot)
	allowDegraded, _ := cmd.Flags().GetBool("allow-degraded")
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
		return errfmt.Errorf(
			"scheduler daemon is not running; command %q requires scheduler-backed maintenance/caches. Start it with 'zqk scheduler start' or re-run with --allow-degraded (see docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md)",
			cmd.Name(),
		)
	}
	logging.Fluent(logger).Warn("Running command while scheduler daemon is not running").
		Command(cmd.Name()).
		String("impact", "maintenance/caches may be stale and results can be partial").
		String("action", "Start scheduler with: zqk scheduler start (details: docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md)").
		Log()
	return nil
}
