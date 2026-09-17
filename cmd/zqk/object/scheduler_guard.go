package object

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	schedulerpkg "github.com/lanceman/zqk/pkg/scheduler"
	"github.com/spf13/cobra"
)

type schedulerCommandRequirement int

const (
	schedulerRequirementNone schedulerCommandRequirement = iota
	schedulerRequirementOptional
	schedulerRequirementRequired
)

var schedulerRunningChecker = schedulerRunningForRoot
var resolveProjectRootForSchedulerGuard = cli.ResolveProjectRoot

func requirementForObjectCommand(name string) schedulerCommandRequirement {
	switch name {
	case "bulk", "bulk-delete", "bulk-update", "import":
		return schedulerRequirementOptional
	case "list", "count":
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

func runObjectSchedulerGuard(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.Name() == "object" {
		return nil
	}
	// License-gate elevated mode for all object verbs (get/update/delete included).
	// TRACK: BLI-REDACTED / ATK-REDACTED
	if err := RequireElevatedInternal(cmd); err != nil {
		return err
	}
	if err := validateAnnotatedObjectKind(cmd, args); err != nil {
		return err
	}
	req := requirementForObjectCommand(cmd.Name())
	if req == schedulerRequirementNone {
		return nil
	}
	projectRoot := resolveProjectRootForSchedulerGuard(".")
	running := schedulerRunningChecker(projectRoot)
	allowDegraded, _ := cmd.Flags().GetBool("allow-degraded")
	block, _ := evaluateSchedulerGuard(req, running, allowDegraded)
	if block {
		if cli.Confirm("Scheduler daemon is down. Attempt auto-restart? [y/N]") {
			if err := cli.AutoRestartDaemon(projectRoot); err != nil {
				return errfmt.Errorf("failed to auto-restart daemon: %v", err)
			}
			// Allow the command to proceed after restart
			return nil
		}
		return errfmt.Errorf("scheduler daemon is not running; object command %q requires scheduler-backed maintenance/caches. Start it with 'zqk scheduler start' or re-run with --allow-degraded", cmd.Name())
	}
	return nil
}
