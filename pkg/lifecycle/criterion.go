// Package lifecycle: helpers to emit criterion_satisfied when remaining-open drains (and other criteria).

package lifecycle

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// backlogItemTerminalStatuses was a package var listing complete, archived, and rejected. Only the
// first two are backlog_item statuses; rejected is a status_mapping alias for archived, so it never
// matched anything read back from storage. isBacklogItemTerminalStatus answers the same question
// from the lifecycle role.

// StorageProvider is the same as in updater: returns storage for a project root.
type StorageProviderForCriterion func(projectRoot string) (storage.ObjectStorageProvider, bool)

func resolveStorageProvider(projectRoot string, getStorage StorageProviderForCriterion) (storage.ObjectStorageProvider, bool) {
	if projectRoot == emptyValue || getStorage == nil {
		return nil, false
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return nil, false
	}
	return nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
}

func appendAndSyncCriterionSatisfied(projectRoot, criterion string, scope map[string]string) {
	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return
	}
	_ = AppendCriterionSatisfied(wal, criterion, scope)
	if syncErr := wal.Sync(); syncErr != nil {
		logging.LogSwallowedError(syncErr)
	}
}

func completeObjectAndEmitShockwaves(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, projectRoot, kind, id, oldStatus, reason string, extraUpdates map[string]any) map[string]any {
	trustedCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), reason)
	updates := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusComplete}
	for k, v := range extraUpdates {
		updates[k] = v
	}
	if updateErr := provider.Update(trustedCtx, secCtx, id, updates); updateErr == nil {
		if flushErr := storage.FlushListingIndexForProjectRoot(projectRoot, kind); flushErr != nil {
			logging.LogSwallowedError(flushErr)
		}
		if updated, readErr := provider.Read(ctx, secCtx, id); readErr == nil && updated != nil {
			ApplyDependencyRefEvents(ctx, logging.NewEventLogger(ctx), provider, projectRoot, kind, id, oldStatus, objects.ObjectStatusComplete, updated)
			return updated
		}
	}
	return nil
}

func areAllLinkedCriteriaSatisfied(ctx context.Context, provider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, critIDs []string) bool {
	if len(critIDs) == 0 {
		return false
	}
	for _, cid := range critIDs {
		co, err := provider.Read(ctx, secCtx, cid)
		if err != nil || co == nil {
			return false
		}
		cst, _ := co[objects.FieldKeyStatus].(string)
		if !CriterionStatusMeetsMilestoneGateForMilestone(cst) {
			return false
		}
	}
	return true
}

func forEachObjectContainingCriterion(ctx context.Context, projectRoot, criterionID, kind string, getStorage StorageProviderForCriterion, fn func(id string)) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || criterionID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: kind,
		Filters: map[string]any{
			objects.FieldKeyCriteriaRefs: map[string]any{"$has": criterionID},
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return
	}
	for _, obj := range result.Objects {
		if id, _ := obj[objects.FieldKeyID].(string); id != emptyValue {
			fn(id)
		}
	}
}

// TryEmitRemainingOpenDrained emits criterion_satisfied when remaining_open_count is 0.
// Unset field fail-closes (no member List).
func TryEmitRemainingOpenDrained(ctx context.Context, projectRoot, containerID string, getStorage StorageProviderForCriterion) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || containerID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	container, err := provider.Read(ctx, secCtx, containerID)
	if err != nil || container == nil {
		return
	}
	n, counted := remainingOpenCountFrom(container)
	if !counted || n != 0 {
		return
	}
	EmitTrustedRemainingOpenDrained(projectRoot, containerID)
}

// TryEmitAllCriteriaCompleteForMilestone checks whether every criterion in the milestone's criteria_refs
// is in a status that counts toward milestone completion (validated or complete). If so, appends
// CriterionSatisfied(all_criteria_complete_for_milestone, milestone_id=milestoneID) to the lifecycle WAL.
// No-op if milestone is already terminal, has no criteria_refs, or any linked criterion is not satisfied.
func TryEmitAllCriteriaCompleteForMilestone(ctx context.Context, projectRoot, milestoneID string, getStorage StorageProviderForCriterion) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || milestoneID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	milObj, err := provider.Read(ctx, secCtx, milestoneID)
	if err != nil || milObj == nil {
		return
	}
	kind, _ := milObj[objects.FieldKeyKind].(string)
	if kind != objects.KindMilestone {
		return
	}
	st, _ := milObj[objects.FieldKeyStatus].(string)
	switch st {
	case statusComplete, statusDeferred, statusArchived:
		return
	}
	critAny, ok := milObj[objects.FieldKeyCriteriaRefs]
	if !ok {
		return
	}
	critAny, ok = nildecode.DecodeNonNilPayload[any](critAny)
	if !ok {
		return
	}
	critIDs := StringRefsFromAny(critAny)
	if !areAllLinkedCriteriaSatisfied(ctx, provider, secCtx, critIDs) {
		return
	}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllCriteriaCompleteForMilestone, map[string]string{scopeMilestoneID: milestoneID})
	completeObjectAndEmitShockwaves(ctx, provider, secCtx, projectRoot, objects.KindMilestone, milestoneID, st, "milestone all criteria complete", nil)
}

