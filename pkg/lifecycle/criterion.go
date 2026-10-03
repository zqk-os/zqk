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

// StorageProviderForCriterion is an alias for StorageProvider for backwards compatibility.
type StorageProviderForCriterion = StorageProvider


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

func completeObjectWithUpdatesAndEmitShockwaves(ctx context.Context, provider storage.ObjectStorageProvider, projectRoot, kind, id, oldStatus, reason string, extraUpdates map[string]any) (map[string]any, bool) {
	secCtx := pkgctx.NewSystemSecurityContext()
	trustedCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), reason)
	updates := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusComplete}
	for k, v := range extraUpdates {
		updates[k] = v
	}
	if updateErr := provider.Update(trustedCtx, secCtx, id, updates); updateErr != nil {
		return nil, false
	}
	if flushErr := storage.FlushListingIndexForProjectRoot(projectRoot, kind); flushErr != nil {
		logging.LogSwallowedError(flushErr)
	}
	updated, readErr := provider.Read(ctx, secCtx, id)
	if readErr != nil || updated == nil {
		return nil, false
	}
	ApplyDependencyRefEvents(ctx, logging.NewEventLogger(ctx), provider, projectRoot, kind, id, oldStatus, objects.ObjectStatusComplete, updated)
	return updated, true
}

func completeObjectAndEmitShockwaves(ctx context.Context, provider storage.ObjectStorageProvider, projectRoot, kind, id, oldStatus, reason string) {
	_, _ = completeObjectWithUpdatesAndEmitShockwaves(ctx, provider, projectRoot, kind, id, oldStatus, reason, nil)
}

func areAllLinkedCriteriaSatisfied(ctx context.Context, provider storage.ObjectStorageProvider, critIDs []string) bool {
	if len(critIDs) == 0 {
		return false
	}
	secCtx := pkgctx.NewSystemSecurityContext()
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
	provider, ok := resolveStorageProvider(getStorage, projectRoot)
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
	if err != nil {
		return
	}
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id != emptyValue {
			fn(id)
		}
	}
}

func readObjectForCriterion(
	ctx context.Context,
	projectRoot, objectID string,
	getStorage StorageProviderForCriterion,
) (storage.ObjectStorageProvider, map[string]any, bool) {
	if objectID == emptyValue {
		return nil, nil, false
	}
	provider, ok := resolveStorageProvider(getStorage, projectRoot)
	if !ok {
		return nil, nil, false
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := provider.Read(ctx, secCtx, objectID)
	if err != nil || obj == nil {
		return nil, nil, false
	}
	return provider, obj, true
}

func criteriaRefsFromObject(obj map[string]any) ([]string, bool) {
	critAny, ok := obj[objects.FieldKeyCriteriaRefs]
	if !ok {
		return nil, false
	}
	critAny, ok = nildecode.DecodeNonNilPayload[any](critAny)
	if !ok {
		return nil, false
	}
	return StringRefsFromAny(critAny), true
}

// TryEmitRemainingOpenDrained emits criterion_satisfied when remaining_open_count is 0.
// Unset field fail-closes (no member List).
func TryEmitRemainingOpenDrained(ctx context.Context, projectRoot, containerID string, getStorage StorageProviderForCriterion) {
	_, container, ok := readObjectForCriterion(ctx, projectRoot, containerID, getStorage)
	if !ok {
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
	provider, milObj, ok := readObjectForCriterion(ctx, projectRoot, milestoneID, getStorage)
	if !ok {
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
	critIDs, ok := criteriaRefsFromObject(milObj)
	if !ok || !areAllLinkedCriteriaSatisfied(ctx, provider, critIDs) {
		return
	}

	scope := map[string]string{scopeMilestoneID: milestoneID}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllCriteriaCompleteForMilestone, scope)
	completeObjectAndEmitShockwaves(ctx, provider, projectRoot, objects.KindMilestone, milestoneID, st, "milestone all criteria complete")
}

// TryEmitForMilestonesContainingCriterion lists milestones whose criteria_refs include criterionID,
// then evaluates TryEmitAllCriteriaCompleteForMilestone for each. Call when a criterion transitions
// toward a satisfied state so milestones can auto-complete when their linked criteria are all met.
func TryEmitForMilestonesContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	forEachObjectContainingCriterion(ctx, projectRoot, criterionID, objects.KindMilestone, getStorage, func(id string) {
		TryEmitAllCriteriaCompleteForMilestone(ctx, projectRoot, id, getStorage)
	})
}

// TryEmitForBacklogItemsContainingCriterion lists backlog items whose criteria_refs include criterionID,
// then evaluates TryEmitAllAcceptanceCriteriaMetForBacklogItem for each. Call when a criterion transitions
// toward a satisfied state so backlog items can auto-complete when their linked criteria are all met.
func TryEmitForBacklogItemsContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	forEachObjectContainingCriterion(ctx, projectRoot, criterionID, objects.KindBacklogItem, getStorage, func(id string) {
		TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, projectRoot, id, getStorage)
	})
}

