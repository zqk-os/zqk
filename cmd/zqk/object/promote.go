package object

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	zqklifecycle "github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/validation/qa"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// NewPromoteCmd creates a new promote command
func NewPromoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectPromoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runPromote)
	return cmd
}

func runPromote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		return promoteObjectIDs(cmd, proc, args)
	})(cmd, args)
}

// promoteObjectIDs advances each id one lifecycle hop when preconditions pass.
// Shared by `object promote` and `object draft promote`.
func promoteObjectIDs(cmd *cobra.Command, proc *cli.Processor, args []string) error {
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
	// flushKinds must be non-empty or CAS index
	// queue is never flushed (nil skipped the listing-index write; next CLI process → object-not-found).
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
		process.TouchMeaningfulActivity()
		if ctx.Err() != nil {
			errors = append(errors, fmt.Sprintf("%s: skipped due to context timeout: %v", idArg, ctx.Err()))
			break
		}
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
				percent: objects.LifecycleProgressPercent(st.Value, lifecycle.PercentComplete),
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
			// Kernel repair: illegal/undefined status (e.g. legacy "proposed") cannot walk
			// the graph. Recover onto an initial lifecycle status when validation allows.
			var recoverOrder []string
			for _, st := range lifecycle.Statuses {
				if st.Origin || st.Preliminary {
					recoverOrder = append(recoverOrder, st.Value)
				}
			}
			if len(recoverOrder) == 0 && len(sortedStatuses) > 0 {
				recoverOrder = append(recoverOrder, sortedStatuses[0].value)
			}
			recovered := ""
			var recoverRejects []string
			for _, candidate := range recoverOrder {
				candidateObj := make(map[string]any)
				for k, v := range current {
					candidateObj[k] = v
				}
				candidateObj[objects.FieldKeyStatus] = candidate
				// Validate as if already at candidate (no edge from the illegal status).
				valOptions := &validation.ValidationOptions{
					CurrentState:          candidate,
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
				valResult, valErr := gv.Validate(ctx, candidateObj, kind, valOptions)
				if valErr == nil && valResult != nil && valResult.IsValid {
					blockingConfig := storage.GetGlobalBlockingCheckConfig()
					blockingErrors := blockingConfig.GetBlockingValidationErrors(valResult.Errors, kind, "")
					if len(blockingErrors) == 0 {
						recovered = candidate
						break
					}
					recoverRejects = append(recoverRejects, fmt.Sprintf("%s: %s", candidate, formatValidationErrorList(blockingErrors)))
					continue
				}
				recoverRejects = append(recoverRejects, fmt.Sprintf("%s: %s", candidate, formatCandidateValidationFailure(valErr, valResult)))
			}
			if recovered == "" {
				errors = append(errors, fmt.Sprintf("%s (%s): current status '%s' is not defined in the lifecycle; recovery to initial failed (%s)",
					id, kind, currentStatus, strings.Join(recoverRejects, "; ")))
				continue
			}
			updateMap := map[string]any{objects.FieldKeyStatus: recovered}
			// Skip transition validation: source status is not in the lifecycle graph.
			promoteCtx := pkgctx.WithLifecycleBreakGlass(
				pkgctx.WithCacheUpdate(ctx, id, kind, ""),
				"promote recover undefined status to lifecycle initial",
			)
			if err = proc.Storage().Update(promoteCtx, secCtx, id, updateMap); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to recover status '%s' → '%s': %v", id, currentStatus, recovered, err))
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Recovered %s from undefined '%s' to initial '%s'\n",
				color.CyanString(id), color.YellowString(currentStatus), color.GreenString(recovered))
			addFlushKind(kind)
			continue
		}

		// Prefer lifecycle transition-graph neighbors (one hop), not "any higher percent".
		// Percent-only probing can jump priority_plan active→complete when mid-lifecycle
		// statuses share "calculated" (75).
		neighborSet := promoteTransitionTargets(lifecycle, currentStatus)
		currentPercent := objects.LifecycleProgressPercent(currentStatus, lifecycle.PercentComplete)
		diag := promoteStuckDiag{
			Kind:                   kind,
			CurrentStatus:          currentStatus,
			CurrentPercent:         currentPercent,
			MissingPercentDefaults: len(lifecycle.PercentComplete.DefaultByStatus) == 0,
		}
		for _, swp := range sortedStatuses {
			if _, ok := neighborSet[swp.value]; ok {
				diag.GraphNeighbors = append(diag.GraphNeighbors, swp.value)
			}
		}
		var probeOrder []string
		if len(neighborSet) > 0 {
			diag.UsedTransitionGraph = true
			for _, swp := range sortedStatuses {
				if _, ok := neighborSet[swp.value]; !ok {
					continue
				}
				archiveHop := promoteAllowsArchiveHop(currentStatus, swp.value)
				if isNonProgressLifecycleProbeCandidate(swp.value, lifecycleStatusByValue(lifecycle, swp.value)) && !archiveHop {
					diag.SkippedNonProgress = append(diag.SkippedNonProgress, swp.value)
					continue
				}
				// Promote is forward-only: skip lateral/recovery edges (e.g. * → draft)
				// whose percent_complete is not greater than the current status.
				// complete→archived is equal-percent by design; allow that hop explicitly.
				if swp.percent <= currentPercent && !isSuccessLifecycleTerminal(swp.value) && !archiveHop {
					diag.SkippedByPercent = append(diag.SkippedByPercent, fmt.Sprintf(
						"%s (percent_complete %.0f <= current %.0f)", swp.value, swp.percent, currentPercent))
					continue
				}
				probeOrder = append(probeOrder, swp.value)
			}
		} else {
			// No declared edges: fall back to forward percent order (legacy).
			for i := currentIdx + 1; i < len(statuses); i++ {
				cand := statuses[i]
				if isNonProgressLifecycleProbeCandidate(cand, lifecycleStatusByValue(lifecycle, cand)) &&
					!promoteAllowsArchiveHop(currentStatus, cand) {
					diag.SkippedNonProgress = append(diag.SkippedNonProgress, cand)
					continue
				}
				probeOrder = append(probeOrder, cand)
			}
		}
		// Do not skip planned→in_progress (or any execution hop) to complete just
		// because the nearer hop failed validation (e.g. PRI still grooming).
		probeOrder = promoteForwardProbeOrder(currentStatus, probeOrder)
		diag.ProbeOrder = append([]string(nil), probeOrder...)

		// Find next valid status (one hop). Keep rejection reasons so a stuck
		// promote reports *why* candidates failed.
		bestStatus := currentStatus
		rejectionByStatus := make(map[string]string)
		var rejectedOrder []string
		for _, candidate := range probeOrder {
			if candidate == currentStatus {
				continue
			}
			if isNonProgressLifecycleProbeCandidate(candidate, lifecycleStatusByValue(lifecycle, candidate)) &&
				!promoteAllowsArchiveHop(currentStatus, candidate) {
				diag.SkippedNonProgress = append(diag.SkippedNonProgress, candidate)
				continue
			}

			// Create a copy of the object and set the candidate status
			candidateObj := make(map[string]any)
			for k, v := range current {
				candidateObj[k] = v
			}
			candidateObj[objects.FieldKeyStatus] = candidate
			// Apply YAML side_effects.clear before composed_integrity (e.g. drop
			// active_order on priority_plan → complete). Probe used to copy status
			// only, so promote active→complete failed while active_order was set.
			for _, field := range objects.TransitionClearFields(lifecycle, currentStatus, candidate) {
				delete(candidateObj, field)
			}

			// CRI-PERSONA-SKILL-BOUND: refuse shovel-ready+ promote without resolving ASK links.
			if kind == objects.KindPersona && personaPromoteRequiresSkillBound(candidate) {
				resolve := func(askID string) (map[string]any, error) {
					return proc.Storage().Read(ctx, secCtx, askID)
				}
				if res := validation.EvaluatePersonaSkillBound(candidateObj, resolve); !res.Bound {
					rejectionByStatus[candidate] = fmt.Sprintf("%s: missing/unresolved agent_skill link (%v)",
						validation.CriteriaIDPersonaSkillBound, res.Missing)
					rejectedOrder = append(rejectedOrder, candidate)
					continue
				}
			}

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
			msg := formatStuckPromote(id, diag, rejectedOrder, rejectionByStatus)
			errors = append(errors, msg)
			continue
		}

		// CRIT-COMPLETE-HOP-VERIFYCOMPLETE-001: complete hop invokes AuditorGate.VerifyComplete for execution work units
		if (kind == objects.KindBacklogItem || kind == objects.KindAgentTask) && objects.GetGlobalStatusChecker().IsWorkDone(kind, bestStatus) {
			gate := qa.NewAuditorGateForProject(proc.Storage(), proc.ProjectRoot())
			if err := gate.VerifyComplete(ctx, id); err != nil {
				errors = append(errors, fmt.Sprintf("%s: qa_success verification failed for complete hop: %v", id, err))
				continue
			}
		}

		finalStatus := bestStatus
		promoteCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")

		// Archive is a promote hop that owns the prune/cluster membrane burrito
		// (children ride the parent; any member failure fails the hop).
		// TRACK: retention plan-promote archive; stage_membrane prune fail-closed.
		if bestStatus == objects.ObjectStatusArchived {
			dependents := func(seed string) []string {
				return storage.DependentsForID(promoteCtx, proc.Storage(), seed)
			}
			hop, hopErr := zqklifecycle.PlanStageMembraneHop(promoteCtx, secCtx, proc.Storage(), lifecycleLoader, dependents, []string{id}, bestStatus)
			if hopErr != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to plan archive promote: %v", id, hopErr))
				continue
			}
			if err := zqklifecycle.ApplyStageMembraneHop(promoteCtx, secCtx, proc.Storage(), dependents, hop); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to apply archive promote burrito: %v", id, err))
				continue
			}
			for _, m := range hop.Members {
				addFlushKind(m.Kind)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Promoted %s from '%s' to '%s' (membrane %d members)\n",
				color.CyanString(id), color.YellowString(currentStatus), color.GreenString(finalStatus), len(hop.AppliedMembers()))
			if hook, ok := promoteCueHooks[kind]; ok {
				if cue := hook(ctx, proc.Storage(), secCtx, id, current, finalStatus); cue != "" {
					fmt.Fprintln(cmd.OutOrStdout(), cue)
				}
			}
			continue
		}

		// Setup update map with the new status plus lifecycle clear side_effects.
		updateMap := map[string]any{
			objects.FieldKeyStatus: bestStatus,
		}
		for _, field := range objects.TransitionClearFields(lifecycle, currentStatus, bestStatus) {
			updateMap[field] = storage.FieldUnset
		}

		// Persist the promotion
		err = proc.Storage().Update(promoteCtx, secCtx, id, updateMap)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to apply promotion to '%s': %v", id, bestStatus, err))
			continue
		}

		// Read back final object status in case lifecycle hooks (e.g. execution lock) advanced it.
		if updatedObj, err := proc.Storage().Read(promoteCtx, secCtx, id); err == nil && updatedObj != nil {
			if st, ok := updatedObj[objects.FieldKeyStatus].(string); ok && st != "" {
				finalStatus = st
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Promoted %s from '%s' to '%s'\n", color.CyanString(id), color.YellowString(currentStatus), color.GreenString(finalStatus))
		if hook, ok := promoteCueHooks[kind]; ok {
			if cue := hook(ctx, proc.Storage(), secCtx, id, current, finalStatus); cue != "" {
				fmt.Fprintln(cmd.OutOrStdout(), cue)
			}
		}
		addFlushKind(kind)
		// BLI status changes can shockwave priority_plan in-process; flush that kind too.
		if kind == objects.KindBacklogItem {
			addFlushKind(objects.KindPriorityPlan)
		}
	}

	// Write-behind drain + CAS listing-index flush for every kind we mutated.
	if len(affectedKinds) > 0 {
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), affectedKinds); err != nil {
			// Status write already succeeded; flush/visibility is best-effort across processes.
			// Hard-failing here turned successful promotes into errors on Darwin CAS visibility waits.
			logging.FluentEvent(proc.Logger()).Warn("promote: durability flush after status write").
				WithError(err).
				String("kinds", strings.Join(affectedKinds, ",")).
				Log()
		}
		logSlowCLIObjectMutationFlush(proc.Logger(), "promote", "", affectedKinds, time.Since(t0), 0)
		proc.TriggerCacheFreshnessCheck("promote", affectedKinds)
	}

	if len(errors) > 0 {
		return fmt.Errorf("promotion completed with errors:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// promoteStuckDiag captures why promote built an empty or failing probe list.
type promoteStuckDiag struct {
	Kind                   string
	CurrentStatus          string
	CurrentPercent         float64
	MissingPercentDefaults bool
	UsedTransitionGraph    bool
	GraphNeighbors         []string
	SkippedByPercent       []string
	SkippedNonProgress     []string
	ProbeOrder             []string
}

// formatStuckPromote explains why promote could not move, including graph/percent filters.
func formatStuckPromote(id string, diag promoteStuckDiag, rejectedOrder []string, rejectionByStatus map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s cannot promote past '%s'", id, diag.CurrentStatus)
	if diag.Kind != "" {
		fmt.Fprintf(&b, " (kind %s)", diag.Kind)
	}
	fmt.Fprintf(&b, " — current percent_complete=%.0f", diag.CurrentPercent)

	if diag.UsedTransitionGraph {
		if len(diag.GraphNeighbors) == 0 {
			b.WriteString("\n  Lifecycle: no one-hop promote edges from this status (manual/auto edges; auto-only excluded).")
		} else {
			fmt.Fprintf(&b, "\n  Lifecycle one-hop edges from '%s': %s", diag.CurrentStatus, strings.Join(diag.GraphNeighbors, ", "))
		}
	} else {
		b.WriteString("\n  Lifecycle: no graph neighbors; using forward percent-order fallback.")
	}
	if len(diag.SkippedByPercent) > 0 {
		b.WriteString("\n  Skipped (not strictly forward by percent_complete):")
		for _, line := range diag.SkippedByPercent {
			fmt.Fprintf(&b, "\n    - %s", line)
		}
		if diag.MissingPercentDefaults {
			b.WriteString("\n  Cause: lifecycle percent_complete.default_by_status is empty/missing, so every status scores 0.")
			fmt.Fprintf(&b, "\n  Fix: set increasing defaults in .zqk/specs/lifecycles/%s_lifecycle.yaml (see doc_entry for pattern).", diag.Kind)
		} else if diag.CurrentPercent < 100 {
			b.WriteString("\n  Fix: raise percent_complete.default_by_status for the next status above the current value, or demote if you need a lateral/recovery move.")
		}
	}
	if len(diag.SkippedNonProgress) > 0 {
		fmt.Fprintf(&b, "\n  Skipped (archive/system/non-progress statuses): %s", strings.Join(diag.SkippedNonProgress, ", "))
	}
	if len(diag.ProbeOrder) > 0 {
		fmt.Fprintf(&b, "\n  Probed candidates: %s", strings.Join(diag.ProbeOrder, ", "))
	} else if len(diag.SkippedByPercent) == 0 && len(diag.GraphNeighbors) == 0 {
		b.WriteString("\n  No further lifecycle statuses to try from here.")
	}

	// complete → archived is a promote hop (membrane burrito). If archive was
	// skipped or rejected, tip promote — not park.
	hasArchiveNeighbor := false
	for _, neighbor := range diag.GraphNeighbors {
		if neighbor == objects.ObjectStatusArchived {
			hasArchiveNeighbor = true
			break
		}
	}
	for _, skipped := range diag.SkippedNonProgress {
		if skipped == objects.ObjectStatusArchived {
			hasArchiveNeighbor = true
			break
		}
	}
	if (diag.CurrentPercent >= 100 || isSuccessLifecycleTerminal(diag.CurrentStatus)) && hasArchiveNeighbor {
		fmt.Fprintf(&b, "\n  Note: '%s' is terminal completion. To archive, use: zqk object promote %s (targets archived; children ride the prune shockwave)", diag.CurrentStatus, id)
	}

	if len(rejectedOrder) == 0 {
		if len(diag.ProbeOrder) > 0 && len(diag.SkippedNonProgress) == len(diag.ProbeOrder) {
			b.WriteString("\n  All probe candidates were non-progress (error/deferred/…); use object park --to deferred|roadmap for lateral exits, or demote if that is intended.")
		}
		return b.String()
	}
	b.WriteString("\n  Rejected candidates (nearest first):")
	for i := len(rejectedOrder) - 1; i >= 0; i-- {
		status := rejectedOrder[i]
		fmt.Fprintf(&b, "\n    - %s: %s", status, rejectionByStatus[status])
	}
	b.WriteString("\n  Tip: satisfy the validation/preconditions above, then re-run: zqk object promote " + id)
	return b.String()
}

// formatStuckLifecycleTransition explains why demote could not move (nearest-first rejections).
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

// promoteForwardProbeOrder drops success-terminal candidates when a nearer
// **execution** hop exists. Otherwise `zqk object promote` on a planned BLI
// falls through to complete after in_progress fails (plan still grooming).
// Keep complete after a seal hop (grooming→active) so a column with no planned
// children can take the grooming→complete escape instead of failing closed.
// When the only remaining hop is complete (already in_progress), keep it.
func promoteForwardProbeOrder(currentStatus string, probeOrder []string) []string {
	if len(probeOrder) < 2 {
		return probeOrder
	}
	// From testing or in_progress, complete is a valid primary target (not an unearned bypass).
	if strings.EqualFold(strings.TrimSpace(currentStatus), "testing") || strings.EqualFold(strings.TrimSpace(currentStatus), objects.ObjectStatusInProgress) {
		return probeOrder
	}
	work := make([]string, 0, len(probeOrder))
	terminals := make([]string, 0, len(probeOrder))
	for _, candidate := range probeOrder {
		if isSuccessLifecycleTerminal(candidate) {
			terminals = append(terminals, candidate)
			continue
		}
		work = append(work, candidate)
	}
	if len(work) == 0 {
		return probeOrder
	}
	if probeHasExecutionHop(work) {
		return work
	}
	return append(work, terminals...)
}

func probeHasExecutionHop(candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(strings.TrimSpace(candidate), objects.ObjectStatusInProgress) {
			return true
		}
	}
	return false
}

// promoteTransitionTargets returns statuses reachable in one hop from current via lifecycle
// transitions that promote may take (from == current or from == "*").
// Auto-only edges (shockwave / "all children done") are excluded so promote cannot overshoot
// shovel-ready active → complete.
func promoteTransitionTargets(lifecycle *objects.Lifecycle, current string) map[string]struct{} {
	// Shared with pkg/objects contract tests ( ).
	return objects.PromoteTransitionTargets(lifecycle, current)
}

// personaPromoteRequiresSkillBound is true for shovel-ready and later persona statuses.
func personaPromoteRequiresSkillBound(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case objects.ObjectStatusApproved, objects.ObjectStatusInProgress, objects.ObjectStatusImplemented:
		return true
	default:
		return false
	}
}

// priorityPlanPromotePackagingCue surfaces PRI≈PR packaging when promoting a wrap-ready plan.
func priorityPlanPromotePackagingCue(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, planID string, current map[string]any, newStatus string) string {
	title, _ := current[objects.FieldKeyTitle].(string)
	counts := map[string]int{}
	listRes, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{Kind: objects.KindBacklogItem})
	if err == nil {
		for _, o := range listRes.Objects {
			ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
			if ref != planID {
				continue
			}
			st, _ := o[objects.FieldKeyStatus].(string)
			if st == "" {
				st = "unknown"
			}
			counts[st]++
		}
	}
	return whatsnext.PriorityPlanPackagingCue(planID, title, newStatus, counts)
}

// PromoteCueHook produces user-facing CLI cues when an object of a given kind is promoted.
type PromoteCueHook func(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id string, current map[string]any, finalStatus string) string

var promoteCueHooks = map[string]PromoteCueHook{
	objects.KindPriorityPlan: priorityPlanPromotePackagingCue,
}

// RegisterPromoteCueHook allows extension kinds to register post-promote messaging cues.
func RegisterPromoteCueHook(kind string, hook PromoteCueHook) {
	promoteCueHooks[kind] = hook
}
