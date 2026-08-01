package object

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

// NewPromoteCmd creates a new promote command
func NewPromoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectPromoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runPromote)
	return cmd
}

func runPromote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()

		// Initialize loaders and validator for dry-run checks
		specsDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalDir, "object_specs")
		specLoader := objects.NewSpecLoader(specsDir)

		// Set builder registry on spec loader to enable version-aware loading
		builderRegistry := builders.GetGlobalRegistry()
		adapter := builders.NewSpecLoaderAdapter(builderRegistry)
		specLoader.SetBuilderRegistry(adapter)

		if err := specLoader.EnsureReady(ctx); err != nil {
			return fmt.Errorf("failed to initialize spec loader: %v", err)
		}

		lifecyclesDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalDir, "lifecycles")
		lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)

		gv := validation.NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

		var errors []string
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

			// Find furthest valid status. Keep rejection reasons so a stuck
			// promote reports *why* candidates failed (not only "already highest").
			bestStatus := currentStatus
			rejectionByStatus := make(map[string]string)
			var rejectedOrder []string
			for i := len(statuses) - 1; i > currentIdx; i-- {
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
				msg := formatStuckLifecycleTransition(id, currentStatus, "highest", "promote", rejectedOrder, rejectionByStatus, false)
				fmt.Fprintln(cmd.OutOrStdout(), msg)
				errors = append(errors, msg)
				continue
			}

			// Setup update map with the new status
			updateMap := map[string]any{
				objects.FieldKeyStatus: bestStatus,
			}

			// Persist the promotion
			promoteCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
			err = proc.Storage().Update(promoteCtx, secCtx, id, updateMap)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to apply promotion to '%s': %v", id, bestStatus, err))
				continue
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Promoted %s from '%s' to '%s'\n", color.CyanString(id), color.YellowString(currentStatus), color.GreenString(bestStatus))
		}

		// Write-behind cache flush
		if len(args) > 0 {
			flushCtx, cancelFlush := storage.DurabilityFlushContext()
			defer cancelFlush()
			_ = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), nil)
		}

		if len(errors) > 0 {
			return fmt.Errorf("promotion completed with errors:\n%s", strings.Join(errors, "\n"))
		}

		return nil
	})(cmd, args)
}

// formatStuckLifecycleTransition explains why promote/demote could not move.
// If nearestFirst is false, rejectedOrder is furthest-first (promote probe order) and is reversed for display.
func formatStuckLifecycleTransition(id, currentStatus, extreme, verb string, rejectedOrder []string, rejectionByStatus map[string]string, nearestFirst bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s cannot %s past '%s' (already at %s reachable status)", id, verb, currentStatus, extreme)
	if len(rejectedOrder) == 0 {
		b.WriteString(": no further non-terminal lifecycle statuses to try")
		return b.String()
	}
	b.WriteString(". Rejected candidates (nearest first):")
	if nearestFirst {
		for _, status := range rejectedOrder {
			fmt.Fprintf(&b, "\n  %s: %s", status, rejectionByStatus[status])
		}
		return b.String()
	}
	for i := len(rejectedOrder) - 1; i >= 0; i-- {
		status := rejectedOrder[i]
		fmt.Fprintf(&b, "\n  %s: %s", status, rejectionByStatus[status])
	}
	return b.String()
}

func formatCandidateValidationFailure(valErr error, valResult *validation.ValidationResult) string {
	if valErr != nil {
		return valErr.Error()
	}
	if valResult == nil {
		return "validation returned nil result"
	}
	if len(valResult.Errors) == 0 {
		return "validation failed with no error details"
	}
	return formatValidationErrorList(valResult.Errors)
}

func formatValidationErrorList(errs []validation.ValidationError) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		switch {
		case e.Field != "" && e.Rule != "":
			parts = append(parts, fmt.Sprintf("%s [%s]: %s", e.Field, e.Rule, e.Message))
		case e.Field != "":
			parts = append(parts, fmt.Sprintf("%s: %s", e.Field, e.Message))
		case e.Rule != "":
			parts = append(parts, fmt.Sprintf("[%s]: %s", e.Rule, e.Message))
		default:
			parts = append(parts, e.Message)
		}
	}
	return strings.Join(parts, "; ")
}

func getPercentComplete(status string, pc objects.PercentCompleteConfig) float64 {
	if pc.DefaultByStatus == nil {
		return 0
	}
	val, ok := pc.DefaultByStatus[status]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case string:
		s := strings.TrimSpace(strings.TrimSuffix(v, "%"))
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		// "calculated" means progress is derived elsewhere; treat as post-setup / pre-complete
		// so promote ordering does not collapse it to 0 (before grooming/prioritizing).
		if strings.EqualFold(s, "calculated") {
			return 75
		}
	}
	return 0
}
