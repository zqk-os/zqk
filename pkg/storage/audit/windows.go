package audit

import (
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// WindowsOverlap reports whether [start1, end1) overlaps [start2, end2).
func WindowsOverlap(start1, end1, start2, end2 time.Time) bool {
	return start1.Before(end2) && start2.Before(end1)
}

// FirstOverlappingID returns the first object id whose aggregation window
// overlaps [windowStart, windowEnd). Empty when none match or times are invalid.
func FirstOverlappingID(objs []map[string]any, windowStart, windowEnd time.Time) string {
	for _, obj := range objs {
		objStartStr, _ := obj[objects.FieldKeyAggregationWindowStart].(string)
		objEndStr, _ := obj[objects.FieldKeyAggregationWindowEnd].(string)
		if objStartStr == "" || objEndStr == "" {
			continue
		}
		objStart, err1 := time.Parse(time.RFC3339, objStartStr)
		objEnd, err2 := time.Parse(time.RFC3339, objEndStr)
		if err1 != nil || err2 != nil {
			continue
		}
		if WindowsOverlap(windowStart, windowEnd, objStart, objEnd) {
			if id := objects.GetString(obj, objects.FieldKeyID); id != "" {
				return id
			}
		}
	}
	return ""
}
