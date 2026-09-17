package system

import (
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/objects"
)

// LifecyclePreconditionPattern defines a pattern for matching lifecycle precondition violations
type LifecyclePreconditionPattern struct {
	// MessagePatterns are substrings to match in the validation error message
	MessagePatterns []string
	// FieldName is the field that needs to be set (e.g., objects.FieldKeyMilestoneRefs, objects.FieldKeyPriorityPlanRef)
	FieldName string
	// TargetKind is the kind of object to reference (e.g., "milestone", "priority_plan")
	TargetKind string
	// Operation is the operation to perform: "append" (for array fields) or "set" (for single value fields)
	Operation string // "append" or "set"
	// PlaceholderID is the placeholder ID to use in the fix command (e.g., "MILESTONE_ID", "PRIORITY_PLAN_ID")
	PlaceholderID string
}

var (
	// Cached lifecycle precondition patterns
	lifecyclePreconditionPatternsCache     []LifecyclePreconditionPattern
	lifecyclePreconditionPatternsCacheOnce sync.Once
)

// getLifecyclePreconditionPatterns returns the list of lifecycle precondition patterns
// Dynamically built and cached for performance
func getLifecyclePreconditionPatterns() []LifecyclePreconditionPattern {
	lifecyclePreconditionPatternsCacheOnce.Do(func() {
		// Build patterns array dynamically
		// This can be extended to load from config/spec in the future
		lifecyclePreconditionPatternsCache = []LifecyclePreconditionPattern{
			{
				MessagePatterns: []string{
					"milestone_ref is set",
					"milestone_refs is not empty",
					"at least one milestone_ref",
					"At least one milestone_ref linked",
				},
				FieldName:     objects.FieldKeyMilestoneRef,
				TargetKind:    objects.KindMilestone,
				Operation:     "set",
				PlaceholderID: "MILESTONE_ID",
			},
			{
				MessagePatterns: []string{
					"priority_plan_ref is set",
				},
				FieldName:     objects.FieldKeyPriorityPlanRef,
				TargetKind:    objects.KindPriorityPlan,
				Operation:     "set",
				PlaceholderID: "PRIORITY_PLAN_ID",
			},
			{
				MessagePatterns: []string{
					"workstream_refs is not empty",
					"at least one workstream_ref",
				},
				FieldName:     objects.FieldKeyWorkstreamRefs,
				TargetKind:    objects.KindWorkstream,
				Operation:     "append",
				PlaceholderID: "WORKSTREAM_ID",
			},
			{
				MessagePatterns: []string{
					"goal_refs is not empty",
					"at least one goal_ref",
				},
				FieldName:     objects.FieldKeyGoalRefs,
				TargetKind:    objects.KindGoal,
				Operation:     "append",
				PlaceholderID: "GOAL_ID",
			},
		}
	})

	return lifecyclePreconditionPatternsCache
}

// matchLifecyclePreconditionPattern matches a validation message against lifecycle precondition patterns
// Returns the matched pattern and true if a match is found, nil and false otherwise
func matchLifecyclePreconditionPattern(message string) (*LifecyclePreconditionPattern, bool) {
	patterns := getLifecyclePreconditionPatterns()
	messageLower := strings.ToLower(message)

	for i := range patterns {
		pattern := &patterns[i]
		for _, msgPattern := range pattern.MessagePatterns {
			if strings.Contains(messageLower, strings.ToLower(msgPattern)) {
				return pattern, true
			}
		}
	}

	return nil, false
}
