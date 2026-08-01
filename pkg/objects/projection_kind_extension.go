package objects

import (
	"strings"
	"sync"
)

// Kind-local projection columns (extension slots 0..len-1) for kinds whose schemas include many
// fields not covered by [globalSlotKeys]. Fields listed here must **not** duplicate a global slot key.
// Any top-level key requested at runtime that is neither global nor listed here is handled via
// [HybridProjectionMask.Overflow] (still correct; avoids copying slot strings only for hot presets).
var kindProjectionExtensions = map[string][]string{
	KindBacklogItem: {
		FieldKeyPriorityTier,
		FieldKeyBlockedByRefs,
		FieldKeyEstimatedEffort,
		FieldKeyActualEffort,
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
	kindExtBuildOnce sync.Once
	kindExtIndexMemo map[string]map[string]int
)

func ensureKindExtensionIndex() {
	kindExtBuildOnce.Do(func() {
		kindExtIndexMemo = make(map[string]map[string]int, len(kindProjectionExtensions))
		for kind, keys := range kindProjectionExtensions {
			idx := make(map[string]int, len(keys))
			for i, k := range keys {
				if k == "" {
					continue
				}
				idx[k] = i
			}
			kindExtIndexMemo[kind] = idx
		}
	})
}

func kindExtensionKeys(kind string) []string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil
	}
	return kindProjectionExtensions[kind]
}

func kindExtensionIndex(kind string) map[string]int {
	ensureKindExtensionIndex()
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil
	}
	return kindExtIndexMemo[kind]
}
