package rollup

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// FetchChildrenForParent retrieves the child objects associated with a parent object.
// It resolves parent-child relationships generically using membership fields and reverse references.
func FetchChildrenForParent(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, parentKind, parentID string) ([]map[string]any, error) {
	if sp == nil || parentID == "" {
		return nil, nil
	}

	storageCtx := pkgctx.NewStorageContext()
	seen := make(map[string]struct{})
	var children []map[string]any

	// 1. Direct kind-indexed membership lookups for core containers
	switch parentKind {
	case objects.KindMilestone:
		resRef, errRef := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyMilestoneRef: parentID,
			},
		})
		if errRef == nil && resRef != nil {
			for _, obj := range resRef.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" && isChildOfParent(obj, parentKind, parentID) {
					seen[id] = struct{}{}
					children = append(children, obj)
				}
			}
		}
		res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyMilestoneRefs: map[string]any{"$has": parentID},
			},
		})
		if err == nil && res != nil {
			for _, obj := range res.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" && isChildOfParent(obj, parentKind, parentID) {
					if _, exists := seen[id]; !exists {
						seen[id] = struct{}{}
						children = append(children, obj)
					}
				}
			}
		}

	case objects.KindPriorityPlan:
		res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind: objects.KindBacklogItem,
			Filters: map[string]any{
				objects.FieldKeyPriorityPlanRef: parentID,
			},
		})
		if err == nil && res != nil {
			for _, obj := range res.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" && isChildOfParent(obj, parentKind, parentID) {
					seen[id] = struct{}{}
					children = append(children, obj)
				}
			}
		}

	default:
		res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{})
		if err == nil && res != nil {
			for _, obj := range res.Objects {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" && isChildOfParent(obj, parentKind, parentID) {
					if _, exists := seen[id]; !exists {
						seen[id] = struct{}{}
						children = append(children, obj)
					}
				}
			}
		}
	}

	// 2. Generic reverse-reference fallback to capture any other child objects (e.g. custom kinds)
	depIDs := storage.DependentsForID(ctx, sp, parentID)
	var toFetch []string
	for _, id := range depIDs {
		if _, exists := seen[id]; !exists && id != "" {
			toFetch = append(toFetch, id)
		}
	}

	if len(toFetch) > 0 {
		bulkRes, err := sp.BulkGet(ctx, secCtx, toFetch)
		if err == nil && bulkRes != nil {
			for _, obj := range bulkRes.Results {
				id, _ := obj[objects.FieldKeyID].(string)
				if id != "" && isChildOfParent(obj, parentKind, parentID) {
					seen[id] = struct{}{}
					children = append(children, obj)
				}
			}
		}
	}

	return children, nil
}

func isChildOfParent(obj map[string]any, parentKind, parentID string) bool {
	if obj == nil || parentID == "" {
		return false
	}

	switch parentKind {
	case objects.KindMilestone:
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind == objects.KindRoadmap || kind == objects.KindGoal || kind == objects.KindStrategicPlan {
			return false
		}
		if objects.GetString(obj, objects.FieldKeyMilestoneRef) == parentID {
			return true
		}
		refs := extractStringSlice(obj[objects.FieldKeyMilestoneRefs])
		for _, r := range refs {
			if r == parentID {
				return true
			}
		}
		return false

	case objects.KindPriorityPlan:
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind != "" && kind != objects.KindBacklogItem {
			return false
		}
		return objects.GetString(obj, objects.FieldKeyPriorityPlanRef) == parentID
	}

	// Generic check: does any field on obj reference parentID?
	for _, val := range obj {
		switch v := val.(type) {
		case string:
			if v == parentID {
				return true
			}
		case []string:
			for _, r := range v {
				if r == parentID {
					return true
				}
			}
		case []any:
			for _, r := range v {
				if s, ok := r.(string); ok && s == parentID {
					return true
				}
			}
		}
	}

	return false
}

func extractStringSlice(val any) []string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}
