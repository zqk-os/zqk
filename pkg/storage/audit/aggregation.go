package audit

import (
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// Group is a set of events that will be aggregated together.
type Group struct {
	Key          string
	EventType    string
	TargetKind   string
	Severity     string
	Count        int
	FirstSeen    time.Time
	LastSeen     time.Time
	SampleEvents []map[string]any
	Events       []map[string]any
}

// Rule defines which events should be aggregated.
type Rule struct {
	EventTypes      []string      `yaml:"event_types"`
	Severities      []string      `yaml:"severities"`
	GroupBy         []string      `yaml:"group_by"`
	Window          time.Duration `yaml:"window"`
	Threshold       int           `yaml:"threshold"`
	PreserveSamples int           `yaml:"preserve_samples"`
}

// DefaultRules returns default aggregation rules per the audit design.
func DefaultRules() []Rule {
	return []Rule{
		{
			EventTypes:      []string{EventTypeCacheInvalidation, EventTypeCacheUpdate},
			Severities:      []string{SeverityLow},
			GroupBy:         []string{objects.FieldKeyEventType, objects.FieldKeyTargetKind},
			Window:          time.Hour,
			Threshold:       10,
			PreserveSamples: 5,
		},
		{
			EventTypes:      []string{EventTypeCacheBulkInvalidation},
			Severities:      []string{SeverityLow, SeverityMedium},
			GroupBy:         []string{objects.FieldKeyEventType, objects.FieldKeyTargetKind},
			Window:          time.Hour,
			Threshold:       5,
			PreserveSamples: 3,
		},
	}
}
