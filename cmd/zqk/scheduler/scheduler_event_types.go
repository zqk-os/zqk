package scheduler

import (
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	schedulerJobEventPrefix    = "scheduler_job_"
	schedulerJobStartedEvent   = "scheduler_job_started"
	schedulerJobCompletedEvent = "scheduler_job_completed"
	schedulerJobFailedEvent    = "scheduler_job_failed"
)

var (
	// Cached scheduler job event types (loaded from audit_event spec)
	schedulerEventTypesCache      []string
	schedulerEventTypesCacheMap   map[string]bool
	schedulerEventTypesCacheOnce  sync.Once
	schedulerEventTypesCacheError error
)

// getSchedulerJobEventTypes returns the list of scheduler job event types
// Dynamically loaded from the audit_event spec and cached for performance
// Filters event types that start with "scheduler_job_"
func getSchedulerJobEventTypes() ([]string, error) {
	schedulerEventTypesCacheOnce.Do(func() {
		// Load audit_event spec
		specLoader := objects.GetGlobalSpecLoader()
		spec, err := specLoader.LoadSpecWithInheritance("audit_event.yaml")
		if err != nil {
			schedulerEventTypesCacheError = err
			// Fallback to known values if spec can't be loaded
			schedulerEventTypesCache = []string{
				schedulerJobStartedEvent,
				schedulerJobCompletedEvent,
				schedulerJobFailedEvent,
			}
			schedulerEventTypesCacheMap = map[string]bool{
				schedulerJobStartedEvent:   true,
				schedulerJobCompletedEvent: true,
				schedulerJobFailedEvent:    true,
			}
			return
		}

		// Extract event_type enum values from spec
		var eventTypes []string
		if spec != nil && spec.ResolvedFields != nil {
			if eventTypeField, ok := spec.ResolvedFields[getAuditEventEventTypeField()].(map[string]any); ok {
				if validation, ok := eventTypeField["validation"].(map[string]any); ok {
					if enumValues, ok := validation["enum"].([]any); ok {
						// Filter for scheduler job event types by prefix.
						for _, enumVal := range enumValues {
							if eventType, ok := enumVal.(string); ok {
								if strings.HasPrefix(eventType, schedulerJobEventPrefix) {
									eventTypes = append(eventTypes, eventType)
								}
							}
						}
					}
				}
			}
		}

		// If no event types found, fallback to known values
		if len(eventTypes) == 0 {
			eventTypes = []string{
				schedulerJobStartedEvent,
				schedulerJobCompletedEvent,
				schedulerJobFailedEvent,
			}
		}

		// Build cache
		schedulerEventTypesCache = eventTypes
		schedulerEventTypesCacheMap = make(map[string]bool, len(eventTypes))
		for _, eventType := range eventTypes {
			schedulerEventTypesCacheMap[eventType] = true
		}
	})

	return schedulerEventTypesCache, schedulerEventTypesCacheError
}

// getSchedulerJobEventTypesMap returns a map of scheduler job event types for fast lookup
// Dynamically loaded from the audit_event spec and cached for performance
func getSchedulerJobEventTypesMap() (map[string]bool, error) {
	_, err := getSchedulerJobEventTypes()
	if err != nil {
		return nil, err
	}
	return schedulerEventTypesCacheMap, nil
}

// isSchedulerJobEventType checks if an event type is a scheduler job event type
// Uses cached lookup for performance
func isSchedulerJobEventType(eventType string) bool {
	eventTypesMap, err := getSchedulerJobEventTypesMap()
	if err != nil {
		return false
	}
	return eventTypesMap[eventType]
}

// isSchedulerJobCompletionEventType checks if an event type is a completion event (completed or failed)
// Uses cached lookup for performance
func isSchedulerJobCompletionEventType(eventType string) bool {
	return isSchedulerJobCompletedEventType(eventType) || isSchedulerJobFailedEventType(eventType)
}

// getSchedulerJobEventTypeCategory returns the category/outcome for a scheduler job event type
// Returns: "started", "completed", "failed", or empty string if not a scheduler job event
func getSchedulerJobEventTypeCategory(eventType string) string {
	if !isSchedulerJobEventType(eventType) {
		return ""
	}
	// Extract category from event type (e.g., "scheduler_job_started" -> "started")
	if strings.HasPrefix(eventType, schedulerJobEventPrefix) {
		return strings.TrimPrefix(eventType, schedulerJobEventPrefix)
	}
	return ""
}

// isSchedulerJobStartedEventType checks if an event type is a started event
// Uses cached lookup for performance
func isSchedulerJobStartedEventType(eventType string) bool {
	if !isSchedulerJobEventType(eventType) {
		return false
	}
	category := getSchedulerJobEventTypeCategory(eventType)
	return category == schedulerStateStarted
}

// isSchedulerJobCompletedEventType checks if an event type is a completed event
// Uses cached lookup for performance
func isSchedulerJobCompletedEventType(eventType string) bool {
	if !isSchedulerJobEventType(eventType) {
		return false
	}
	category := getSchedulerJobEventTypeCategory(eventType)
	return category == schedulerStateCompleted
}

// isSchedulerJobFailedEventType checks if an event type is a failed event
// Uses cached lookup for performance
func isSchedulerJobFailedEventType(eventType string) bool {
	if !isSchedulerJobEventType(eventType) {
		return false
	}
	category := getSchedulerJobEventTypeCategory(eventType)
	return category == schedulerStateFailed
}
