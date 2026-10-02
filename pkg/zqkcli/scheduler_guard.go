package internal

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

type schedulerCommandRequirement = schedulerpkg.CommandRequirement

const (
	schedulerRequirementNone     = schedulerpkg.RequirementNone
	schedulerRequirementOptional = schedulerpkg.RequirementOptional
	schedulerRequirementRequired = schedulerpkg.RequirementRequired
)

var (
	schedulerRunningForRoot            = schedulerpkg.IsSchedulerRunningForRoot
	schedulerRunningChecker            = schedulerRunningForRoot
	resolveProjectRootForSchedulerGuard = cli.ResolveProjectRoot
	evaluateSchedulerGuard             = schedulerpkg.EvaluateSchedulerGuard
)

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

func runInternalSchedulerGuard(cmd *cobra.Command, _ []string) error { // args reserved for parity with object guard
	if cmd == nil || cmd.Name() == "internal" {
		return nil
	}
	req := requirementForInternalCommand(cmd.Name())
	if req == schedulerRequirementNone {
		return nil
	}
	_, running, allowDegraded := cli.EvaluateCommandSchedulerState(cmd, resolveProjectRootForSchedulerGuard, schedulerRunningChecker)
	block, _ := evaluateSchedulerGuard(req, running, allowDegraded)
	if block {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("scheduler daemon is not running; internal command %q requires scheduler-backed maintenance/caches. Start it with 'zqk scheduler start' or re-run with --allow-degraded", cmd.Name())))
	}
	return nil
}
