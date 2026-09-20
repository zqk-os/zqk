package storage

import (
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// kindPromptTemplate matches ontology spelling; no objects.Kind* alias yet in this repo.
const kindPromptTemplate = "prompt_template"

// workstreamCategoryComponent is the workstream category enum value "component" (not objects.KindComponent).
const workstreamCategoryComponent = "component"

const (
	compTestStatusDraft      = "draft"
	testStatusComplete       = "complete"
	compTestStatusActive     = "active"
	testStatusNotStarted     = "not_started"
	compTestStatusExploring  = "exploring"
	testStatusInProgress     = "in_progress"
	testStatusArchived       = "archived"
	testStatusRejected       = "rejected"
	testStatusProposed       = "proposed"
	testStatusCreated        = "created"
	testStatusValidated      = "validated"
	testStatusDeferred       = "deferred"
	testCategoryA            = "CategoryA"
	testCategoryB            = "CategoryB"
	testCategoryPerf         = "performance"
	testDomainApplication    = "application"
	testDomainFeature        = "feature"
	testDomainOps            = "ops"
	testDomainTooling        = "tooling"
	testDomainSystem         = "system"
	testTimestampStart       = "2025-01-01T00:00:00Z"
	testTimestampEnd         = "2025-01-02T00:00:00Z"
	testMetricTypeSystem     = "system"
	testMetricTypeCommand    = "command"
	testFieldLow             = "low"
	compTestChangeTypeCreate = "create"
	testDomainVisualization  = "visualization"
	testSpecInterpreter      = "test_interpreter"
	testSpecBroker           = "test_broker"
	testStakeholderFmt       = ConstStreamStakeholderInt
	testCommandFmt           = "test_command_%d"
)

// Helper functions

func hasField(kindFields *objects.KindFields, fieldName string) bool {
	for i := range kindFields.AllFields {
		field := kindFields.AllFields[i]
		if field.Name == fieldName {
			return true
		}
	}
	return false
}

func getFieldValue(obj map[string]any, field string) any {
	return obj[field]
}

func verifySortOrder(t *testing.T, objList []map[string]any, field string, ascending bool) {
	if len(objList) < 2 {
		return // Can't verify sort with less than 2 items
	}

	for i := 0; i < len(objList)-1; i++ {
		val1 := getFieldValue(objList[i], field)
		val2 := getFieldValue(objList[i+1], field)

		if val1 == nil || val2 == nil {
			continue // Skip nil values
		}

		comparison := compareValues(val1, val2)
		if ascending {
			if comparison > 0 {
				t.Errorf(ConstStreamSortOrderViolationValValAtPositionsIntInt, val1, val2, i, i+1)
			}
		} else {
			if comparison < 0 {
				t.Errorf(ConstStreamSortOrderViolationValValAtPositionsIntInt2, val1, val2, i, i+1)
			}
		}
	}
}

func verifyGrouping(t *testing.T, groups map[string][]map[string]any, groupField string) {
	for groupKey, groupObjs := range groups {
		for _, obj := range groupObjs {
			objValue := getFieldValue(obj, groupField)
			// Handle nil values - they should be grouped under empty string key (as per groupObjects implementation)
			if objValue == nil {
				if groupKey != emptyValue {
					t.Errorf(ConstStreamGroupingViolationObjectValHasStrNilButIsInGroup, obj[objects.FieldKeyID], groupField, groupKey)
				}
				continue
			}
			// Handle list/slice values - groupObjects uses fmt.Sprintf("%v", val) for group keys
			objValueStr := fmt.Sprintf("%v", objValue)
			// For lists, the groupKey is the string representation of the entire list
			if objValueStr != groupKey {
				// For list fields, the group key might be formatted as "[item1 item2]" or similar
				// Check if it's a list and if the string representation matches
				if listVal, ok := objValue.([]any); ok {
					// Try different string representations
					// The groupObjects function uses fmt.Sprintf("%v", val) which for slices gives "[item1 item2]"
					// But we need to check if the actual list matches
					listStr := fmt.Sprintf("%v", listVal)
					if listStr != groupKey {
						// For list grouping, each unique list gets its own group
						// So if the string representations don't match, it's a violation
						t.Errorf(ConstStreamGroupingViolationObjectValHasStrValStrStrButIsIn, obj[objects.FieldKeyID], groupField, objValue, listStr, groupKey)
					}
				} else {
					t.Errorf(ConstStreamGroupingViolationObjectValHasStrValStrStrButIsIn, obj[objects.FieldKeyID], groupField, objValue, objValueStr, groupKey)
				}
			}
		}
	}
}

func compareValues(a, b any) int {
	// Simple comparison for strings and numbers
	switch aVal := a.(type) {
	case string:
		bVal, ok := b.(string)
		if !ok {
			return 0
		}
		if aVal < bVal {
			return -1
		} else if aVal > bVal {
			return 1
		}
		return 0
	case int:
		bVal, ok := b.(int)
		if !ok {
			return 0
		}
		if aVal < bVal {
			return -1
		} else if aVal > bVal {
			return 1
		}
		return 0
	case float64:
		bVal, ok := b.(float64)
		if !ok {
			return 0
		}
		if aVal < bVal {
			return -1
		} else if aVal > bVal {
			return 1
		}
		return 0
	}
	return 0
}

func minTwo(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// initializeStatusCache loads initial statuses from lifecycle definitions
// This should be called during test initialization to establish fallback cache
// Lifecycles and built-in objects may have conflicting statuses because built-ins
// are immutable without admin intervention
func setKindSpecificFields(obj map[string]any, kind string, index int) {
	switch kind {
	case objects.KindGoal:
		obj[objects.FieldKeyTarget] = fmt.Sprintf("%d", 100+index*10)
		obj[objects.FieldKeyMetric] = "count"
	case objects.KindMilestone:
		// Milestone-specific fields if needed
	case objects.KindRequirement:
		// Requirement-specific fields if needed
	case objects.KindAccount:
		// Account requires username field
		obj[objects.FieldKeyUsername] = fmt.Sprintf("testuser%d", index+1)
		// Add optional fields from base_object for grouping tests
		// priority_tier enum: P0, P1, P2, P3 (from base_object spec)
		priorityTiers := []string{"P0", "P1", "P2", "P3"}
		obj[objects.FieldKeyPriorityTier] = priorityTiers[index%len(priorityTiers)]
		// stakeholders is a list field
		obj[objects.FieldKeyStakeholders] = []string{fmt.Sprintf(ConstStreamStakeholderInt, index+1)}
	case objects.KindAuditEvent:
		// Audit event requires operation and event_type
		obj[objects.FieldKeyOperation] = fmt.Sprintf(ConstStreamTestOperationInt, index+1)
		obj[objects.FieldKeyEventType] = ConstStreamSystemConfigChange // Use valid enum value
	case objects.KindAuditAggregationMetric:
		// Audit aggregation metric requires many fields
		obj[objects.FieldKeyMetricType] = testMetricTypeSystem
		obj[objects.FieldKeyAggregationWindowStart] = testTimestampStart
		obj[objects.FieldKeyAggregationWindowEnd] = testTimestampEnd
		obj[objects.FieldKeyFirstSeen] = testTimestampStart
		obj[objects.FieldKeyLastSeen] = testTimestampEnd
		obj[objects.FieldKeyCollectionCount] = 1.0
		obj[objects.FieldKeyEventCount] = 10.0
		obj[objects.FieldKeyEventTypeCounts] = map[string]any{"create": 5, "update": 5}
	case objects.KindBaseMetric:
		// Base metric requires metric_type and timestamps
		obj[objects.FieldKeyMetricType] = testCategoryPerf // Use valid enum value
		obj[objects.FieldKeyFirstSeen] = testTimestampStart
		obj[objects.FieldKeyLastSeen] = testTimestampEnd
		obj[objects.FieldKeyCollectionCount] = 1.0
	case objects.KindChangeJournalEntry:
		// Change journal entry requires object_ref and change_type
		// Note: Referenced goals need to be created separately - this test will skip if goals don't exist
		obj[objects.FieldKeyObjectRef] = fmt.Sprintf("GOAL-%03d", index+1)
		obj[objects.FieldKeyChangeType] = compTestChangeTypeCreate
	case objects.KindCodeReference:
		// Code reference requires file_path and line_start
		obj[objects.FieldKeyFilePath] = fmt.Sprintf("test/file_%d.go", index+1)
		obj[objects.FieldKeyLineStart] = 1.0
	case objects.KindCommandMetric:
		// Command metric requires many fields
		obj[objects.FieldKeyCommand] = fmt.Sprintf(testCommandFmt, index+1)
		obj[objects.FieldKeyNormalizedCmd] = fmt.Sprintf(testCommandFmt, index+1)
		obj[objects.FieldKeyMetricType] = testMetricTypeCommand
		obj[objects.FieldKeyInvocationCount] = 1.0
		obj[objects.FieldKeySuccessCount] = 1.0
		obj[objects.FieldKeyFailureCount] = 0.0
		obj[objects.FieldKeyTimeoutCount] = 0.0
		obj[objects.FieldKeyAvgDurationSeconds] = 0.5
		obj[objects.FieldKeyFastestDurationSeconds] = 0.1
		obj[objects.FieldKeySlowestDurationSeconds] = 1.0
		obj[objects.FieldKeyBaselineDurationSeconds] = 0.5
		obj[objects.FieldKeyErrorRate] = 0.0
		obj[objects.FieldKeyTimeoutRate] = 0.0
		obj[objects.FieldKeyFirstSeen] = testTimestampStart
		obj[objects.FieldKeyLastSeen] = testTimestampEnd
		obj[objects.FieldKeyCollectionCount] = 1.0
	case objects.KindComponent:
		// Component requires spec_interpreter, spec_context_broker, component_type, domain
		// domain enum: visualization, ui, api, integration, custom
		validDomains := []string{testDomainVisualization, "ui", "api", "integration", "custom"}
		obj[objects.FieldKeySpecInterpreter] = testSpecInterpreter
		obj[objects.FieldKeySpecContextBroker] = testSpecBroker
		obj[objects.FieldKeyComponentType] = "test_type"
		obj[objects.FieldKeyDomain] = validDomains[index%len(validDomains)]
	case objects.KindContextRefreshSchedule:
		// Context refresh schedule requires target and cadence
		obj[objects.FieldKeyTarget] = "test_target"
		obj[objects.FieldKeyCadence] = "daily"
	case objects.KindWorkstream:
		// Workstream requires owner_ref, entry_point, and category (with enum)
		obj[objects.FieldKeyOwnerRef] = ConstStreamAccountAcc001
		obj[objects.FieldKeyEntryPoint] = fmt.Sprintf("docs/workstreams/test_%d.md", index+1)
		// category enum: application, system, feature, component, ops, tooling
		validCategories := []string{testDomainApplication, testDomainSystem, testDomainFeature, workstreamCategoryComponent, testDomainOps, testDomainTooling}
		obj[objects.FieldKeyCategory] = validCategories[index%len(validCategories)]
		// Add optional fields for grouping tests
		// Note: workstream priority_tier should use P0-P3 format from base_object, but workstream might override it
		// For now, use generic values that work for grouping tests
		workstreamPriorityTiers := []string{"P0", "P1", "P2", "P3"}
		obj[objects.FieldKeyPriorityTier] = workstreamPriorityTiers[index%len(workstreamPriorityTiers)]
		stageTypes := []string{"planning", "development", "testing", "deployment", "maintenance"}
		obj[objects.FieldKeyStageType] = stageTypes[index%len(stageTypes)]
		obj[objects.FieldKeyStakeholders] = []string{fmt.Sprintf(testStakeholderFmt, index+1)}
	case objects.KindDisplay:
		// Display requires domain, display_type, spec_interpreter, spec_context_broker
		obj[objects.FieldKeyDomain] = testDomainVisualization
		obj[objects.FieldKeyDisplayType] = "dashboard"
		obj[objects.FieldKeySpecInterpreter] = testSpecInterpreter
		obj[objects.FieldKeySpecContextBroker] = testSpecBroker
	}
}