// TryEmitAllAcceptanceCriteriaMetForBacklogItem appends all_acceptance_criteria_met_for_backlog_item when every
// CRIT-* listed in the backlog item's criteria_refs is validated/complete and the item is in_progress.
func TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx context.Context, projectRoot, backlogItemID string, getStorage StorageProviderForCriterion) {
	provider, bli, ok := readObjectForCriterion(ctx, projectRoot, backlogItemID, getStorage)
	if !ok {
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
	if !areAllLinkedCriteriaSatisfied(ctx, provider, critIDs) {
		return
	}

	scope := map[string]string{scopeBacklogItemID: backlogItemID}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllAcceptanceCriteriaMetForBacklogItem, scope)
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

func collectBacklogItemsForMilestone(ctx context.Context, provider storage.ObjectStorageProvider, milestoneID string) map[string]map[string]any {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	seen := make(map[string]map[string]any)
	filters := []storage.ListFilter{
		{
			Kind:    objects.KindBacklogItem,
			Filters: map[string]any{objects.FieldKeyMilestoneRef: milestoneID},
		},
		{
			Kind:    objects.KindBacklogItem,
			Filters: map[string]any{objects.FieldKeyMilestoneRefs: map[string]any{"$has": milestoneID}},
		},
	}
	for _, f := range filters {
		if res, err := provider.List(ctx, secCtx, storageCtx, f); err == nil && res != nil {
			for _, obj := range res.Objects {
				if id, _ := obj[objects.FieldKeyID].(string); id != "" {
					seen[id] = obj
				}
			}
		}
	}
	return seen
}

// TryEmitAllBacklogItemsCompleteForMilestone checks whether all backlog items referencing milestoneID
// are in a terminal status. If so, appends CriterionSatisfied(all_backlog_items_complete_for_milestone, milestone_id=milestoneID)
// to the lifecycle WAL.
func TryEmitAllBacklogItemsCompleteForMilestone(ctx context.Context, projectRoot, milestoneID string, getStorage StorageProviderForCriterion) {
	provider, milObj, ok := readObjectForCriterion(ctx, projectRoot, milestoneID, getStorage)
	if !ok {
		return
	}
	st, _ := milObj[objects.FieldKeyStatus].(string)
	if isTerminalStatus(st) {
		return
	}

	seen := collectBacklogItemsForMilestone(ctx, provider, milestoneID)
	if len(seen) == 0 {
		return
	}
	for _, obj := range seen {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if !isBacklogItemTerminalStatus(status) {
			return
		}
	}

	scope := map[string]string{scopeMilestoneID: milestoneID}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllBacklogCompleteForMilestone, scope)
	completeObjectAndEmitShockwaves(ctx, provider, projectRoot, objects.KindMilestone, milestoneID, st, "milestone all backlog complete")
}

// TryEmitForTestCasesContainingCriterion lists test cases whose criteria_refs include criterionID,
// decrements their remaining_open_count, and auto-transitions the test_case to complete when 0.
func TryEmitForTestCasesContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	if criterionID == emptyValue {
		return
	}
	provider, ok := resolveStorageProvider(getStorage, projectRoot)
	if !ok {
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
			if updatedTc, ok := completeObjectWithUpdatesAndEmitShockwaves(ctx, provider, projectRoot, objects.KindTestCase, tcID, tcStatus, "test_case criteria drained", map[string]any{
				objects.FieldKeyRemainingOpenCount: 0,
			}); ok {
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
	forEachObjectContainingCriterion(ctx, projectRoot, criterionID, objects.KindRequirement, getStorage, func(id string) {
		TryEmitAllCriteriaCompleteForRequirement(ctx, projectRoot, id, getStorage)
	})
}

// TryEmitAllCriteriaCompleteForRequirement checks whether every criterion in the requirement's criteria_refs
// is satisfied (validated or complete) AND all linked test cases are complete. If so, appends
// CriterionSatisfied(all_criteria_complete_for_requirement, requirement_id=requirementID) to the lifecycle WAL,
// transitions the requirement to complete, and propagates shockwaves.
func TryEmitAllCriteriaCompleteForRequirement(ctx context.Context, projectRoot, requirementID string, getStorage StorageProviderForCriterion) {
	provider, reqObj, ok := readObjectForCriterion(ctx, projectRoot, requirementID, getStorage)
	if !ok {
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
	critIDs, ok := criteriaRefsFromObject(reqObj)
	if !ok || !areAllLinkedCriteriaSatisfied(ctx, provider, critIDs) {
		return
	}

	// Verify all test cases pointing to this requirement are complete
	secCtx := pkgctx.NewSystemSecurityContext()
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

	scope := map[string]string{scopeRequirementID: requirementID}
	appendAndSyncCriterionSatisfied(projectRoot, criterionAllCriteriaCompleteForRequirement, scope)
	completeObjectAndEmitShockwaves(ctx, provider, projectRoot, objects.KindRequirement, requirementID, st, "requirement all criteria complete")
}
