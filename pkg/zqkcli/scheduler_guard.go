package internal

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

type schedulerCommandRequirement int

const (
	schedulerRequirementNone schedulerCommandRequirement = iota
	schedulerRequirementOptional
	schedulerRequirementRequired
)

var schedulerRunningChecker = schedulerRunningForRoot
var resolveProjectRootForSchedulerGuard = cli.ResolveProjectRoot

func requirementForInternalCommand(name string) schedulerCommandRequirement {
	switch name {
	case "bulk", "process":
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

func runInternalSchedulerGuard(cmd *cobra.Command, _ []string) error { // args reserved for parity with object guard
	if cmd == nil || cmd.Name() == "internal" {
		return nil
	}
	req := requirementForInternalCommand(cmd.Name())
	if req == schedulerRequirementNone {
		return nil
	}
	projectRoot := resolveProjectRootForSchedulerGuard(".")
	running := schedulerRunningChecker(projectRoot)
	allowDegraded, _ := cmd.Flags().GetBool("allow-degraded")
	block, _ := evaluateSchedulerGuard(req, running, allowDegraded)
	if block {
		return errfmt.Errorf("scheduler daemon is not running; internal command %q requires scheduler-backed maintenance/caches. Start it with 'zqk scheduler start' or re-run with --allow-degraded", cmd.Name())
	}
	return nil
}
