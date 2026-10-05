package object

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	zqklifecycle "github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/validation/qa"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// NewPromoteCmd creates a new promote command
func NewPromoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectPromoteCommandBuilder()
	cmd.Flags().String("to", "", "Target lifecycle status to advance through valid intermediate hops")
	cli.BindAsyncProgress(cmd, runPromote)
	return cmd
}

func runPromote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		return promoteObjectIDs(cmd, proc, args)
	})(cmd, args)
}

// promoteObjectIDs advances each id one lifecycle hop when preconditions pass.
// When --to <status> is specified, it advances across valid intermediate hops until reaching target status.
// Shared by `object promote` and `object draft promote`.
func promoteObjectIDs(cmd *cobra.Command, proc *cli.Processor, args []string) error {
	toStatus, _ := cmd.Flags().GetString("to")
	toStatus = strings.TrimSpace(strings.ToLower(toStatus))
	if toStatus == "" {
		return executeLifecycleTransitions(cmd, proc, args, "promote", "promotion", promoteTarget)
	}

	const maxHops = 8
	for hop := 0; hop < maxHops; hop++ {
		allReached := true
		err := executeLifecycleTransitions(cmd, proc, args, "promote", "promotion", func(cmd *cobra.Command, proc *cli.Processor, tc *transitionContext, target *loadedLifecycleTarget) error {
			if strings.EqualFold(target.currentStatus, toStatus) {
				return nil
			}
			allReached = false
			return promoteTarget(cmd, proc, tc, target)
		})
		if err != nil {
			return err
		}
		if allReached {
			return nil
		}
	}
	return nil
}

