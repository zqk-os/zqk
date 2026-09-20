package lifecycle

import (
	"context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/storage"
)

// RecomputeRefsFromScope returns the status-relevant graph refs for the given scope.
// Used by the beyond-threshold rollback path when the rollback point was pruned.
// For scopeType ScopeTypeLifecycle, scopeID format is "kind:id:toStatus" (e.g. "priority_plan:PRI-1:complete").
// Returns nil if scope cannot be parsed or provider is nil.
func RecomputeRefsFromScope(ctx context.Context, scopeType, scopeID string, provider storage.ObjectStorageProvider) []rollback.ObjectRef {
	if provider == nil {
		return nil
	}
	if scopeType != rollback.ScopeTypeLifecycle || scopeID == emptyValue {
		return nil
	}
	parts := strings.SplitN(scopeID, ":", 3)
	if len(parts) < 3 {
		return nil
	}
	kind, id, toStatus := parts[0], parts[1], parts[2]
	req := TransitionRequest{Kind: kind, ID: id, ToStatus: toStatus}
	return StatusRelevantRefs(ctx, provider, req)
}

// StatusRelevantRefs returns object refs (kind, id) that form the status-relevant graph for the
// given transition request. Used to capture a rollback snapshot before apply.
func StatusRelevantRefs(ctx context.Context, provider storage.ObjectStorageProvider, req TransitionRequest) []rollback.ObjectRef {
	refs := []rollback.ObjectRef{{Kind: req.Kind, ID: req.ID}}
	if req.Kind != objects.KindPriorityPlan || req.ID == emptyValue || provider == nil {
		return refs
	}
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Filters: map[string]any{objects.FieldKeyPriorityPlanRef: req.ID},
	}
	result, err := provider.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return refs
	}
	for _, obj := range result.Objects {
		if id, _ := obj[objects.FieldKeyID].(string); id != emptyValue {
			refs = append(refs, rollback.ObjectRef{Kind: objects.KindBacklogItem, ID: id})
		}
	}
	return refs
}
