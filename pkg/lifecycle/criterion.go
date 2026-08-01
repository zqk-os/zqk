// Package lifecycle: helpers to emit criterion_satisfied when conditions are met (e.g. all backlog items complete for plan).

package lifecycle

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// Backlog item terminal statuses (same as priority plan rule: complete, archived, rejected).
var backlogItemTerminalStatuses = []any{statusComplete, statusArchived, statusRejected}

// StorageProvider is the same as in updater: returns storage for a project root.
type StorageProviderForCriterion func(projectRoot string) (storage.ObjectStorageProvider, bool)

// TryEmitAllBacklogItemsCompleteForPlan checks whether all backlog items for planID are in a terminal status.
// If so, appends CriterionSatisfied(all_backlog_items_complete_for_plan, plan_id=planID) to the lifecycle WAL.
// Call from lifecycle hook when a backlog_item transitions to complete (e.g. in a goroutine). No-op if storage unavailable or not all complete.
func TryEmitAllBacklogItemsCompleteForPlan(ctx context.Context, projectRoot, planID string, getStorage StorageProviderForCriterion) {
	if projectRoot == emptyValue || planID == emptyValue {
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
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: planID},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return
	}
	for _, obj := range result.Objects {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if !statusInList(status, backlogItemTerminalStatuses) {
			return
		}
	}
	scope := map[string]string{scopePlanID: planID}
	_ = AppendCriterionSatisfied(wal, criterionAllBacklogComplete, scope)
	_ = wal.Sync()
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
	_ = wal.Sync()
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

// TryEmitForBacklogItemsContainingCriterion evaluates backlog items linked from the criterion's backlog_item_refs
// when that criterion moves toward satisfied. No list scan: uses the reverse ref on criteria.
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
	co, err := provider.Read(ctx, secCtx, criterionID)
	if err != nil || co == nil {
		return
	}
	if k, _ := co[objects.FieldKeyKind].(string); k != objects.KindCriteria {
		return
	}
	refs := StringRefsFromAny(co[objects.FieldKeyBacklogItemRefs])
	for _, bliID := range refs {
		if bliID != emptyValue {
			TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, projectRoot, bliID, getStorage)
		}
	}
}

// TryEmitAllAcceptanceCriteriaMetForBacklogItem appends all_acceptance_criteria_met_for_backlog_item when every
// CRIT-* listed in the backlog item's acceptance_criteria is validated/complete and the item is in_progress.
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
	critIDs := critIDsFromBacklogAcceptanceList(bli[objects.FieldKeyAcceptanceCriteria])
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
	_ = wal.Sync()
}

func critIDsFromBacklogAcceptanceList(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimSpace(s[:i])
			}
			if strings.HasPrefix(s, prefixCrit) {
				out = append(out, s)
			}
		}
	case []string:
		for _, s := range t {
			s = strings.TrimSpace(s)
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimSpace(s[:i])
			}
			if strings.HasPrefix(s, prefixCrit) {
				out = append(out, s)
			}
		}
	}
	return out
}

// CriterionStatusMeetsMilestoneGateForMilestone returns true if criteria.status counts as satisfied
// for milestone completion (aligned with milestone get overlay: validated or complete).
func CriterionStatusMeetsMilestoneGateForMilestone(status string) bool {
	switch status {
	case objects.ObjectStatusValidated, objects.ObjectStatusCompleted, statusComplete:
		return true
	default:
		return false
	}
}

func StringRefsFromAny(v any) []string {
	switch t := v.(type) {
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
		if !statusInList(status, backlogItemTerminalStatuses) {
			return
		}
	}
	scope := map[string]string{scopeMilestoneID: milestoneID}
	_ = AppendCriterionSatisfied(wal, criterionAllBacklogCompleteForMilestone, scope)
	_ = wal.Sync()
}

func statusInList(status string, list []any) bool {
	for _, v := range list {
		if s, ok := v.(string); ok && s == status {
			return true
		}
	}
	return false
}
