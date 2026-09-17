package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewAddCmd creates the workflow add subcommand
func NewAddCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowAddCommandBuilder()
	cli.BindAsyncProgress(cmd, runAdd)
	cli.RequireStorage(cmd, true)
	return cmd
}

//nolint:gocyclo // Existing complexity
func runAdd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return cli.Guard(cmd).Require(false, "at least one backlog item ID is required").Return()
	}

	var flags clipkg.FlagBag
	milestoneID := flags.String(cmd, "milestone")
	priorityFlag := flags.String(cmd, "priority")
	_ = priorityFlag
	override := flags.Bool(cmd, "override")
	reasonCode := flags.String(cmd, "reason-code")
	if err := flags.Err(); err != nil {
		return err
	}

	if override && reasonCode == "" {
		return cli.Guard(cmd).Require(false, "--reason-code is required when using --override").Return()
	}

	// Smart parse arguments
	var planID string
	var activeOrder int = 1
	var bliIDs []string

	lastArg := args[len(args)-1]
	isPlanOrOrder := false
	if strings.HasPrefix(lastArg, "PRI-") {
		planID = lastArg
		isPlanOrOrder = true
	} else {
		var val int
		_, err := fmt.Sscan(lastArg, &val)
		if err == nil {
			activeOrder = val
			isPlanOrOrder = true
		}
	}

	if isPlanOrOrder {
		bliIDs = args[:len(args)-1]
	} else {
		bliIDs = args
	}

	if len(bliIDs) == 0 {
		return cli.Guard(cmd).Require(false, "at least one backlog item ID is required").Return()
	}

	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		secCtx := proc.SecurityContext()

		if override {
			targetID := ""
			if len(bliIDs) > 0 {
				targetID = bliIDs[0]
			}
			if err := clipkg.EnforceOverrideFriction(cmd, proc.OperationContext(), proc.SecurityContext(), proc.Storage(), targetID, "backlog_item", reasonCode); err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
		}

		var resolvedPlanID string
		var resolvedMilestoneID string
		var autoGoalID string

		if planID != "" {
			// Resolve specific plan ID
			plan, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), planID)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("priority plan not found").Return()
			}
			resolvedPlanID = planID
			planStatus, _ := plan[objects.FieldKeyStatus].(string)
			_ = planStatus
		} else {
			// Search for active priority plan matching active order
			listFilter := storage.ListFilter{
				Kind: "priority_plan",
			}
			planList, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), listFilter)
			if err == nil {
				for _, planObj := range planList.Objects {
					status, _ := planObj[objects.FieldKeyStatus].(string)
					if status == objects.ObjectStatusGrooming || status == objects.ObjectStatusPlanned {
						orderVal, ok := planObj[objects.FieldKeyActiveOrder]
						var order int = 0
						if ok && orderVal != nil {
							if f, ok := orderVal.(float64); ok {
								order = int(f)
							} else if i, ok := orderVal.(int); ok {
								order = i
							}
						}
						if order == activeOrder {
							pID, _ := planObj[objects.FieldKeyID].(string)
							resolvedPlanID = pID
							break
						}
					}
				}
			}
		}

		// If no plan was found/resolved, scaffold new system objects
		if resolvedPlanID == "" {
			fmt.Fprintln(cmd.OutOrStdout(), color.CyanString(fmt.Sprintf("No active plan found for active order %d. Scaffolding new plan, milestone, and goal...", activeOrder)))
			pID, mID, gID, err := scaffoldPlanMilestoneGoal(proc.OperationContext(), proc, activeOrder)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to scaffold plan resources").Return()
			}
			resolvedPlanID = pID
			resolvedMilestoneID = mID
			autoGoalID = gID
			fmt.Fprintln(cmd.OutOrStdout(), color.CyanString(fmt.Sprintf("Scaffolded plan %s, milestone %s, and goal %s", pID, mID, gID)))
		}

		// Resolve milestone
		if resolvedMilestoneID == "" {
			if milestoneID != "" {
				resolvedMilestoneID = milestoneID
			} else {
				// Find milestones referencing this plan
				listFilter := storage.ListFilter{
					Kind: "milestone",
				}
				milestoneList, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), listFilter)
				if err == nil {
					for _, mObj := range milestoneList.Objects {
						mID, _ := mObj[objects.FieldKeyID].(string)
						if milestoneReferencesPlan(mObj, resolvedPlanID) {
							resolvedMilestoneID = mID
							break
						}
					}
				}

				// Fallback: use first in_progress milestone in the system
				if resolvedMilestoneID == "" {
					milestoneList, err = proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), listFilter)
					if err == nil {
						for _, mObj := range milestoneList.Objects {
							mID, _ := mObj[objects.FieldKeyID].(string)
							status, _ := mObj[objects.FieldKeyStatus].(string)
							if status == "in_progress" {
								resolvedMilestoneID = mID
								break
							}
						}
					}
				}
			}
		}

		// Verify milestone exists if resolved
		if resolvedMilestoneID != "" {
			_, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), resolvedMilestoneID)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("resolved milestone not found").Return()
			}
		}

		// Update each backlog item
		var updatedKinds []string
		for _, bliArg := range bliIDs {
			id, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", bliArg)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to resolve item").Return()
			}

			current, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to read backlog item").Return()
			}

			objKind, _ := current[objects.FieldKeyKind].(string)
			if objKind != "backlog_item" {
				return cli.Guard(cmd).Require(false, fmt.Sprintf("object %s is of kind '%s', must be 'backlog_item'", id, objKind)).Return()
			}

			// Ensure item is not in a terminal status
			currentStatus, _ := current[objects.FieldKeyStatus].(string)
			if isTerminal, _ := objects.GetGlobalLifecycleLoader().IsTerminalStatusForKind("backlog_item", currentStatus); isTerminal {
				return cli.Guard(cmd).Require(false, fmt.Sprintf("backlog item %s is already in terminal status '%s'", id, currentStatus)).Return()
			}

			updates := make(map[string]any)
			updates[objects.FieldKeyPriorityPlanRef] = resolvedPlanID

			// Update milestone_refs
			var milestoneRefs []any
			if refsRaw, ok := current[objects.FieldKeyMilestoneRefs]; ok && refsRaw != nil {
				if list, ok := refsRaw.([]any); ok {
					milestoneRefs = list
				}
			}
			if resolvedMilestoneID != "" {
				found := false
				for _, r := range milestoneRefs {
					if rStr, ok := r.(string); ok && rStr == resolvedMilestoneID {
						found = true
						break
					}
				}
				if !found {
					milestoneRefs = append(milestoneRefs, resolvedMilestoneID)
				}
			}
			updates[objects.FieldKeyMilestoneRefs] = milestoneRefs

			// Update goal_refs if missing both goal and req references
			var goalRefs []any
			if refsRaw, ok := current[objects.FieldKeyGoalRefs]; ok && refsRaw != nil {
				if list, ok := refsRaw.([]any); ok {
					goalRefs = list
				}
			}
			var reqRefs []any
			if refsRaw, ok := current[objects.FieldKeyRequirementRefs]; ok && refsRaw != nil {
				if list, ok := refsRaw.([]any); ok {
					reqRefs = list
				}
			}
			if len(goalRefs) == 0 && len(reqRefs) == 0 && autoGoalID != "" {
				goalRefs = append(goalRefs, autoGoalID)
				updates[objects.FieldKeyGoalRefs] = goalRefs
			}

			// Set priority
			priority := priorityFlag
			if priority == "" {
				if existingPriority, ok := current[objects.FieldKeyPriority].(string); ok && existingPriority != "" {
					priority = existingPriority
				} else {
					priority = "medium"
				}
			}
			updates[objects.FieldKeyPriority] = priority

			// planned/in_progress require priority_tier (lifecycle precondition).
			// Keep priority and priority_tier strictly synchronized.
			// TRACK: TDE-ENV-ALLOW-CI-OVERRIDES-STALE-TESTS-001
			if tier, ok := objects.PriorityToTier(priority); ok {
				updates[objects.FieldKeyPriorityTier] = tier
			} else if tier, ok := current[objects.FieldKeyPriorityTier].(string); ok && strings.TrimSpace(tier) != "" {
				updates[objects.FieldKeyPriorityTier] = tier
			} else {
				updates[objects.FieldKeyPriorityTier] = "P2"
			}

			// Toggle status to planned
			updates[objects.FieldKeyStatus] = "planned"

			// Build cache context
			opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), id, objKind, "")
			if override {
				opCtx = pkgctx.WithLifecycleBreakGlass(opCtx, "workflow add force")
			}

			// Update object
			if err := proc.Storage().Update(opCtx, secCtx, id, updates); err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to update backlog item").Return()
			}

			fmt.Fprintln(cmd.OutOrStdout(), color.GreenString(fmt.Sprintf("✓ Backlog item %s added to plan %s (milestone: %s, priority: %s, status: planned)", id, resolvedPlanID, resolvedMilestoneID, priority)))
			updatedKinds = append(updatedKinds, objKind)
		}

		// Flush mutation visibility
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), updatedKinds); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Objects updated successfully, but index refresh is delayed."))
		}

		// Trigger cache freshness check
		proc.TriggerCacheFreshnessCheck("update", updatedKinds)

		return nil
	})(cmd, args)
}

