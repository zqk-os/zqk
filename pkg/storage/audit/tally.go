package audit

import "github.com/zqk-os/zqk/pkg/objects"

// EventTally is the count rollup AggregateAuditEvents writes onto a metric.
type EventTally struct {
	EventCount       int
	EventIDs         []string
	EventTypeCounts  map[string]int
	StatusCounts     map[string]int
	ObjectKindCounts map[string]int
	OperationCounts  map[string]int
	ErrorEventCount  int
}

// TallyEvents counts type/status/kind/operation and collects event IDs.
func TallyEvents(events []map[string]any) EventTally {
	out := EventTally{
		EventCount:       len(events),
		EventIDs:         make([]string, 0, len(events)),
		EventTypeCounts:  make(map[string]int),
		StatusCounts:     make(map[string]int),
		ObjectKindCounts: make(map[string]int),
		OperationCounts:  make(map[string]int),
	}
	for _, event := range events {
		if id := objects.GetString(event, objects.FieldKeyID); id != "" {
			out.EventIDs = append(out.EventIDs, id)
		}
		if eventType := objects.GetString(event, objects.FieldKeyEventType); eventType != "" {
			out.EventTypeCounts[eventType]++
		}
		if status := objects.GetString(event, objects.FieldKeyStatus); status != "" {
			out.StatusCounts[status]++
			if IsErrorStatus(status) {
				out.ErrorEventCount++
			}
		}
		if objectKind := objects.GetString(event, objects.FieldKeyTargetKind); objectKind != "" {
			out.ObjectKindCounts[objectKind]++
		}
		if operation := objects.GetString(event, objects.FieldKeyOperation); operation != "" {
			out.OperationCounts[operation]++
		}
	}
	return out
}

// ErrorRate is error events over EventCount. Zero when EventCount is 0.
func (t EventTally) ErrorRate() float64 {
	if t.EventCount <= 0 {
		return 0
	}
	return float64(t.ErrorEventCount) / float64(t.EventCount)
}

// EventTypeCountsAny is the builder map. Spec requires minCount 1, so an empty
// tally gets EventTypeCountKeyAggregated = EventCount.
func (t EventTally) EventTypeCountsAny() map[string]any {
	out := countsAny(t.EventTypeCounts)
	if len(out) == 0 {
		out[EventTypeCountKeyAggregated] = t.EventCount
	}
	return out
}

func (t EventTally) ObjectKindCountsAny() map[string]any {
	return countsAny(t.ObjectKindCounts)
}

func (t EventTally) OperationCountsAny() map[string]any {
	return countsAny(t.OperationCounts)
}

func (t EventTally) StatusCountsAny() map[string]any {
	return countsAny(t.StatusCounts)
}

func countsAny(m map[string]int) map[string]any {
	out := make(map[string]any, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}
