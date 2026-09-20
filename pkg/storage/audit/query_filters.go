package audit

import (
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	FilterOpEq  = "$eq"
	FilterOpNe  = "$ne"
	FilterOpIn  = "$in"
	FilterOpGte = "$gte"
	FilterOpLte = "$lte"
	FilterOpLt  = "$lt"
)

// CreatedAtInclusiveWindow is a List filter for created_at in [start, end].
func CreatedAtInclusiveWindow(start, end time.Time) map[string]any {
	return map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{
			FilterOpGte: start.Format(time.RFC3339),
			FilterOpLte: end.Format(time.RFC3339),
		},
	}
}

// CreatedAtBefore is a List filter for created_at < cutoff.
func CreatedAtBefore(cutoff time.Time) map[string]any {
	return map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{
			FilterOpLt: cutoff.Format(time.RFC3339),
		},
	}
}

// StatusEq is a List filter for status == value.
func StatusEq(status string) map[string]any {
	return map[string]any{
		objects.FieldKeyStatus: map[string]any{FilterOpEq: status},
	}
}

// StatusNe is a List filter for status != value.
func StatusNe(status string) map[string]any {
	return map[string]any{
		objects.FieldKeyStatus: map[string]any{FilterOpNe: status},
	}
}

// MergeFilters copies parts into one filter map (later keys win).
func MergeFilters(parts ...map[string]any) map[string]any {
	out := make(map[string]any)
	for _, p := range parts {
		maps.Copy(out, p)
	}
	return out
}

// WithStatusIn adds status $in statuses to filters. Empty statuses leaves filters unchanged.
func WithStatusIn(filters map[string]any, statuses []string) map[string]any {
	if len(statuses) == 0 {
		return filters
	}
	out := maps.Clone(filters)
	if out == nil {
		out = make(map[string]any)
	}
	out[objects.FieldKeyStatus] = map[string]any{FilterOpIn: statuses}
	return out
}

// IDsIn is a List filter for id $in ids.
func IDsIn(ids []string) map[string]any {
	return map[string]any{
		objects.FieldKeyID: map[string]any{FilterOpIn: ids},
	}
}

// ExactAggregationWindow is a List filter for an exact aggregation window.
func ExactAggregationWindow(start, end string) map[string]any {
	return map[string]any{
		objects.FieldKeyAggregationWindowStart: map[string]any{FilterOpEq: start},
		objects.FieldKeyAggregationWindowEnd:   map[string]any{FilterOpEq: end},
	}
}

// SourceEq is a List filter for source == value.
func SourceEq(source string) map[string]any {
	return map[string]any{
		objects.FieldKeySource: map[string]any{FilterOpEq: source},
	}
}

// CollectIDs returns non-empty id fields from listed objects.
func CollectIDs(objs []map[string]any) []string {
	out := make([]string, 0, len(objs))
	for _, obj := range objs {
		if id := objects.GetString(obj, objects.FieldKeyID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// EligibleAggregationStatuses are terminal non-archived audit_event statuses.
var EligibleAggregationStatuses = []string{
	StatusCompleted,
	objects.FieldKeyFailed,
	"reverted",
	"error",
}

// WindowStatusFilters is the List filter for aggregation ingest.
func WindowStatusFilters(start, end time.Time) map[string]any {
	return WithStatusIn(CreatedAtInclusiveWindow(start, end), EligibleAggregationStatuses)
}
