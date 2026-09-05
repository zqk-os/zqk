// Package lifecycle: helpers to emit criterion_satisfied when remaining-open drains (and other criteria).

package lifecycle

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// backlogItemTerminalStatuses was a package var listing complete, archived, and rejected. Only the
// first two are backlog_item statuses; rejected is a status_mapping alias for archived, so it never
// matched anything read back from storage. isBacklogItemTerminalStatus answers the same question
// from the lifecycle role.

// StorageProvider is the same as in updater: returns storage for a project root.
type StorageProviderForCriterion func(projectRoot string) (storage.ObjectStorageProvider, bool)

// TryEmitRemainingOpenDrained emits criterion_satisfied when remaining_open_count is 0.
// Unset field fail-closes (no member List).
// TRACK: BLI-CEF-CONTAINER-REMAINING-OPEN-001
func TryEmitRemainingOpenDrained(ctx context.Context, projectRoot, containerID string, getStorage StorageProviderForCriterion) {
	if projectRoot == emptyValue || containerID == emptyValue || getStorage == nil {
		return
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
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
	if projectRoot == emptyValue || milestoneID == emptyValue {
		return
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
		return
	}
	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
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
	if len(critIDs) == 0 {
		return
	}
	for _, cid := range critIDs {
		co, err := provider.Read(ctx, secCtx, cid)
		if err != nil || co == nil {
			return
		}
		cst, _ := co[objects.FieldKeyStatus].(string)
		if !CriterionStatusMeetsMilestoneGateForMilestone(cst) {
			return
		}
	}
	scope := map[string]string{scopeMilestoneID: milestoneID}
	_ = AppendCriterionSatisfied(wal, criterionAllCriteriaCompleteForMilestone, scope)
	if syncErr := wal.Sync(); syncErr != nil {
		logging.LogSwallowedError(syncErr)
	}
}

// TryEmitForMilestonesContainingCriterion lists milestones whose criteria_refs include criterionID,
// then evaluates TryEmitAllCriteriaCompleteForMilestone for each. Call when a criterion transitions
// toward a satisfied state so milestones can auto-complete when their linked criteria are all met.
func TryEmitForMilestonesContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	if projectRoot == emptyValue || criterionID == emptyValue {
		return
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindMilestone,
		Filters: map[string]any{
			objects.FieldKeyCriteriaRefs: map[string]any{"$has": criterionID},
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return
	}
	for _, mil := range result.Objects {
		mid, _ := mil[objects.FieldKeyID].(string)
		if mid != emptyValue {
			TryEmitAllCriteriaCompleteForMilestone(ctx, projectRoot, mid, getStorage)
		}
	}
}

// TryEmitForBacklogItemsContainingCriterion lists backlog items whose criteria_refs include criterionID,
// then evaluates TryEmitAllAcceptanceCriteriaMetForBacklogItem for each. Call when a criterion transitions
// toward a satisfied state so backlog items can auto-complete when their linked criteria are all met.
func TryEmitForBacklogItemsContainingCriterion(ctx context.Context, projectRoot, criterionID string, getStorage StorageProviderForCriterion) {
	if projectRoot == emptyValue || criterionID == emptyValue {
		return
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyCriteriaRefs: map[string]any{"$has": criterionID},
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return
	}
	for _, bli := range result.Objects {
		bliID, _ := bli[objects.FieldKeyID].(string)
		if bliID != emptyValue {
			TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, projectRoot, bliID, getStorage)
		}
	}
}

// TryEmitAllAcceptanceCriteriaMetForBacklogItem appends all_acceptance_criteria_met_for_backlog_item when every
// CRIT-* listed in the backlog item's criteria_refs is validated/complete and the item is in_progress.
func TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx context.Context, projectRoot, backlogItemID string, getStorage StorageProviderForCriterion) {
	if projectRoot == emptyValue || backlogItemID == emptyValue {
		return
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
		return
	}
	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
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
	if len(critIDs) == 0 {
		return
	}
	for _, cid := range critIDs {
		co, err := provider.Read(ctx, secCtx, cid)
		if err != nil || co == nil {
			return
		}
		cst, _ := co[objects.FieldKeyStatus].(string)
		if !CriterionStatusMeetsMilestoneGateForMilestone(cst) {
			return
		}
	}
	scope := map[string]string{scopeBacklogItemID: backlogItemID}
	_ = AppendCriterionSatisfied(wal, criterionAllAcceptanceCriteriaMetForBacklogItem, scope)
	if syncErr := wal.Sync(); syncErr != nil {
		logging.LogSwallowedError(syncErr)
	}
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
	if projectRoot == emptyValue || milestoneID == emptyValue {
		return
	}
	provider, ok := getStorage(projectRoot)
	if !ok {
		return
	}
	provider, ok = nildecode.DecodeNonNilPayload[storage.ObjectStorageProvider](provider)
	if !ok {
		return
	}
	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyMilestoneRefs: map[string]any{"$has": milestoneID},
		},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return
	}
	for _, obj := range result.Objects {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if !isBacklogItemTerminalStatus(status) {
			return
		}
	}
	scope := map[string]string{scopeMilestoneID: milestoneID}
	_ = AppendCriterionSatisfied(wal, criterionAllBacklogCompleteForMilestone, scope)
	if syncErr := wal.Sync(); syncErr != nil {
		logging.LogSwallowedError(syncErr)
	}
}
