package object

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// NewDemoteCmd creates a new demote command
func NewDemoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectDemoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runDemote)
	return cmd
}

func runDemote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		tc, err := setupTransitionContext(cmd, proc, args)
		if err != nil {
			return err
		}

		ctx := tc.ctx
		secCtx := tc.secCtx
		lifecycleLoader := tc.env.lifecycleLoader
		gv := tc.env.validator
		flushTracker := tc.flushTracker

		var errors []string
		for _, idArg := range tc.args {
			target, err := resolveAndLoadLifecycleTarget(ctx, secCtx, proc, lifecycleLoader, idArg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", idArg, err))
				continue
			}

			id := target.id
			current := target.current
			kind := target.kind
			currentStatus := target.currentStatus
			lifecycle := target.lifecycle
			statuses := target.statuses
			currentIdx := target.currentIdx

			if currentIdx == -1 {
				errors = append(errors, fmt.Sprintf("%s (%s): current status '%s' is not defined in the lifecycle", id, kind, currentStatus))
				continue
			}

			// Find furthest valid status backwards. Keep rejection reasons.
			bestStatus := currentStatus
			rejectionByStatus := make(map[string]string)
			var rejectedOrder []string // nearest-first (probe order)
			for i := currentIdx - 1; i >= 0; i-- {
				candidate := statuses[i]

				if isNonProgressLifecycleProbeCandidate(candidate, lifecycleStatusByValue(lifecycle, candidate)) {
					continue
				}

				// Create a copy of the object and set the candidate status
				candidateObj := make(map[string]any)
				for k, v := range current {
					candidateObj[k] = v
				}
				candidateObj[objects.FieldKeyStatus] = candidate

				// Set up validation options
				valOptions := &validation.ValidationOptions{
					CurrentState:          currentStatus,
					ValidateLifecycle:     true,
					ValidateSemanticTypes: true,
				}
				storage.BindValidationLookups(valOptions, ctx, proc.Storage(), secCtx)

				// Validate in-memory
				valResult, valErr := gv.Validate(ctx, candidateObj, kind, valOptions)
				if valErr == nil && valResult != nil && valResult.IsValid {
					// We must also check blocking errors per validation tier
					blockingConfig := storage.GetGlobalBlockingCheckConfig()
					blockingErrors := blockingConfig.GetBlockingValidationErrors(valResult.Errors, kind, "")
					if len(blockingErrors) == 0 {
						bestStatus = candidate
						break
					}
					rejectionByStatus[candidate] = formatValidationErrorList(blockingErrors)
					rejectedOrder = append(rejectedOrder, candidate)
					continue
				}
				rejectionByStatus[candidate] = formatCandidateValidationFailure(valErr, valResult)
				rejectedOrder = append(rejectedOrder, candidate)
			}
			if bestStatus == currentStatus {
				msg := formatStuckLifecycleTransition(id, currentStatus, "lowest", "demote", rejectedOrder, rejectionByStatus, true)
				fmt.Fprintln(cmd.OutOrStdout(), msg)
				errors = append(errors, msg)
				continue
			}

			// Persist the demotion
			demoteCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
			err = proc.Storage().Update(demoteCtx, secCtx, id, map[string]any{
				objects.FieldKeyStatus: bestStatus,
			})
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to apply demotion to '%s': %v", id, bestStatus, err))
				continue
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Demoted %s from '%s' to '%s'\n", color.CyanString(id), color.YellowString(currentStatus), color.GreenString(bestStatus))
			flushTracker.addWithBacklogCascade(kind)
		}

		flushObjectMutationVisibility(proc, "demote", flushTracker.kinds())

		if len(errors) > 0 {
			return fmt.Errorf("demotion completed with errors:\n%s", strings.Join(errors, "\n"))
		}

		return nil
	})(cmd, args)
}
