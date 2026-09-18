package object

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

// NewDemoteCmd creates a new demote command
func NewDemoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectDemoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runDemote)
	return cmd
}

func runDemote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		args = expandObjectIDArgs(cmd, args)
		if len(args) == 0 {
			return fmt.Errorf("at least one object ID is required (positional, comma-separated, and/or --ids)")
		}

		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()

		// Initialize loaders and validator for dry-run checks
		specsDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalObjectSpecsDir)
		specLoader := objects.NewSpecLoader(specsDir)

		// Set builder registry on spec loader to enable version-aware loading
		builderRegistry := builders.GetGlobalRegistry()
		adapter := builders.NewSpecLoaderAdapter(builderRegistry)
		specLoader.SetBuilderRegistry(adapter)

		if err := specLoader.EnsureReady(ctx); err != nil {
			return fmt.Errorf("failed to initialize spec loader: %v", err)
		}

		lifecyclesDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalLifecyclesDir)
		lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)

		gv := validation.NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

		var errors []string
		// TRACK: [REDACTED-ID] — nil flushKinds skipped CAS index flush.
		affectedKinds := make([]string, 0, len(args))
		kindSet := make(map[string]bool, len(args))
		addFlushKind := func(k string) {
			if k == "" || kindSet[k] {
				return
			}
			kindSet[k] = true
			affectedKinds = append(affectedKinds, k)
		}
		for _, idArg := range args {
			// Resolve natural language intents
			id, err := proc.ResolveSemanticArgument(ctx, "", idArg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to resolve ID: %v", idArg, err))
				continue
			}

			// Read current object
			current, err := proc.Storage().Read(ctx, secCtx, id)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to read object: %v", id, err))
				continue
			}

			kind, _ := current[objects.FieldKeyKind].(string)
			currentStatus, _ := current[objects.FieldKeyStatus].(string)

			// Load lifecycle
			lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s (%s): no lifecycle defined: %v", id, kind, err))
				continue
			}

			// Sort statuses by percent complete default values to establish progression order
			type statusWithPercent struct {
				value   string
				percent float64
			}
			var sortedStatuses []statusWithPercent
			for _, st := range lifecycle.Statuses {
				sortedStatuses = append(sortedStatuses, statusWithPercent{
					value:   st.Value,
					percent: getPercentComplete(st.Value, lifecycle.PercentComplete),
				})
			}
			sort.Slice(sortedStatuses, func(i, j int) bool {
				return sortedStatuses[i].percent < sortedStatuses[j].percent
			})

			// Populate statuses slice in sorted order
			var statuses []string
			currentIdx := -1
			for i, swp := range sortedStatuses {
				statuses = append(statuses, swp.value)
				if swp.value == currentStatus {
					currentIdx = i
				}
			}

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
				valOptions.ObjectLookup = func(targetID string) (map[string]any, error) {
					return proc.Storage().Read(ctx, secCtx, targetID)
				}
				valOptions.ObjectStatusLookup = func(targetID string) (string, error) {
					obj, err := valOptions.ObjectLookup(targetID)
					if err != nil {
						return "", err
					}
					status, _ := obj[objects.FieldKeyStatus].(string)
					return status, nil
				}
				valOptions.DependentsLookup = func(targetID string) []string {
					return storage.DependentsForID(ctx, proc.Storage(), targetID)
				}

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
			addFlushKind(kind)
			if kind == objects.KindBacklogItem {
				addFlushKind(objects.KindPriorityPlan)
			}
		}

		if len(affectedKinds) > 0 {
			flushCtx, cancelFlush := storage.DurabilityFlushContext()
			defer cancelFlush()
			t0 := time.Now()
			if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), affectedKinds); err != nil {
				logging.FluentEvent(proc.Logger()).Warn("demote: durability flush after status write").
					WithError(err).
					String("kinds", strings.Join(affectedKinds, ",")).
					Log()
			}
			logSlowCLIObjectMutationFlush(proc.Logger(), "demote", "", affectedKinds, time.Since(t0), 0)
			proc.TriggerCacheFreshnessCheck("demote", affectedKinds)
		}

		if len(errors) > 0 {
			return fmt.Errorf("demotion completed with errors:\n%s", strings.Join(errors, "\n"))
		}

		return nil
	})(cmd, args)
}
