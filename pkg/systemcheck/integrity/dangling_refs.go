package integrity

import (
	"context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// DanglingRefHit records an instance where an object's reference field points to a missing target.
type DanglingRefHit struct {
	ObjectID   string `json:"object_id"`
	ObjectKind string `json:"object_kind"`
	Field      string `json:"field"`
	MissingRef string `json:"missing_ref"`
}

// CriticalKindList returns the list of critical process and governance kinds that require reference validation.
func CriticalKindList() []string {
	kinds := kernelcas.ListCriticalKinds()
	if len(kinds) > 0 {
		return kinds
	}
	// Spec index unavailable: still scan process kinds that carry *_ref/_refs so
	// heal-dangling cannot silently report planned=0 while GhostRefs remain.
	return []string{
		objects.KindBacklogItem,
		objects.KindCriteria,
		objects.KindRequirement,
		objects.KindMilestone,
		objects.KindPriorityPlan,
		objects.KindGoal,
		objects.KindPolicy,
		objects.KindRiskBlocker,
		objects.KindTechnicalDebt,
		objects.KindConvergenceSession,
	}
}

// IsRefFieldName reports whether field name matches reference conventions.
func IsRefFieldName(name string) bool {
	return strings.HasSuffix(name, "_ref") || strings.HasSuffix(name, "_refs") || name == "dependencies"
}

// RefIDsFromValue extracts string identifier slices from an untyped field value.
func RefIDsFromValue(v any) []string {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{t}
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// ScanDanglingRefs scans critical kinds in storage for unresolvable references.
func ScanDanglingRefs(ctx context.Context, sec *pkgctx.SecurityContext, store storage.ObjectStorageProvider, sampleLimit int) []DanglingRefHit {
	var hits []DanglingRefHit
	for _, kind := range CriticalKindList() {
		res, err := store.List(ctx, sec, nil, storage.ListFilter{Kind: kind})
		if err != nil || res == nil {
			continue
		}
		for _, obj := range res.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			for field, val := range obj {
				if !IsRefFieldName(field) {
					continue
				}
				for _, refID := range RefIDsFromValue(val) {
					exists, err := store.Exists(ctx, sec, refID)
					if err != nil || exists {
						continue
					}
					hits = append(hits, DanglingRefHit{
						ObjectID: id, ObjectKind: kind, Field: field, MissingRef: refID,
					})
					if sampleLimit > 0 && len(hits) >= sampleLimit {
						return hits
					}
				}
			}
		}
	}
	return hits
}

// BuildUnlinkUpdates produces the mutation map to cleanly remove unresolvable references from an object.
func BuildUnlinkUpdates(obj map[string]any, field string, missing map[string]struct{}) (map[string]any, error) {
	val, ok := obj[field]
	if !ok {
		return nil, errfmt.Errorf("field %s missing", field)
	}
	if strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs") {
		if s, ok := val.(string); ok {
			if _, gone := missing[s]; gone {
				return map[string]any{field: storage.FieldUnset}, nil
			}
		}
		return nil, nil
	}
	keep := make([]string, 0)
	for _, id := range RefIDsFromValue(val) {
		if _, gone := missing[id]; !gone {
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return map[string]any{field: storage.FieldUnset}, nil
	}
	return map[string]any{field: keep}, nil
}
