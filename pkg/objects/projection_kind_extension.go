package objects

import (
	"strings"
	"sync"
)

// Kind-local projection columns (extension slots 0..len-1) for kinds whose schemas include many
// fields not covered by [globalSlotKeys]. Fields listed here must **not** duplicate a global slot key.
// Any top-level key requested at runtime that is neither global nor listed here is handled via
// [HybridProjectionMask.Overflow] (still correct; avoids copying slot strings only for hot presets).
//
// Work-envelope clocks/effort are **not** listed here: they follow KindHasTrait
// (completable → started_at; effort_aware → estimated_effort, actual_effort).
// completed_at stays a global slot.
var kindProjectionExtensions = map[string][]string{
	KindBacklogItem: {
		FieldKeyPriorityTier,
		FieldKeyBlockedByRefs,
		FieldKeyWorkstreamRefs,
	},
	KindMilestone: {
		FieldKeyBlockingMilestoneRefs,
		FieldKeyLinkedMilestoneRefs,
	},
	KindAuditEvent: {
		FieldKeyAggregationWindowStart,
		FieldKeyAggregationWindowEnd,
	},
}

var (
	kindExtKeysMemo  sync.Map // kind -> []string
	kindExtIndexMemo sync.Map // kind -> map[string]int
)

func workEnvelopeExtensionKeys(kind string) (keys []string, ok bool) {
	completable, errC := KindHasTrait(kind, "completable")
	effort, errE := KindHasTrait(kind, "effort_aware")
	if errC != nil || errE != nil {
		return nil, false
	}
	if completable {
		keys = append(keys, FieldKeyStartedAt)
	}
	if effort {
		keys = append(keys, FieldKeyEstimatedEffort, FieldKeyActualEffort)
	}
	return keys, true
}

func mergeKindProjectionKeys(kind string) (keys []string, cacheable bool) {
	extra, extraOK := workEnvelopeExtensionKeys(kind)
	cacheable = extraOK
	base := kindProjectionExtensions[kind]
	if len(base) == 0 && len(extra) == 0 {
		return nil, cacheable
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	appendUnique := func(k string) {
		if k == "" {
			return
		}
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for _, k := range base {
		appendUnique(k)
	}
	for _, k := range extra {
		appendUnique(k)
	}
	return out, cacheable
}

func kindExtensionKeys(kind string) []string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil
	}
	if v, ok := kindExtKeysMemo.Load(kind); ok {
		return v.([]string)
	}
	keys, cacheable := mergeKindProjectionKeys(kind)
	if cacheable {
		kindExtKeysMemo.Store(kind, keys)
	}
	return keys
}

func kindExtensionIndex(kind string) map[string]int {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil
	}
	if v, ok := kindExtIndexMemo.Load(kind); ok {
		return v.(map[string]int)
	}
	keys := kindExtensionKeys(kind)
	if len(keys) == 0 {
		return nil
	}
	idx := make(map[string]int, len(keys))
	for i, k := range keys {
		idx[k] = i
	}
	if _, cached := kindExtKeysMemo.Load(kind); cached {
		kindExtIndexMemo.Store(kind, idx)
	}
	return idx
}
