package storage

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Sweep actions recorded on each ObjectDraftPlaneSweepItem.
const (
	sweepActionWouldDelete = "would_delete"
	sweepActionDeleted     = "deleted"
	// sweepOperationName labels the operation for RequireDraftPlaneApplyGate diagnostics.
	sweepOperationName = "sweep"
)

// processAdminSweepKinds are the kernel CAS kinds whose drafts may only be swept by a caller
// holding process-admin authority. The gate is keyed on the *capability* (see
// [processAdminAuthority]), not on a persona, seat, or vendor name: whoever the process-admin
// role is assigned to can change without this code changing.
var processAdminSweepKinds = map[string]bool{
	objects.KindBacklogItem:    true,
	objects.KindGoal:           true,
	objects.KindMilestone:      true,
	objects.KindCriteria:       true,
	objects.KindTechnicalDebt:  true,
	objects.KindDecision:       true,
	objects.KindWorkstream:     true,
	objects.KindAgentSkill:     true,
	objects.KindPolicy:         true,
	objects.KindPromptTemplate: true,
	objects.KindAgentTask:      true,
	objects.KindWorkflow:       true,
	objects.KindPersona:        true,
}

// requiresProcessAdminSweep reports whether sweeping this kind needs process-admin authority.
func requiresProcessAdminSweep(kind string) bool {
	return processAdminSweepKinds[kind]
}

// processAdminAuthority marks security contexts that expose process-admin capability.
type processAdminAuthority interface {
	IsProcessAdmin() bool
}

// lacksProcessAdminAuthority reports whether secCtx cannot prove process-admin capability
// (nil, or a context type that does not implement [processAdminAuthority]).
func lacksProcessAdminAuthority(secCtx interface{}) bool {
	if secCtx == nil {
		return true
	}
	_, ok := secCtx.(processAdminAuthority)
	return !ok
}

// checkSweepEntitlement denies a mutating sweep of a process-admin kind when the caller cannot
// prove process-admin capability. Dry-run listing is always allowed.
func checkSweepEntitlement(secCtx interface{}, opts ObjectDraftPlaneSweepOptions, kind string) error {
	if opts.DryRun || !requiresProcessAdminSweep(kind) {
		return nil
	}
	if lacksProcessAdminAuthority(secCtx) {
		return errfmt.Errorf("draft sweep entitlement denied: %s kind requires process admin authority", kind)
	}
	return nil
}

// SweepObjectDraftPlaneWithEntitlement matches draft-plane entries and, unless DryRun, deletes
// them — after the caller clears both the entitlement check and the apply gate.
func SweepObjectDraftPlaneWithEntitlement(secCtx interface{}, ctx context.Context, projectRoot string, opts ObjectDraftPlaneSweepOptions) (*ObjectDraftPlaneSweepResult, error) {
	out := &ObjectDraftPlaneSweepResult{DryRun: opts.DryRun, DraftRoot: ObjectDraftPlaneRoot(projectRoot)}

	if err := authorizeSweepApply(secCtx, opts); err != nil {
		return out, err
	}

	matched, skipped, inv, err := MatchObjectDraftPlane(projectRoot, sweepMatchOptions(opts))
	if err != nil {
		return out, err
	}
	populateSweepMatches(out, inv, matched, skipped)

	return applySweepToMatched(secCtx, out, matched, opts)
}

func populateSweepMatches(out *ObjectDraftPlaneSweepResult, inv ObjectDraftPlaneInventory, matched, skipped []ObjectDraftPlaneCandidate) {
	out.Inventory = inv
	out.Matched = len(matched)
	out.Skipped = len(skipped)
	appendSkippedSweepItems(out, skipped)
}

// sweepMatchOptions projects sweep options onto the draft-plane match filter.
func sweepMatchOptions(opts ObjectDraftPlaneSweepOptions) ObjectDraftPlaneMatchOptions {
	return ObjectDraftPlaneMatchOptions{
		Kind: opts.Kind, IDPrefix: opts.IDPrefix, Status: opts.Status,
		OlderThan: opts.OlderThan, All: opts.All, Max: opts.Max, IncludeCASBacked: true,
	}
}

// authorizeSweepApply runs the pre-match gates for a mutating sweep: process-admin entitlement
// for the requested kind, then the blast-radius apply gate. Dry-run needs neither.
func authorizeSweepApply(secCtx interface{}, opts ObjectDraftPlaneSweepOptions) error {
	if opts.DryRun {
		return nil
	}
	if err := checkSweepEntitlement(secCtx, opts, opts.Kind); err != nil {
		return err
	}
	return RequireDraftPlaneApplyGate(sweepMatchOptions(opts), sweepOperationName)
}

// appendSkippedSweepItems records why each non-matching entry was left alone.
func appendSkippedSweepItems(out *ObjectDraftPlaneSweepResult, skipped []ObjectDraftPlaneCandidate) {
	for _, s := range skipped {
		out.Items = append(out.Items, ObjectDraftPlaneSweepItem{
			ID: s.ID, Kind: s.Kind, Status: s.Status, Path: s.Path,
			Action: s.Action, Reason: s.Reason,
		})
	}
}

// applySweepToMatched records would-delete items on dry-run, otherwise re-checks entitlement per
// matched kind (a filter may span kinds) and removes the draft file.
func applySweepToMatched(secCtx interface{}, out *ObjectDraftPlaneSweepResult, matched []ObjectDraftPlaneCandidate, opts ObjectDraftPlaneSweepOptions) (*ObjectDraftPlaneSweepResult, error) {
	for _, m := range matched {
		item := ObjectDraftPlaneSweepItem{ID: m.ID, Kind: m.Kind, Status: m.Status, Path: m.Path}
		if opts.DryRun {
			item.Action = sweepActionWouldDelete
			out.Items = append(out.Items, item)
			continue
		}
		if err := checkSweepEntitlement(secCtx, opts, m.Kind); err != nil {
			return nil, err
		}
		if err := fileutil.Remove(item.Path); err != nil && !fileutil.IsNotExist(err) {
			return out, errfmt.Newf("object draft plane sweep: delete %s", m.ID).Wrap(err)
		}
		out.Deleted++
		item.Action = sweepActionDeleted
		out.Items = append(out.Items, item)
	}
	return out, nil
}
