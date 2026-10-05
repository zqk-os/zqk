package object

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
	schedulerRunningForRoot             = schedulerpkg.IsSchedulerRunningForRoot
	schedulerRunningChecker             = schedulerRunningForRoot
	resolveProjectRootForSchedulerGuard = cli.ResolveProjectRoot
	evaluateSchedulerGuard              = schedulerpkg.EvaluateSchedulerGuard
)

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

func runObjectSchedulerGuard(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.Name() == "object" {
		return nil
	}
	// License-gate elevated mode for all object verbs (get/update/delete included).
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
	projectRoot, running, allowDegraded := cli.EvaluateCommandSchedulerState(cmd, resolveProjectRootForSchedulerGuard, schedulerRunningChecker)
	block, _ := evaluateSchedulerGuard(req, running, allowDegraded)
	if block {
		if cli.Confirm("Scheduler daemon is down. Attempt auto-restart? [y/N]") {
			if err := cli.AutoRestartDaemon(projectRoot); err != nil {
				return errfmt.Errorf("failed to auto-restart daemon: %v", err)
			}
			// Allow the command to proceed after restart
			return nil
		}
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("scheduler daemon is not running; object command %q requires scheduler-backed maintenance/caches. Start it with 'zqk scheduler start' or re-run with --allow-degraded", cmd.Name())))
	}
	return nil
}
