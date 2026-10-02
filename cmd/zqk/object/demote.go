package object

import (
	"fmt"

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
		return executeLifecycleTransitions(cmd, proc, args, "demote", "demotion", demoteTarget)
	})(cmd, args)
}

func demoteTarget(cmd *cobra.Command, proc *cli.Processor, tc *transitionContext, target *loadedLifecycleTarget) error {
	if target.currentIdx == -1 {
		return fmt.Errorf("%s (%s): current status '%s' is not defined in the lifecycle", target.id, target.kind, target.currentStatus)
	}

	probe := newCandidateProbeState(target.currentStatus)
	for i := target.currentIdx - 1; i >= 0; i-- {
		candidate := target.statuses[i]

		if isNonProgressLifecycleProbeCandidate(candidate, lifecycleStatusByValue(target.lifecycle, candidate)) {
			continue
		}

		// Create a copy of the object and set the candidate status
		candidateObj := make(map[string]any)
		for k, v := range target.current {
			candidateObj[k] = v
		}
		candidateObj[objects.FieldKeyStatus] = candidate

		// Set up validation options
		valOptions := &validation.ValidationOptions{
			CurrentState:          target.currentStatus,
			ValidateLifecycle:     true,
			ValidateSemanticTypes: true,
		}
		storage.BindValidationLookups(valOptions, tc.ctx, proc.Storage(), tc.secCtx)

		// Validate in-memory
		valResult, valErr := tc.env.validator.Validate(tc.ctx, candidateObj, target.kind, valOptions)
		if valErr == nil && valResult != nil && valResult.IsValid {
			// We must also check blocking errors per validation tier
			blockingConfig := storage.GetGlobalBlockingCheckConfig()
			blockingErrors := blockingConfig.GetBlockingValidationErrors(valResult.Errors, target.kind, "")
			if len(blockingErrors) == 0 {
				probe.bestStatus = candidate
				break
			}
			probe.recordRejection(candidate, formatValidationErrorList(blockingErrors))
			continue
		}
		probe.recordRejection(candidate, formatCandidateValidationFailure(valErr, valResult))
	}
	if probe.bestStatus == target.currentStatus {
		msg := formatStuckLifecycleTransition(target.id, target.currentStatus, "lowest", "demote", probe.rejectedOrder, probe.rejectionByStatus, true)
		fmt.Fprintln(cmd.OutOrStdout(), msg)
		return fmt.Errorf("%s", msg)
	}

	// Persist the demotion
	demoteCtx := pkgctx.WithCacheUpdate(tc.ctx, target.id, target.kind, "")
	err := proc.Storage().Update(demoteCtx, tc.secCtx, target.id, map[string]any{
		objects.FieldKeyStatus: probe.bestStatus,
	})
	if err != nil {
		return fmt.Errorf("%s: failed to apply demotion to '%s': %v", target.id, probe.bestStatus, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Demoted %s from '%s' to '%s'\n", color.CyanString(target.id), color.YellowString(target.currentStatus), color.GreenString(probe.bestStatus))
	tc.flushTracker.addWithBacklogCascade(target.kind)
	return nil
}
