package storage

import (
	"context"
	"strings"

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
// TRACK: BLI-1785723654802038000-b14064bc — merge list even when index non-empty
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
	if strings.HasPrefix(id, "CRIT-") {
		tstRes, tstErr := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), ListFilter{
			Kind: objects.KindTestCase,
		})
		if tstErr == nil && tstRes != nil {
			for _, obj := range tstRes.Objects {
				oid, _ := obj[objects.FieldKeyID].(string)
				if oid == "" {
					continue
				}
				if _, ok := seen[oid]; ok {
					continue
				}
				if objectContainsRef(obj, objects.FieldKeyCriteriaRefs, id) || objectContainsRef(obj, "criteria_ref", id) {
					seen[oid] = struct{}{}
					out = append(out, oid)
				}
			}
		}
	}
	if !revIndex.IsReady() && res == nil {
		return nil
	}
	return out
}

func objectContainsRef(obj map[string]any, fieldKey, targetID string) bool {
	v, ok := obj[fieldKey]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case string:
		return val == targetID
	case []string:
		for _, s := range val {
			if s == targetID {
				return true
			}
		}
	case []any:
		for _, item := range val {
			if s, ok := item.(string); ok && s == targetID {
				return true
			}
		}
	}
	return false
}