func promoteTarget(cmd *cobra.Command, proc *cli.Processor, tc *transitionContext, target *loadedLifecycleTarget) error {
	ctx := tc.ctx
	secCtx := tc.secCtx
	lifecycleLoader := tc.env.lifecycleLoader
	gv := tc.env.validator
	flushTracker := tc.flushTracker

	id := target.id
	current := target.current
	kind := target.kind
	currentStatus := target.currentStatus
	lifecycle := target.lifecycle
	statuses := target.statuses
	sortedStatuses := target.sortedStatuses
	currentIdx := target.currentIdx

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
			koi.SetStatus(candidateObj, candidate)
			if _, ok := candidateObj[objects.FieldKeyCreatedAt]; !ok || candidateObj[objects.FieldKeyCreatedAt] == nil || candidateObj[objects.FieldKeyCreatedAt] == "" {
				now := zqktime.NowRFC3339UTC()
				actor := objects.DefaultSystemAccountID
				if secCtx != nil && secCtx.AccountID != "" {
					actor = pkgctx.ActorIDForAttribution(secCtx.AccountID)
				}
				candidateObj[objects.FieldKeyCreatedAt] = now
				if _, ok := candidateObj[objects.FieldKeyCreatedBy]; !ok || candidateObj[objects.FieldKeyCreatedBy] == nil || candidateObj[objects.FieldKeyCreatedBy] == "" {
					candidateObj[objects.FieldKeyCreatedBy] = actor
				}
				if _, ok := candidateObj[objects.FieldKeyUpdatedAt]; !ok || candidateObj[objects.FieldKeyUpdatedAt] == nil || candidateObj[objects.FieldKeyUpdatedAt] == "" {
					candidateObj[objects.FieldKeyUpdatedAt] = now
				}
				if _, ok := candidateObj[objects.FieldKeyUpdatedBy]; !ok || candidateObj[objects.FieldKeyUpdatedBy] == nil || candidateObj[objects.FieldKeyUpdatedBy] == "" {
					candidateObj[objects.FieldKeyUpdatedBy] = actor
				}
			}
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
				return koi.Status(obj), nil
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
			return fmt.Errorf("%s (%s): current status '%s' is not defined in the lifecycle; recovery to initial failed (%s)",
				id, kind, currentStatus, strings.Join(recoverRejects, "; "))
		}
		updateMap := map[string]any{objects.FieldKeyStatus: recovered}
		// Skip transition validation: source status is not in the lifecycle graph.
		promoteCtx := pkgctx.WithLifecycleBreakGlass(
			pkgctx.WithCacheUpdate(ctx, id, kind, ""),
			"promote recover undefined status to lifecycle initial",
		)
		promoteCtx = storage.WithCLIOperation(storage.WithSkipWriteBehind(promoteCtx))
		if err := proc.Storage().Update(promoteCtx, secCtx, id, updateMap); err != nil {
			return fmt.Errorf("%s: failed to recover status '%s' → '%s': %v", id, currentStatus, recovered, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Recovered %s from undefined '%s' to initial '%s'\n",
			color.CyanString(id), color.YellowString(currentStatus), color.GreenString(recovered))
		flushTracker.add(kind)
		return nil
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
	probe := newCandidateProbeState(currentStatus)
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
		koi.SetStatus(candidateObj, candidate)
		// Apply YAML side_effects.clear before composed_integrity (e.g. drop
		// active_order on priority_plan → complete). Probe used to copy status
		// only, so promote active→complete failed while active_order was set.
		for _, field := range objects.TransitionClearFields(lifecycle, currentStatus, candidate) {
			delete(candidateObj, field)
		}
		// If candidate is crossing out of draft plane (or current lacked membrane timestamps),
		// supply provisional timestamps and actor identity so validation does not reject
		// an object for fields that will be stamped upon crossing the CAS membrane.
		if _, ok := candidateObj[objects.FieldKeyCreatedAt]; !ok || candidateObj[objects.FieldKeyCreatedAt] == nil || candidateObj[objects.FieldKeyCreatedAt] == "" {
			now := zqktime.NowRFC3339UTC()
			actor := objects.DefaultSystemAccountID
			if secCtx != nil && secCtx.AccountID != "" {
				actor = pkgctx.ActorIDForAttribution(secCtx.AccountID)
			}
			candidateObj[objects.FieldKeyCreatedAt] = now
			if _, ok := candidateObj[objects.FieldKeyCreatedBy]; !ok || candidateObj[objects.FieldKeyCreatedBy] == nil || candidateObj[objects.FieldKeyCreatedBy] == "" {
				candidateObj[objects.FieldKeyCreatedBy] = actor
			}
			if _, ok := candidateObj[objects.FieldKeyUpdatedAt]; !ok || candidateObj[objects.FieldKeyUpdatedAt] == nil || candidateObj[objects.FieldKeyUpdatedAt] == "" {
				candidateObj[objects.FieldKeyUpdatedAt] = now
			}
			if _, ok := candidateObj[objects.FieldKeyUpdatedBy]; !ok || candidateObj[objects.FieldKeyUpdatedBy] == nil || candidateObj[objects.FieldKeyUpdatedBy] == "" {
				candidateObj[objects.FieldKeyUpdatedBy] = actor
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
			return koi.Status(obj), nil
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
				probe.bestStatus = candidate
				break
			}
			probe.recordRejection(candidate, formatValidationErrorList(blockingErrors))
			continue
		}
		probe.recordRejection(candidate, formatCandidateValidationFailure(valErr, valResult))
	}

	if probe.bestStatus == currentStatus {
		msg := formatStuckPromote(id, diag, probe.rejectedOrder, probe.rejectionByStatus)
		return fmt.Errorf("%s", msg)
	}
	bestStatus := probe.bestStatus

	// Complete hop invokes AuditorGate.VerifyComplete for execution work units
	if (kind == objects.KindBacklogItem || kind == objects.KindAgentTask) && objects.GetGlobalStatusChecker().IsWorkDone(kind, bestStatus) {
		gate := qa.NewAuditorGateForProject(proc.Storage(), proc.ProjectRoot())
		if err := gate.VerifyComplete(ctx, id); err != nil {
			return fmt.Errorf("%s: qa_success verification failed for complete hop: %v", id, err)
		}
	}

	finalStatus := bestStatus
	promoteCtx := pkgctx.WithCacheUpdate(ctx, id, kind, "")
	promoteCtx = storage.WithCLIOperation(storage.WithSkipWriteBehind(promoteCtx))

	// Archive is a promote hop that owns the prune/cluster membrane burrito
	// (children ride the parent; any member failure fails the hop).
	// TRACK: retention plan-promote archive; stage_membrane prune fail-closed.
	if bestStatus == objects.ObjectStatusArchived {
		dependents := func(seed string) []string {
			return storage.DependentsForID(promoteCtx, proc.Storage(), seed)
		}
		hop, hopErr := zqklifecycle.PlanStageMembraneHop(promoteCtx, secCtx, proc.Storage(), lifecycleLoader, dependents, []string{id}, bestStatus)
		if hopErr != nil {
			return fmt.Errorf("%s: failed to plan archive promote: %v", id, hopErr)
		}
		if err := zqklifecycle.ApplyStageMembraneHop(promoteCtx, secCtx, proc.Storage(), dependents, hop); err != nil {
			return fmt.Errorf("%s: failed to apply archive promote burrito: %v", id, err)
		}
		for _, m := range hop.Members {
			flushTracker.add(m.Kind)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Promoted %s from '%s' to '%s' (membrane %d members)\n",
			color.CyanString(id), color.YellowString(currentStatus), color.GreenString(finalStatus), len(hop.AppliedMembers()))
		if hook, ok := promoteCueHooks[kind]; ok {
			if cue := hook(ctx, proc.Storage(), secCtx, id, current, finalStatus); cue != "" {
				fmt.Fprintln(cmd.OutOrStdout(), cue)
			}
		}
		return nil
	}

	// Setup update map with the new status plus lifecycle clear side_effects.
	updateMap := map[string]any{
		objects.FieldKeyStatus: bestStatus,
	}
	for _, field := range objects.TransitionClearFields(lifecycle, currentStatus, bestStatus) {
		updateMap[field] = storage.FieldUnset
	}

	// Persist the promotion
	err := proc.Storage().Update(promoteCtx, secCtx, id, updateMap)
	if err != nil {
		return fmt.Errorf("%s: failed to apply promotion to '%s': %v", id, bestStatus, err)
	}

	// Read back final object status in case lifecycle hooks (e.g. execution lock) advanced it.
	if updatedObj, err := proc.Storage().Read(promoteCtx, secCtx, id); err == nil && updatedObj != nil {
		if st := koi.Status(updatedObj); st != "" {
			finalStatus = st
		}
	}
	// Sweep / unlink any preliminary draft file now that the object is promoted across the CAS membrane.
	if isPrelim, _ := objects.GetGlobalLifecycleLoader().IsPreliminaryStatusForKind(kind, finalStatus); !isPrelim {
		_ = storage.DeleteObjectDraftFile(proc.ProjectRoot(), kind, id)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Promoted %s from '%s' to '%s'\n", color.CyanString(id), color.YellowString(currentStatus), color.GreenString(finalStatus))
	if hook, ok := promoteCueHooks[kind]; ok {
		if cue := hook(ctx, proc.Storage(), secCtx, id, current, finalStatus); cue != "" {
			fmt.Fprintln(cmd.OutOrStdout(), cue)
		}
	}
	flushTracker.addWithBacklogCascade(kind)
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
		fmt.Fprintf(&b, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("\n  Note: '%s' is terminal completion. To archive, use: zqk object promote %s (targets archived; children ride the prune shockwave)", diag.CurrentStatus, id)))
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
	b.WriteString(paths.RewriteCanonicalCLIInvocations("\n  Tip: satisfy the validation/preconditions above, then re-run: zqk object promote ") + id)
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

// priorityPlanPromotePackagingCue surfaces PRI≈PR packaging when promoting a wrap-ready plan.
func priorityPlanPromotePackagingCue(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, planID string, current map[string]any, newStatus string) string {
	title := koi.Title(current)
	counts := map[string]int{}
	listRes, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: planID},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyPriorityPlanRef, objects.FieldKeyStatus},
	})
	if err == nil {
		for _, o := range listRes.Objects {
			if koi.GetString(o, objects.FieldKeyPriorityPlanRef) != planID {
				continue
			}
			st := koi.Status(o)
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
