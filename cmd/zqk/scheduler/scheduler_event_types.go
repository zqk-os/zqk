package scheduler

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

const (
	schedulerJobEventPrefix    = "scheduler_job_"
	schedulerJobStartedEvent   = "scheduler_job_started"
	schedulerJobCompletedEvent = "scheduler_job_completed"
	schedulerJobFailedEvent    = "scheduler_job_failed"
)

type schedulerJobEventTypeInfo struct {
	list []string
	set  map[string]bool
	err  error
}

var schedulerJobEventTypes stampmemo.Table[schedulerJobEventTypeInfo]

func auditEventSpecPath() string {
	dir := paths.FirstExistingFromCwd(paths.ProcessInternalObjectSpecsDir)
	return paths.FindDomainFile(dir, paths.ObjectSpecFileName("audit_event"))
}

func loadSchedulerJobEventTypes() schedulerJobEventTypeInfo {
	fallback := schedulerJobEventTypeInfo{
		list: []string{
			schedulerJobStartedEvent,
			schedulerJobCompletedEvent,
			schedulerJobFailedEvent,
		},
		set: map[string]bool{
			schedulerJobStartedEvent:   true,
			schedulerJobCompletedEvent: true,
			schedulerJobFailedEvent:    true,
		},
	}

	specLoader := objects.GetGlobalSpecLoader()
	spec, err := specLoader.LoadSpecWithInheritance("audit_event.yaml")
	if err != nil {
		fallback.err = err
		return fallback
	}

	var eventTypes []string
	if spec != nil && spec.ResolvedFields != nil {
		if eventTypeField, ok := spec.ResolvedFields[getAuditEventEventTypeField()].(map[string]any); ok {
			if validation, ok := eventTypeField["validation"].(map[string]any); ok {
				if enumValues, ok := validation["enum"].([]any); ok {
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
	if len(eventTypes) == 0 {
		return fallback
	}
	set := make(map[string]bool, len(eventTypes))
	for _, eventType := range eventTypes {
		set[eventType] = true
	}
	return schedulerJobEventTypeInfo{list: eventTypes, set: set}
}

func getSchedulerJobEventTypeInfo() (schedulerJobEventTypeInfo, error) {
	path := auditEventSpecPath()
	info, err := schedulerJobEventTypes.Load(path, stampmemo.Of(path), func() (schedulerJobEventTypeInfo, error) {
		loaded := loadSchedulerJobEventTypes()
		return loaded, loaded.err
	})
	if err != nil {
		return info, err
	}
	return info, info.err
}

func getSchedulerJobEventTypes() ([]string, error) {
	info, err := getSchedulerJobEventTypeInfo()
	return info.list, err
}

func getSchedulerJobEventTypesMap() (map[string]bool, error) {
	info, err := getSchedulerJobEventTypeInfo()
	if err != nil {
		return nil, err
	}
	return info.set, nil
}

func isSchedulerJobEventType(eventType string) bool {
	eventTypesMap, err := getSchedulerJobEventTypesMap()
	if err != nil {
		return false
	}
	return eventTypesMap[eventType]
}

func isSchedulerJobCompletionEventType(eventType string) bool {
	return isSchedulerJobCompletedEventType(eventType) || isSchedulerJobFailedEventType(eventType)
}

func getSchedulerJobEventTypeCategory(eventType string) string {
	if !isSchedulerJobEventType(eventType) {
		return ""
	}
	if strings.HasPrefix(eventType, schedulerJobEventPrefix) {
		return strings.TrimPrefix(eventType, schedulerJobEventPrefix)
	}
	return ""
}

func isSchedulerJobStartedEventType(eventType string) bool {
	if !isSchedulerJobEventType(eventType) {
		return false
	}
	category := getSchedulerJobEventTypeCategory(eventType)
	return category == schedulerStateStarted
}

func isSchedulerJobCompletedEventType(eventType string) bool {
	if !isSchedulerJobEventType(eventType) {
		return false
	}
	category := getSchedulerJobEventTypeCategory(eventType)
	return category == schedulerStateCompleted
}

func isSchedulerJobFailedEventType(eventType string) bool {
	if !isSchedulerJobEventType(eventType) {
		return false
	}
	category := getSchedulerJobEventTypeCategory(eventType)
	return category == schedulerStateFailed
}