// TryEmitForMilestonesContainingCriterion lists milestones whose criteria_refs include criterionID,
// then evaluates TryEmitAllCriteriaCompleteForMilestone for each. Call when a criterion transitions
// toward a satisfied state so milestones can auto-complete when their linked criteria are all met.
func TryEmitForMilestonesContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	forEachObjectContainingCriterion(ctx, projectRoot, criterionID, objects.KindMilestone, getStorage, func(mid string) {
		TryEmitAllCriteriaCompleteForMilestone(ctx, projectRoot, mid, getStorage)
	})
}

// TryEmitForBacklogItemsContainingCriterion lists backlog items whose criteria_refs include criterionID,
// then evaluates TryEmitAllAcceptanceCriteriaMetForBacklogItem for each. Call when a criterion transitions
// toward a satisfied state so backlog items can auto-complete when their linked criteria are all met.
func TryEmitForBacklogItemsContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	forEachObjectContainingCriterion(ctx, projectRoot, criterionID, objects.KindBacklogItem, getStorage, func(bliID string) {
		TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, projectRoot, bliID, getStorage)
	})
}

// TryEmitAllAcceptanceCriteriaMetForBacklogItem appends all_acceptance_criteria_met_for_backlog_item when every
// CRIT-* listed in the backlog item's criteria_refs is validated/complete and the item is in_progress.
func TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx context.Context, projectRoot, backlogItemID string, getStorage StorageProviderForCriterion) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || backlogItemID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	bli, err := provider.Read(ctx, secCtx, backlogItemID)
	if err != nil || bli == nil {
		return
	}
	if k, _ := bli[objects.FieldKeyKind].(string); k != objects.KindBacklogItem {
		return
	}
	st, _ := bli[objects.FieldKeyStatus].(string)
	if st != statusInProgress {
		return
	}
	critIDs := StringRefsFromAny(bli[objects.FieldKeyCriteriaRefs])
	if !areAllLinkedCriteriaSatisfied(ctx, provider, secCtx, critIDs) {
		return
	}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllAcceptanceCriteriaMetForBacklogItem, map[string]string{scopeBacklogItemID: backlogItemID})
}

// CriterionStatusMeetsMilestoneGateForMilestone returns true if criteria.status counts as satisfied
// for milestone completion. Lifecycle YAML `satisfied: true` is SSOT (validated, complete).
func CriterionStatusMeetsMilestoneGateForMilestone(status string) bool {
	return objects.GetGlobalStatusChecker().IsSatisfied(objects.KindCriteria, status)
}

func StringRefsFromAny(v any) []string {
	switch t := v.(type) {
	case string:
		if t == emptyValue {
			return nil
		}
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// TryEmitAllBacklogItemsCompleteForMilestone checks whether all backlog items referencing milestoneID
// are in a terminal status. If so, appends CriterionSatisfied(all_backlog_items_complete_for_milestone, milestone_id=milestoneID)
// to the lifecycle WAL.
func TryEmitAllBacklogItemsCompleteForMilestone(ctx context.Context, projectRoot, milestoneID string, getStorage StorageProviderForCriterion) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || milestoneID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	milObj, err := provider.Read(ctx, secCtx, milestoneID)
	if err != nil || milObj == nil {
		return
	}
	st, _ := milObj[objects.FieldKeyStatus].(string)
	if isTerminalStatus(st) {
		return
	}

	storageCtx := pkgctx.NewStorageContext()
	seen := make(map[string]map[string]any)
	filter1 := storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyMilestoneRef: milestoneID,
		},
	}
	if res1, err := provider.List(ctx, secCtx, storageCtx, filter1); err == nil && res1 != nil {
		for _, obj := range res1.Objects {
			if id, _ := obj[objects.FieldKeyID].(string); id != "" {
				seen[id] = obj
			}
		}
	}
	filter2 := storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyMilestoneRefs: map[string]any{"$has": milestoneID},
		},
	}
	if res2, err := provider.List(ctx, secCtx, storageCtx, filter2); err == nil && res2 != nil {
		for _, obj := range res2.Objects {
			if id, _ := obj[objects.FieldKeyID].(string); id != "" {
				seen[id] = obj
			}
		}
	}
	if len(seen) == 0 {
		return
	}
	for _, obj := range seen {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if !isBacklogItemTerminalStatus(status) {
			return
		}
	}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllBacklogCompleteForMilestone, map[string]string{scopeMilestoneID: milestoneID})
	completeObjectAndEmitShockwaves(ctx, provider, secCtx, projectRoot, objects.KindMilestone, milestoneID, st, "milestone all backlog complete", nil)
}

