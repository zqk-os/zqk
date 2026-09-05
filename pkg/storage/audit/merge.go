package audit

import (
	"fmt"
	"maps"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

const (
	EventTypeCountKeyAggregated = "aggregated"
	MergeKeyEventCount          = "event_count"
	MergeKeyEventTypeCounts     = "event_type_counts"
	MergeKeyStatusCounts        = "status_counts"
	MergeKeyObjectKindCounts    = "object_kind_counts"
	MergeKeyOperationCounts     = "operation_counts"
	MergeKeyAggregatedEventIDs  = "aggregated_event_ids"
	MergeKeyCollectionCount     = "collection_count"
	MergeKeyLastSeen            = "last_seen"
	MergeKeyErrorEventCount     = "error_event_count"
	MergeKeyErrorRate           = "error_rate"
	MergeKeyTitle               = "title"
	TitleFmt                    = "Audit Event Aggregation: %d events from %s to %s"
	Prefix                      = "AUD-"
	RangeSeparator              = ".."
)

// ErrorStatuses are counted as errors for error_rate.
var ErrorStatuses = map[string]struct{}{
	objects.FieldKeyFailed: {},
	"reverted":             {},
	"error":                {},
}

func IsErrorStatus(status string) bool {
	_, ok := ErrorStatuses[strings.TrimSpace(strings.ToLower(status))]
	return ok
}

func MapFromAnyToInt(m map[string]any) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = IntFromAny(v)
	}
	return out
}

func IntFromAny(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func MapIntToAny(m map[string]int) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// MergeMetrics combines two aggregation-metric maps (counts, IDs, error rate, title).
func MergeMetrics(prefix, separator string, metric1, metric2 map[string]any) map[string]any {
	merged := make(map[string]any)
	maps.Copy(merged, metric1)

	count1, _ := metric1[MergeKeyEventCount].(int)
	count2, _ := metric2[MergeKeyEventCount].(int)
	merged[MergeKeyEventCount] = count1 + count2

	typeCounts1Any, _ := metric1[MergeKeyEventTypeCounts].(map[string]any)
	typeCounts2Any, _ := metric2[MergeKeyEventTypeCounts].(map[string]any)
	typeCounts1 := MapFromAnyToInt(typeCounts1Any)
	typeCounts2 := MapFromAnyToInt(typeCounts2Any)
	mergedTypeCounts := make(map[string]int)
	maps.Copy(mergedTypeCounts, typeCounts1)
	for k, v := range typeCounts2 {
		mergedTypeCounts[k] += v
	}
	if len(mergedTypeCounts) == 0 {
		mergedTypeCounts[EventTypeCountKeyAggregated] = count1 + count2
	}
	merged[MergeKeyEventTypeCounts] = MapIntToAny(mergedTypeCounts)

	statusCounts1Any, _ := metric1[MergeKeyStatusCounts].(map[string]any)
	statusCounts2Any, _ := metric2[MergeKeyStatusCounts].(map[string]any)
	statusCounts1 := MapFromAnyToInt(statusCounts1Any)
	statusCounts2 := MapFromAnyToInt(statusCounts2Any)
	if statusCounts1 != nil || statusCounts2 != nil {
		mergedStatusCounts := make(map[string]int)
		maps.Copy(mergedStatusCounts, statusCounts1)
		for k, v := range statusCounts2 {
			mergedStatusCounts[k] += v
		}
		merged[MergeKeyStatusCounts] = MapIntToAny(mergedStatusCounts)
	}

	kindCounts1, _ := metric1[MergeKeyObjectKindCounts].(map[string]int)
	kindCounts2, _ := metric2[MergeKeyObjectKindCounts].(map[string]int)
	if kindCounts1 != nil || kindCounts2 != nil {
		if kindCounts1 == nil {
			kindCounts1 = make(map[string]int)
		}
		if kindCounts2 == nil {
			kindCounts2 = make(map[string]int)
		}
		mergedKindCounts := make(map[string]int)
		maps.Copy(mergedKindCounts, kindCounts1)
		for k, v := range kindCounts2 {
			mergedKindCounts[k] += v
		}
		merged[MergeKeyObjectKindCounts] = mergedKindCounts
	}

	opCounts1, _ := metric1[MergeKeyOperationCounts].(map[string]int)
	opCounts2, _ := metric2[MergeKeyOperationCounts].(map[string]int)
	if opCounts1 != nil || opCounts2 != nil {
		if opCounts1 == nil {
			opCounts1 = make(map[string]int)
		}
		if opCounts2 == nil {
			opCounts2 = make(map[string]int)
		}
		mergedOpCounts := make(map[string]int)
		maps.Copy(mergedOpCounts, opCounts1)
		for k, v := range opCounts2 {
			mergedOpCounts[k] += v
		}
		merged[MergeKeyOperationCounts] = mergedOpCounts
	}

	ids1, _ := metric1[MergeKeyAggregatedEventIDs].([]string)
	ids2, _ := metric2[MergeKeyAggregatedEventIDs].([]string)
	allIDs := make([]string, 0, len(ids1)+len(ids2))
	allIDs = append(allIDs, ids1...)
	allIDs = append(allIDs, ids2...)
	expandedIDs := ExpandIDRanges(prefix, separator, allIDs)
	eventsForCompression := make([]map[string]any, len(expandedIDs))
	for i, id := range expandedIDs {
		eventsForCompression[i] = map[string]any{objects.FieldKeyID: id}
	}
	merged[MergeKeyAggregatedEventIDs] = CompressEventIDs(prefix, separator, eventsForCompression)

	collectionCount1, _ := metric1[MergeKeyCollectionCount].(int)
	collectionCount2, _ := metric2[MergeKeyCollectionCount].(int)
	merged[MergeKeyCollectionCount] = collectionCount1 + collectionCount2

	lastSeen1, _ := metric1[MergeKeyLastSeen].(string)
	lastSeen2, _ := metric2[MergeKeyLastSeen].(string)
	if lastSeen2 > lastSeen1 {
		merged[MergeKeyLastSeen] = lastSeen2
	}

	errorCount := 0
	if statusCountsAny, ok := merged[MergeKeyStatusCounts].(map[string]any); ok {
		for status, countAny := range statusCountsAny {
			if IsErrorStatus(status) {
				errorCount += IntFromAny(countAny)
			}
		}
	}
	merged[MergeKeyErrorEventCount] = errorCount
	if eventCount, ok := merged[MergeKeyEventCount].(int); ok && eventCount > 0 {
		merged[MergeKeyErrorRate] = float64(errorCount) / float64(eventCount)
	} else {
		merged[MergeKeyErrorRate] = float64(0)
	}

	eventCount, _ := merged[MergeKeyEventCount].(int)
	windowStart, _ := merged[objects.FieldKeyAggregationWindowStart].(string)
	windowEnd, _ := merged[objects.FieldKeyAggregationWindowEnd].(string)
	merged[MergeKeyTitle] = fmt.Sprintf(TitleFmt, eventCount, windowStart, windowEnd)

	return merged
}

// AsMetricMap type-asserts a batch-processor result to a metric map.
func AsMetricMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// MergeAnyMetrics merges two batch results. False types return nil, false.
func MergeAnyMetrics(prefix, separator string, first, second any) (map[string]any, bool) {
	a, ok1 := AsMetricMap(first)
	b, ok2 := AsMetricMap(second)
	if !ok1 || !ok2 {
		return nil, false
	}
	return MergeMetrics(prefix, separator, a, b), true
}
