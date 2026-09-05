package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// DependentsForID returns one-level reverse dependents for id.
// Prefer the in-memory reverse-reference index (loaded/persisted with CUD),
// then union list(backlog_item, priority_plan_ref=id) so a stale/partial
// reverse index cannot hide ready children (promote precondition flake).
// Membership occupancy is EdgeRoleMembership; this list fallback is the
// safety net until the reverse index is field+role keyed.
//
// TRACK: [REDACTED-ID] — list fallback is a safety net while
// reverse-index SaveCache/load settles; prefer cache hits for hot path.
// TRACK: REDACTED — merge list even when index non-empty
// (partial index returned archived-only deps and blocked PRI-SYM-005 promote).
func DependentsForID(ctx context.Context, sp ObjectStorageProvider, id string) []string {
	if id == "" {
		return nil
	}
	if root := reverseReferenceBoundProjectRoot(); root != emptyValue {
		ensureReverseReferenceIndexLoaded(root)
	} else if f, ok := sp.(*FileObjectStorage); ok && f != nil && f.projectRoot != emptyValue {
		BindReverseReferenceIndexProjectRoot(f.projectRoot)
	}
	revIndex := GetGlobalReverseReferenceIndex()
	var deps []string
	if revIndex.IsReady() {
		deps = revIndex.GetDependents(id)
	}
	seen := make(map[string]struct{}, len(deps)+8)
	out := make([]string, 0, len(deps)+8)
	for _, oid := range deps {
		if oid == "" {
			continue
		}
		if _, ok := seen[oid]; ok {
			continue
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	if sp == nil {
		if !revIndex.IsReady() {
			return nil
		}
		return out
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: id,
		},
	})
	if err != nil {
		if !revIndex.IsReady() {
			return nil
		}
		return out
	}
	if res != nil {
		for _, obj := range res.Objects {
			oid, _ := obj[objects.FieldKeyID].(string)
			if oid == "" {
				continue
			}
			if _, ok := seen[oid]; ok {
				continue
			}
			seen[oid] = struct{}{}
			out = append(out, oid)
		}
	}
	if !revIndex.IsReady() && res == nil {
		return nil
	}
	return out
}