// TryEmitForTestCasesContainingCriterion lists test cases whose criteria_refs include criterionID,
// decrements their remaining_open_count, and auto-transitions the test_case to complete when 0.
func TryEmitForTestCasesContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || criterionID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindTestCase,
		Filters: map[string]any{
			objects.FieldKeyCriteriaRefs: map[string]any{"$has": criterionID},
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return
	}
	for _, tc := range result.Objects {
		tcID, _ := tc[objects.FieldKeyID].(string)
		tcStatus, _ := tc[objects.FieldKeyStatus].(string)
		if tcID == emptyValue || isTerminalStatus(tcStatus) {
			continue
		}
		if _, ok := remainingOpenCountFrom(tc); !ok {
			_ = SeedRemainingOpenCountFromMembers(ctx, provider, tcID, false)
		}
		rem, err := casDecrementRemainingOpenCount(ctx, provider, secCtx, tcID)
		if err != nil {
			continue
		}
		if rem <= 0 {
			extra := map[string]any{objects.FieldKeyRemainingOpenCount: 0}
			if updatedTc := completeObjectAndEmitShockwaves(ctx, provider, secCtx, projectRoot, objects.KindTestCase, tcID, tcStatus, "test_case criteria drained", extra); updatedTc != nil {
				for _, reqID := range StringRefsFromAny(updatedTc[objects.FieldKeyRequirementRefs]) {
					TryEmitAllCriteriaCompleteForRequirement(ctx, projectRoot, reqID, getStorage)
				}
			}
		}
	}
}

// TryEmitForRequirementsContainingCriterion lists requirements whose criteria_refs include criterionID,
// then evaluates TryEmitAllCriteriaCompleteForRequirement for each. Call when a criterion transitions
// toward a satisfied state so requirements can auto-complete when their linked criteria are all met.
func TryEmitForRequirementsContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	forEachObjectContainingCriterion(ctx, projectRoot, criterionID, objects.KindRequirement, getStorage, func(reqID string) {
		TryEmitAllCriteriaCompleteForRequirement(ctx, projectRoot, reqID, getStorage)
	})
}

// TryEmitAllCriteriaCompleteForRequirement checks whether every criterion in the requirement's criteria_refs
// is satisfied (validated or complete) AND all linked test cases are complete. If so, appends
// CriterionSatisfied(all_criteria_complete_for_requirement, requirement_id=requirementID) to the lifecycle WAL,
// transitions the requirement to complete, and propagates shockwaves.
func TryEmitAllCriteriaCompleteForRequirement(ctx context.Context, projectRoot, requirementID string, getStorage StorageProviderForCriterion) {
	provider, ok := resolveStorageProvider(projectRoot, getStorage)
	if !ok || requirementID == emptyValue {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	reqObj, err := provider.Read(ctx, secCtx, requirementID)
	if err != nil || reqObj == nil {
		return
	}
	kind, _ := reqObj[objects.FieldKeyKind].(string)
	if kind != objects.KindRequirement {
		return
	}
	st, _ := reqObj[objects.FieldKeyStatus].(string)
	if isTerminalStatus(st) {
		return
	}
	critAny, ok := reqObj[objects.FieldKeyCriteriaRefs]
	if !ok {
		return
	}
	critAny, ok = nildecode.DecodeNonNilPayload[any](critAny)
	if !ok {
		return
	}
	critIDs := StringRefsFromAny(critAny)
	if !areAllLinkedCriteriaSatisfied(ctx, provider, secCtx, critIDs) {
		return
	}

	// Verify all test cases pointing to this requirement are complete
	storageCtx := pkgctx.NewStorageContext()
	tcFilter := storage.ListFilter{
		Kind: objects.KindTestCase,
		Filters: map[string]any{
			objects.FieldKeyRequirementRefs: map[string]any{"$has": requirementID},
		},
	}
	if tcResult, tcErr := provider.List(ctx, secCtx, storageCtx, tcFilter); tcErr == nil && tcResult != nil {
		for _, tcObj := range tcResult.Objects {
			tcStatus, _ := tcObj[objects.FieldKeyStatus].(string)
			if tcStatus != objects.ObjectStatusComplete && tcStatus != objects.ObjectStatusArchived {
				return
			}
		}
	}

	appendAndSyncCriterionSatisfied(projectRoot, criterionAllCriteriaCompleteForRequirement, map[string]string{scopeRequirementID: requirementID})
	completeObjectAndEmitShockwaves(ctx, provider, secCtx, projectRoot, objects.KindRequirement, requirementID, st, "requirement all criteria complete", nil)
}
