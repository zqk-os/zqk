package object

import (
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewPPlanAddCmd creates the pplan add subcommand
func NewPPlanAddCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("add")
	builder.WithShort("Add a backlog item to a priority plan (child-owned priority_plan_ref)")
	help := clipkg.DynamicHelpBuilder("Add a backlog item to a priority plan")
	help.WithDescriptionLines("Associates a backlog item with a priority plan by updating the backlog item's priority_plan_ref.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Note: Priority plan membership is child-owned on BLI.priority_plan_ref.")
	help.AddExample("Add backlog item to plan", "%s pplan add PLAN-123456789012345678-abcdef120-bdc4dd36 ITEM-REDACTED-de06babe")
	help.AddExample("Alternative positional syntax", "%s pplan PLAN-123456789012345678-abcdef120-bdc4dd36 add ITEM-REDACTED-de06babe")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(2))

	cmd := builder.Build()
	cli.BindAsyncProgress(cmd, runPPlanAdd)
	return cmd
}

func runPPlanAdd(cmd *cobra.Command, args []string) error {
	planID, bliID := parsePlanAndBLI(args[0], args[1])
	return executePPlanAdd(cmd, planID, bliID)
}

func executePPlanAdd(cmd *cobra.Command, rawPlanID, rawBliID string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		planID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawPlanID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve priority plan ID %s: %w", rawPlanID, err)).Return()
		}
		bliID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawBliID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve backlog item ID %s: %w", rawBliID, err)).Return()
		}

		// Read backlog item
		bli, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), bliID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("backlog item %s not found: %w", bliID, err)).Return()
		}

		// Shockwave allow/deny rule: verify backlog item status is not terminal (archived, rejected)
		if bliStatus, _ := bli[objects.FieldKeyStatus].(string); bliStatus == pplanStatusArchived || bliStatus == pplanStatusRejected {
			return cli.Guard(cmd).Err(errfmt.Errorf("association denied: backlog item %s is in terminal status %s", bliID, bliStatus)).Return()
		}

		// Read priority plan to verify existence and check status
		plan, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), planID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("priority plan %s not found: %w", planID, err)).Return()
		}

		// Shockwave allow/deny rule: verify priority plan is not in terminal/closed status
		if planStatus, _ := plan[objects.FieldKeyStatus].(string); planStatus == pplanStatusArchived || planStatus == pplanStatusRejected || planStatus == pplanStatusComplete {
			return cli.Guard(cmd).Err(errfmt.Errorf("association denied: priority plan %s is in terminal status %s", planID, planStatus)).Return()
		}

		// Update backlog_item priority_plan_ref (child-owned membership)
		updates := map[string]any{
			"priority_plan_ref": planID,
		}

		err = proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), bliID, updates)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to update backlog item %s: %w", bliID, err)).Return()
		}

		msg := "✓ Associated backlog_item " + bliID + " with priority_plan " + planID
		logging.FluentEvent(proc.Logger()).Info(msg).Log()
		cmd.Printf("%s\n", msg)
		return nil
	})(cmd, []string{rawPlanID, rawBliID})
}

// NewPPlanRemoveCmd creates the pplan remove subcommand
func NewPPlanRemoveCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("remove")
	builder.WithShort("Remove a backlog item association from a priority plan")
	help := clipkg.DynamicHelpBuilder("Remove a backlog item association from a priority plan")
	help.WithDescriptionLines("Clears priority_plan_ref from the specified backlog item.")
	help.AddExample("Remove backlog item from plan", "%s pplan remove PLAN-123456789012345678-abcdef120-bdc4dd36 ITEM-REDACTED-de06babe")
	help.AddExample("Alternative positional syntax", "%s pplan PLAN-123456789012345678-abcdef120-bdc4dd36 remove ITEM-REDACTED-de06babe")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(2))

	cmd := builder.Build()
	cli.BindAsyncProgress(cmd, runPPlanRemove)
	return cmd
}

func runPPlanRemove(cmd *cobra.Command, args []string) error {
	planID, bliID := parsePlanAndBLI(args[0], args[1])
	return executePPlanRemove(cmd, planID, bliID)
}

func executePPlanRemove(cmd *cobra.Command, rawPlanID, rawBliID string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		planID, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", rawPlanID)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to resolve priority plan ID %s: %w", rawPlanID, err)).Return()
		}
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
		currentPlanRef, _ := bli[pplanFieldPriorityRef].(string)
		if currentPlanRef != "" && currentPlanRef != planID {
			return cli.Guard(cmd).Err(errfmt.Errorf("remove denied: backlog item %s is bound to plan %s, not %s", bliID, currentPlanRef, planID)).Return()
		}

		// Clear priority_plan_ref on backlog_item
		updates := map[string]any{
			"priority_plan_ref": "",
		}

		err = proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), bliID, updates)
		if err != nil {
			return cli.Guard(cmd).Err(errfmt.Errorf("failed to update backlog item %s: %w", bliID, err)).Return()
		}

		msg := "✓ Removed backlog_item " + bliID + " association from priority_plan " + planID
		logging.FluentEvent(proc.Logger()).Info(msg).Log()
		cmd.Printf("%s\n", msg)
		return nil
	})(cmd, []string{rawPlanID, rawBliID})
}

func parsePlanAndBLI(arg1, arg2 string) (planID string, bliID string) {
	if strings.HasPrefix(arg1, "PLAN-") || strings.HasPrefix(arg2, "ITEM-") {
		return arg1, arg2
	}
	if strings.HasPrefix(arg2, "PLAN-") || strings.HasPrefix(arg1, "ITEM-") {
		return arg2, arg1
	}
	return arg1, arg2
}
