package object

import (
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewPPlanAddCmd creates the pplan add subcommand
func NewPPlanAddCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectPplanAddCommandBuilder()
	cli.BindAsyncProgress(cmd, runPPlanAdd)
	return cmd
}

func runPPlanAdd(cmd *cobra.Command, args []string) error {
	planID, bliIDs := parsePlanAndBLIs(args)
	return executePPlanAdd(cmd, planID, bliIDs...)
}

func executePPlanAdd(cmd *cobra.Command, rawPlanID string, rawBliIDs ...string) error {
	if len(rawBliIDs) == 0 {
		return cli.Guard(cmd).Err(errfmt.Errorf("at least one backlog item ID is required")).Return()
	}
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		planID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawPlanID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve priority plan ID %s: %w", rawPlanID, err)).Return()
		}

		// Read priority plan to verify existence and check status
		plan, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), planID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("priority plan %s not found: %w", planID, err)).Return()
		}

		planKind, _ := plan[objects.FieldKeyKind].(string)
		// If rawPlanID resolved to a backlog_item and we have exactly 1 rawBliID that resolves to priority_plan, swap them
		if planKind == objects.KindBacklogItem && len(rawBliIDs) == 1 {
			if altPlan, altErr := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), rawBliIDs[0]); altErr == nil {
				if altKind, _ := altPlan[objects.FieldKeyKind].(string); altKind == objects.KindPriorityPlan {
					planID, rawBliIDs[0] = rawBliIDs[0], planID
					plan = altPlan
				}
			}
		}

		planStatus, _ := plan[objects.FieldKeyStatus].(string)
		statusRole := objects.GetGlobalStatusChecker()
		// Same rule for the plan: terminal plans cannot accept items.
		if statusRole.Role(objects.KindPriorityPlan, planStatus) == objects.LifecycleRoleTerminal {
			return cli.Guard(cmd).Err(errfmt.Errorf("association denied: priority plan %s is in terminal status %s", planID, planStatus)).Return()
		}

		// Sealed-column immutability (BLI-1789167113049711000-359f8a8c): active or in_progress plans are sealed against new work.
		override := false
		if cmd.Flags().Lookup("override") != nil {
			override, _ = cmd.Flags().GetBool("override")
		}
		if cmd.Flags().Lookup("force") != nil {
			if force, _ := cmd.Flags().GetBool("force"); force {
				override = true
			}
		}
		if planStatus == objects.ObjectStatusInProgress || planStatus == objects.ObjectStatusActive {
			if !override {
				return cli.Guard(cmd).Err(errfmt.Errorf("association denied: priority plan %s is '%s' (sealed / execution-facing); open a grooming plan for new work instead of stuffing the locked column", planID, planStatus)).Return()
			}
			reasonCode := ""
			if cmd.Flags().Lookup("reason-code") != nil {
				reasonCode, _ = cmd.Flags().GetString("reason-code")
			}
			if reasonCode == "" {
				return cli.Guard(cmd).Require(false, "--reason-code is required when using --override on sealed priority plans").Return()
			}
		}

		for _, rawBliID := range rawBliIDs {
			bliID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawBliID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve backlog item ID %s: %w", rawBliID, err)).Return()
			}
			if bliID == planID {
				return cli.Guard(cmd).Err(errfmt.Errorf("cannot associate priority plan %s with itself", planID)).Return()
			}

			// Read backlog item
			bli, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), bliID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("backlog item %s not found: %w", bliID, err)).Return()
			}

			// Shockwave allow/deny rule: the item must not already be terminal.
			if bliStatus, _ := bli[objects.FieldKeyStatus].(string); statusRole.Role(objects.KindBacklogItem, bliStatus) == objects.LifecycleRoleTerminal {
				return cli.Guard(cmd).Err(errfmt.Errorf("association denied: backlog item %s is in terminal status %s", bliID, bliStatus)).Return()
			}

			currentPlanRef, _ := bli[objects.FieldKeyPriorityPlanRef].(string)
			if currentPlanRef == planID {
				cmd.Printf("Backlog item %s is already associated with priority plan %s\n", bliID, planID)
				continue
			}

			// Update backlog_item priority_plan_ref (child-owned membership)
			updates := map[string]any{
				objects.FieldKeyPriorityPlanRef: planID,
			}

			err = proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), bliID, updates)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("failed to update backlog item %s: %w", bliID, err)).Return()
			}

			// Override add onto a live column: increment remaining_open_count (seed from List if unset).
			if planStatus == objects.ObjectStatusInProgress || planStatus == objects.ObjectStatusActive {
				lifecycle.NoteOpenCountableMemberEntered(proc.OperationContext(), proc.Storage(), planID)
			}

			msg := "✓ Associated backlog_item " + bliID + " with priority_plan " + planID
			logging.FluentEvent(proc.Logger()).Info(msg).Log()
			cmd.Printf("%s\n", msg)
		}

		return nil
	})(cmd, append([]string{rawPlanID}, rawBliIDs...))
}

