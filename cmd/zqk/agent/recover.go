package agent

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewRecoverCmd creates the recover command
func NewRecoverCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentRecoverCommandBuilder()
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Flags().Bool("stale", false, "Include stale in_progress tasks")
	cmd.Flags().Bool("all", false, "Recover across all plans")
	cmd.RunE = runRecover
	return cmd
}

func runRecover(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		sp := proc.Storage()
		if sp == nil {
			return errfmt.Errorf("storage provider is not available")
		}

		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		staleFlag, _ := cmd.Flags().GetBool("stale")
		allFlag, _ := cmd.Flags().GetBool("all")

		var planID string
		if len(args) > 0 && !allFlag {
			planArg := args[0]
			var err error
			planID, err = proc.ResolveSemanticArgument(ctx, objects.KindPriorityPlan, planArg)
			if err != nil {
				pipelineID, pErr := proc.ResolveSemanticArgument(ctx, objects.KindPipeline, planArg)
				if pErr == nil {
					planID = pipelineID
				} else {
					stratID, sErr := proc.ResolveSemanticArgument(ctx, objects.KindStrategicPlan, planArg)
					if sErr != nil {
						return errfmt.Newf("semantic routing failed for priority_plan, pipeline, and strategic_plan").Wrap(err)
					}
					planID = stratID
				}
			}
		}

		var tasks []map[string]any
		if planID != "" {
			var listed []map[string]any
			for _, filter := range recoverAgentTaskListFilters(planID) {
				res, listErr := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
				if listErr != nil {
					return errfmt.Newf("failed to query agent tasks").Wrap(listErr)
				}
				if res != nil {
					listed = append(listed, res.Objects...)
				}
			}
			resInProg, _ := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
				Kind: objects.KindAgentTask,
				Filters: map[string]any{
					objects.FieldKeyPriorityPlanRef: planID,
					objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
				},
			})
			if resInProg != nil {
				listed = append(listed, resInProg.Objects...)
			}
			tasks = mergeObjectsByID(listed)
		} else {
			// Cross-plan recovery: find all failed/error tasks and in_progress tasks
			var listed []map[string]any
			for _, st := range []string{objects.ObjectStatusError, objects.ObjectStatusFailed, objects.ObjectStatusInProgress} {
				res, listErr := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
					Kind: objects.KindAgentTask,
					Filters: map[string]any{
						objects.FieldKeyStatus: st,
					},
				})
				if listErr == nil && res != nil {
					listed = append(listed, res.Objects...)
				}
			}
			tasks = mergeObjectsByID(listed)
		}

		staleThreshold := 1 * time.Hour
		now := time.Now().UTC()
		recoveredCount := 0
		archivedCount := 0
		checker := objects.GetGlobalStatusChecker()

		for _, obj := range tasks {
			id, _ := obj[objects.FieldKeyID].(string)
			title, _ := obj[objects.FieldKeyTitle].(string)
			status, _ := obj[objects.FieldKeyStatus].(string)
			planRef, _ := obj[objects.FieldKeyPriorityPlanRef].(string)

			// Check parent plan status
			parentIsTerminal := false
			if planRef != "" {
				if parentObj, err := sp.Read(ctx, secCtx, planRef); err == nil && parentObj != nil {
					parentStatus, _ := parentObj[objects.FieldKeyStatus].(string)
					if checker.IsTerminal(objects.KindPriorityPlan, parentStatus) {
						parentIsTerminal = true
					}
				}
			}

			if status == objects.ObjectStatusInProgress {
				// Only recover/archive in_progress tasks if:
				// 1. Parent plan is terminal, OR
				// 2. Task is stale (> staleThreshold), OR
				// 3. Explicitly targeted with planID or staleFlag
				isStale := false
				updatedAtStr, _ := obj[objects.FieldKeyUpdatedAt].(string)
				if updatedAtStr == "" {
					updatedAtStr, _ = obj[objects.FieldKeyClaimedAt].(string)
				}
				if updatedAtStr != "" {
					if t, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
						if now.Sub(t) >= staleThreshold {
							isStale = true
						}
					}
				} else {
					isStale = true
				}

				if !parentIsTerminal && !isStale && !staleFlag {
					// Task is actively in progress within normal window, don't interrupt
					continue
				}
			}

			if parentIsTerminal {
				// Parent plan is already complete or archived: archive the task
				obj[objects.FieldKeyStatus] = objects.ObjectStatusArchived
				obj[objects.FieldKeyClaimedBy] = storage.FieldUnset
				obj[objects.FieldKeyClaimedAt] = storage.FieldUnset

				updateCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.NewSystemContext(), "agent recover archive orphaned task")
				if err := sp.Update(updateCtx, secCtx, id, obj); err != nil {
					return errfmt.Newf("failed to archive agent task %s", id).Wrap(err)
				}
				archivedCount++
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("✓ Archived orphaned task (parent plan terminal): %s (%s)\n", title, id)))
				continue
			}

			var feedbackHistory []string

			// Extract feedback from steps
			stepsAny, ok := obj[objects.FieldKeyTaskSteps]
			if ok {
				if steps, ok := stepsAny.([]any); ok {
					for _, stepAny := range steps {
						if stepMap, ok := stepAny.(map[string]any); ok {
							stepStatus, _ := stepMap[objects.FieldKeyStatus].(string)
							if stepStatus == objects.ObjectStatusFailed || stepStatus == objects.ObjectStatusRejected || stepStatus == objects.ObjectStatusError {
								feedback, _ := stepMap[objects.FieldKeyVerificationFeedback].(string)
								if feedback != "" {
									stepTitle, _ := stepMap[objects.FieldKeyTitle].(string)
									feedbackHistory = append(feedbackHistory, fmt.Sprintf("Step '%s' failed with: %s", stepTitle, feedback))
								}
								// Reset step status to pending_verification so it can be retried
								stepMap[objects.FieldKeyStatus] = objects.ObjectStatusPendingVerification
								stepMap[objects.FieldKeyVerificationFeedback] = ""
							}
						}
					}
				}
			}

			// Inject feedback into description
			if len(feedbackHistory) > 0 {
				desc, _ := obj[objects.FieldKeyDescription].(string)
				desc += "\n\n## Verification Feedback (Previous Failure)\n"
				for _, f := range feedbackHistory {
					desc += fmt.Sprintf("- %s\n", f)
				}
				obj[objects.FieldKeyDescription] = desc
			}

			// Transition status to approved and release claim
			obj[objects.FieldKeyStatus] = objects.ObjectStatusApproved
			obj[objects.FieldKeyClaimedBy] = storage.FieldUnset
			obj[objects.FieldKeyClaimedAt] = storage.FieldUnset

			updateCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.NewSystemContext(), "agent recover force")
			err := sp.Update(updateCtx, secCtx, id, obj)
			if err != nil {
				return errfmt.Newf("failed to update agent task %s", id).Wrap(err)
			}

			recoveredCount++
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("✓ Recovered task: %s (%s)\n", title, id)))
		}

		if planID != "" {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("\nRecovered %d task(s), archived %d task(s) for plan %s\n", recoveredCount, archivedCount, planID)))
		} else {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("\nRecovered %d task(s), archived %d orphaned task(s)\n", recoveredCount, archivedCount)))
		}

		return nil
	})(cmd, args)
}
