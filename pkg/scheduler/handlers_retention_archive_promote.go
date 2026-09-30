package scheduler

import (
	"context"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// objectReader is the storage surface retentionArchiveCandidate needs.
type objectReader interface {
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
}

// retentionArchiveCandidate reports whether retention may archive this object.
// priority_plan: only complete (last-child closeout) — promote→archived owns the prune burrito.
// backlog_item with a live/complete plan_ref: skip; the plan promote shockwave archives children.
// backlog_item under an already-archived plan (or orphan): eligible for direct archive.
func retentionArchiveCandidate(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	sp objectReader,
	kind string,
	obj map[string]any,
	archiveStatus string,
) bool {
	if obj == nil {
		return false
	}
	status, _ := obj[objects.FieldKeyStatus].(string)
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" || status == archiveStatus {
		return false
	}
	switch kind {
	case objects.KindPriorityPlan:
		return status == objects.ObjectStatusComplete
	case objects.KindBacklogItem:
		planRef := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyPriorityPlanRef))
		if planRef == "" {
			return true
		}
		if sp == nil {
			return false
		}
		plan, err := sp.Read(ctx, secCtx, planRef)
		if err != nil || plan == nil {
			// Fail closed: do not BulkUpdate a child whose plan cannot be verified.
			return false
		}
		planStatus, _ := plan[objects.FieldKeyStatus].(string)
		planStatus = strings.ToLower(strings.TrimSpace(planStatus))
		return planStatus == objects.ObjectStatusArchived
	default:
		return true
	}
}

// archiveViaPromoteMembrane archives seed through Plan+Apply stage membrane
// (PRI prune fail-closed burrito). Used for complete priority_plan retention.
func (h *RetentionToleranceHandler) archiveViaPromoteMembrane(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID, seedID string,
) error {
	if h == nil || h.storage == nil || h.projectRoot == "" {
		return errfmt.Errorf("retention archive promote: storage/project root unavailable")
	}
	lifecyclesDir := filepath.Join(h.projectRoot, paths.ProcessInternalLifecyclesDir)
	loader := objects.NewLifecycleLoader(lifecyclesDir)
	dependents := func(id string) []string {
		return storagepkg.DependentsForID(ctx, h.storage, id)
	}
	hop, err := lifecycle.PlanStageMembraneHop(ctx, secCtx, h.storage, loader, dependents, []string{seedID}, objects.ObjectStatusArchived)
	if err != nil {
		return err
	}
	if err := lifecycle.ApplyStageMembraneHop(ctx, secCtx, h.storage, dependents, hop); err != nil {
		return err
	}
	kinds := make(map[string]struct{}, len(hop.Members))
	for _, m := range hop.Members {
		kinds[m.Kind] = struct{}{}
	}
	for kind := range kinds {
		storagepkg.InvalidateListCacheForKind(kind)
	}
	RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceArchivedByTolerance).
		JobID(jobID).
		String("seed_id", seedID).
		String("path", "promote_membrane").
		Int("members", len(hop.AppliedMembers())).
		Log()
	return nil
}