// NewPPlanRemoveCmd creates the pplan remove subcommand
func NewPPlanRemoveCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectPplanRemoveCommandBuilder()
	cli.BindAsyncProgress(cmd, runPPlanRemove)
	return cmd
}

func runPPlanRemove(cmd *cobra.Command, args []string) error {
	planID, bliIDs := parsePlanAndBLIs(args)
	return executePPlanRemove(cmd, planID, bliIDs...)
}

func executePPlanRemove(cmd *cobra.Command, rawPlanID string, rawBliIDs ...string) error {
	if len(rawBliIDs) == 0 {
		return cli.Guard(cmd).Err(errfmt.Errorf("at least one backlog item ID is required")).Return()
	}
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		parkFlag, _ := cmd.Flags().GetBool("park")
		planID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawPlanID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve priority plan ID %s: %w", rawPlanID, err)).Return()
		}

		for _, rawBliID := range rawBliIDs {
			bliID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawBliID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve backlog item ID %s: %w", rawBliID, err)).Return()
			}

			// Read backlog item
			bli, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), bliID)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("backlog item %s not found: %w", bliID, err)).Return()
			}

			// Remove guard: verify that backlog item is currently associated with planID before clearing
			currentPlanRef, _ := bli[objects.FieldKeyPriorityPlanRef].(string)
			if currentPlanRef != "" && currentPlanRef != planID {
				return cli.Guard(cmd).Err(errfmt.Errorf("remove denied: backlog item %s is bound to plan %s, not %s", bliID, currentPlanRef, planID)).Return()
			}
			if currentPlanRef == "" {
				cmd.Printf("Backlog item %s has no priority plan association\n", bliID)
				continue
			}

			updates := map[string]any{
				objects.FieldKeyPriorityPlanRef: storage.FieldUnset,
			}

			statusChecker := objects.GetGlobalStatusChecker()
			currentStatus, _ := bli[objects.FieldKeyStatus].(string)
			if statusChecker.Role(objects.KindBacklogItem, currentStatus) == objects.LifecycleRoleShovelReady {
				// Detached child can no longer be shovel-ready; demote down to grooming/realign role
				updates[objects.FieldKeyStatus] = demoteUnlinkedChildStatus(objects.KindBacklogItem, currentStatus)
			}

			updateCtx := proc.OperationContext()
			err = proc.Storage().Update(updateCtx, proc.SecurityContext(), bliID, updates)
			if err != nil {
				return cli.Guard(cmd).Err(errfmt.Errorf("failed to update backlog item %s: %w", bliID, err)).Return()
			}

			// Override remove must shrink the open-children ledger (and may complete or pause the plan).
			lifecycle.ApplyPlanChildMembershipRemoved(
				proc.OperationContext(),
				proc.Logger(),
				proc.Storage(),
				proc.ProjectRoot(),
				planID,
				bliID,
				lifecycle.WithPark(parkFlag),
			)

			_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), objects.KindBacklogItem)
			_ = storage.FlushListingIndexForProjectRoot(proc.ProjectRoot(), objects.KindPriorityPlan)
			proc.TriggerCacheFreshnessCheck("pplan_remove", []string{objects.KindBacklogItem, objects.KindPriorityPlan})

			msg := "✓ Removed backlog_item " + bliID + " association from priority_plan " + planID
			logging.FluentEvent(proc.Logger()).Info(msg).Log()
			cmd.Printf("%s\n", msg)
		}

		return nil
	})(cmd, append([]string{rawPlanID}, rawBliIDs...))
}

func parsePlanAndBLIs(args []string) (planID string, bliIDs []string) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 1 {
		return args[0], nil
	}
	// Case 1: First argument is PRI-* -> remaining args are BLIs
	if strings.HasPrefix(args[0], "PRI-") {
		return args[0], args[1:]
	}
	// Case 2: Last argument is PRI-* -> preceding args are BLIs
	if strings.HasPrefix(args[len(args)-1], "PRI-") {
		return args[len(args)-1], args[:len(args)-1]
	}
	// Case 3: Exactly two arguments, first is BLI-* -> second is plan
	if len(args) == 2 && strings.HasPrefix(args[0], "BLI-") {
		return args[1], []string{args[0]}
	}
	// Default: args[0] is plan, args[1:] are BLIs
	return args[0], args[1:]
}

func parsePlanAndBLI(arg1, arg2 string) (planID string, bliID string) {
	plan, blis := parsePlanAndBLIs([]string{arg1, arg2})
	if len(blis) > 0 {
		return plan, blis[0]
	}
	return plan, ""
}
