package crud

import (
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func EffectiveListLimit(filter *ListFilter, _ *pkgctx.StorageContext) int {
	if filter == nil {
		return 0
	}
	if filter.Limit == 0 {
		return 0
	}
	return filter.Limit
}

// idsFromListFilter returns object IDs mentioned in list filters (id=..., id $eq, id $in).
// Used so CAS list includes those IDs in the candidate set even when not yet in the index (discovery path).
func IdsFromListFilter(filters map[string]any) []string {
	if len(filters) == 0 {
		return nil
	}
	raw, ok := filters[objects.FieldKeyID]
	if !ok {
		return nil
	}
	// Simple equality: id=SCH-020 -> "SCH-020"
	if s, ok := raw.(string); ok && s != emptyValue {
		return []string{s}
	}
	// Operator map: id={"$eq": "SCH-020"} or id={"$in": ["SCH-020", "SCH-021"]}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if eq, ok := m["$eq"]; ok {
		if s, ok := eq.(string); ok && s != emptyValue {
			return []string{s}
		}
	}
	if in, ok := m["$in"]; ok {
		sl, ok := in.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(sl))
		for _, v := range sl {
			if s, ok := v.(string); ok && s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// listFilterIsOnlyCreatedAtRange returns true when filters contain only a created_at range ($gte and/or $lte).
func ListFilterIsOnlyCreatedAtRange(filters map[string]any) bool {
	if len(filters) == 0 || len(filters) > 1 {
		return false
	}
	ca, ok := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if !ok || len(ca) == 0 {
		return false
	}
	for k := range ca {
		if k != "$gte" && k != "$lte" && k != "$gt" && k != "$lt" {
			return false
		}
	}
	return true
}

// parseCreatedAtOlderThan parses created_at $lt or $lte from filters. Returns cutoff time and true if found (no $gte).
// Used for "older than X" retention/cleanup queries so we can use cache.QueryOlderThan and avoid loading all IDs.
func ParseCreatedAtOlderThan(filters map[string]any) (cutoff time.Time, ok bool) {
	ca, _ := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if ca == nil {
		return time.Time{}, false
	}
	if _, hasGte := ca["$gte"]; hasGte {
		return time.Time{}, false
	}
	if s, v := ca["$lt"].(string); v && s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	if s, v := ca["$lte"].(string); v && s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseCreatedAtRangeFromFilters parses created_at $gte and $lte from filters. Returns startTime, endTime, true if both found.
func ParseCreatedAtRangeFromFilters(filters map[string]any) (startTime, endTime time.Time, ok bool) {
	ca, _ := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if ca == nil {
		return time.Time{}, time.Time{}, false
	}
	var hasStart, hasEnd bool
	if s := objects.GetString(ca, "$gte"); s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startTime = t
			hasStart = true
		}
	}
	if s := objects.GetString(ca, "$lte"); s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			endTime = t
			hasEnd = true
		}
	}
	return startTime, endTime, hasStart && hasEnd
}