// milestoneReferencesPlan reports whether a milestone object links to planID via
// priority_plan_refs (list) or priority_plan_ref (singular).
func milestoneReferencesPlan(mObj map[string]any, planID string) bool {
	if planID == "" || mObj == nil {
		return false
	}
	if ref, ok := mObj[objects.FieldKeyPriorityPlanRef].(string); ok && ref == planID {
		return true
	}
	refsRaw, ok := mObj[objects.FieldKeyPriorityPlanRefs]
	if !ok || refsRaw == nil {
		return false
	}
	switch refs := refsRaw.(type) {
	case []any:
		for _, refRaw := range refs {
			if refStr, ok := refRaw.(string); ok && refStr == planID {
				return true
			}
		}
	case []string:
		for _, refStr := range refs {
			if refStr == planID {
				return true
			}
		}
	}
	return false
}

func scaffoldPlanMilestoneGoal(ctx context.Context, proc *cli.Processor, activeOrder int) (string, string, string, error) {
	timestamp := time.Now().UnixNano()
	planID := fmt.Sprintf("PRI-AUTO-%d", timestamp)
	milestoneID := fmt.Sprintf("MIL-AUTO-%d", timestamp)
	goalID := fmt.Sprintf("GOAL-AUTO-%d", timestamp)

	secCtx := proc.SecurityContext()

	// 1. Create Goal
	goal := map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        "goal",
		objects.FieldKeyTitle:       fmt.Sprintf("Auto-generated Goal %d", timestamp),
		objects.FieldKeyDescription: "Auto-generated Goal to satisfy hierarchical integrity",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyNamespaceID: "zqk:kernel",
		objects.FieldKeyCreatedAt:   time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:   "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:   time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:   "ACC-1785920548450214012-68b850c0",
	}
	if err := proc.Storage().Create(ctx, secCtx, goal); err != nil {
		return "", "", "", errfmt.Newf("failed to create auto goal").Wrap(err)
	}

	// 2. Create Priority Plan
	plan := map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        "priority_plan",
		objects.FieldKeyTitle:       fmt.Sprintf("Auto-generated Plan (Active Order %d)", activeOrder),
		objects.FieldKeyDescription: "Auto-generated Priority Plan via workflow add",
		objects.FieldKeyStatus:      objects.ObjectStatusGrooming,
		objects.FieldKeyActiveOrder: activeOrder,
		objects.FieldKeyNamespaceID: "zqk:kernel",
		objects.FieldKeyCreatedAt:   time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:   "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:   time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:   "ACC-1785920548450214012-68b850c0",
	}
	if err := proc.Storage().Create(ctx, secCtx, plan); err != nil {
		return "", "", "", errfmt.Newf("failed to create auto priority plan").Wrap(err)
	}

	// 3. Create Milestone
	milestone := map[string]any{
		objects.FieldKeyID:               milestoneID,
		objects.FieldKeyKind:             "milestone",
		objects.FieldKeyTitle:            fmt.Sprintf("Auto-generated Milestone %d", timestamp),
		objects.FieldKeyDescription:      "Auto-generated Milestone linking auto plan and goal",
		objects.FieldKeyStatus:           objects.ObjectStatusNotStarted,
		objects.FieldKeyPriorityPlanRefs: []any{planID},
		objects.FieldKeyGoalRefs:         []any{goalID},
		objects.FieldKeyNamespaceID:      "zqk:kernel",
		objects.FieldKeyCreatedAt:        time.Now().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:        "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:        time.Now().Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:        "ACC-1785920548450214012-68b850c0",
	}
	if err := proc.Storage().Create(ctx, secCtx, milestone); err != nil {
		return "", "", "", errfmt.Newf("failed to create auto milestone").Wrap(err)
	}

	// Flush mutation visibility for the newly scaffolded kinds to ensure they are visible for validation
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	_ = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{"goal", "priority_plan", "milestone"})

	return planID, milestoneID, goalID, nil
}
