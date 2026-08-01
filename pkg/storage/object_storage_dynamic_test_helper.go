package storage

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// kindCorporateInitiative matches ontology spelling; no objects.Kind* exported for this kind yet.
const kindCorporateInitiative = "corporate_initiative"

const (
	testStatusExploring  = "exploring"
	testStatusDraft      = "draft"
	testStatusOpen       = "open"
	testStatusPending    = objects.ObjectStatusPending
	testStatusPlanned    = objects.ObjectStatusPlanned
	testStatusActive     = objects.ObjectStatusActive
	testItemPrefix       = "item"
	testItemFmt          = "item%d"
	testValueDefault     = "test_value"
	testValueNumbered    = "test_value_123"
	testDateTimeStart    = "2025-01-01T00:00:00Z"
	testDateTimeEnd      = "2025-01-02T00:00:00Z"
	testDateTimeFixed    = "2025-12-26T00:00:00Z"
	testDateFixed        = "2025-12-26"
	testMetricTypePerf   = "performance"
	testChangeTypeCreate = "create"
)

// Helper functions

func getFieldNames(fields map[string]any) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names
}

// generateTestID generates a test ID in the correct format for a given kind
func generateTestID(kind string) string {
	// Map kinds to their ID prefixes (from id_validator.go)
	kindPrefixes := map[string]string{
		objects.KindBacklogItem:            "BLI",
		objects.KindGoal:                   "GOAL",
		objects.KindMilestone:              "MIL",
		objects.KindWorkstream:             "WS",
		objects.KindPriorityPlan:           "PRIO",
		objects.KindCriteria:               "CRIT",
		objects.KindRequirement:            "REQ",
		objects.KindRoadmap:                "ROAD",
		objects.KindDecision:               "DEC",
		objects.KindAccount:                "ACC",
		objects.KindAuditAggregationMetric: "AAM",
		objects.KindCommandMetric:          "CMD",
		objects.KindAuditEvent:             "AUD",
		objects.KindBaseMetric:             "BAS",
		objects.KindCodeReference:          "COD",
		objects.KindContextRefreshSchedule: "CON",
		kindCorporateInitiative:            "CI",
		objects.KindDisplay:                "DSP",
		objects.KindDocEntry:               "DOC",
		objects.KindExtensibleObject:       "EXT",
		objects.KindIntegrityManifest:      "INT",
		objects.KindMetadataPackage:        "MET",
		objects.KindMission:                "MIS",
		objects.KindPersona:                "PER",
		objects.KindQuestion:               "QUE",
		objects.KindRelease:                "REL",
		objects.KindResolver:               "RES",
		objects.KindRiskBlocker:            "RIS",
		objects.KindRollbackReport:         "RBR",
		objects.KindRule:                   "RUL",
		objects.KindScenario:               "SCE",
		objects.KindSchedulerJob:           "SCH",
		objects.KindTemplate:               "TEM",
		objects.KindVision:                 "VIS",
		"fission_event":                    "FIS",
		"auto_fix_rule":                    "AFR",
		"test_case":                        "TEST",
		"workflow":                         "WFL",
		"test_command_rule":                "TCR",
		"kind_mapping_metric":              "KMM",
		"file_lock_metric":                 "FLM",
		"maturation_report":                "MAT",
		"technical_debt":                   "TDE",
		"policy":                           "POLICY-OBS",
		"test_audit_aggregation_metric":    "TAM",
		"command_spec":                     "CMS",
		"commercial_sequence":              "CSE",
		"agent_architecture":               "AGENT-ARCH",
		"agent_instruction":                "AGI",
		"resume_profile":                   "RSP",
		"agent_onboarding_preparation":     "AGE",
		"metrics_exchange_contract":        "MXC",
		"compression_policy":               "COMPOL",
		"domain_registry":                  "DOMAIN-REG",
		"base_sampler":                     "BSA",
		"capacity_advertisement":           "CADV",
		"compute_advertisement":            "CAD",
		"inference_heuristic":              "IFH",
		"infrastructure_adapter":           "IFA",
		"list_metric_sampler":              "LIS",
		"ordered_list_metric_sampler":      "ORD",
		"sampler_profile":                  "SAM",
		"scalar_metric_sampler":            "SCA",
		"status_history_metric_sampler":    "STA",
		"agent_feed":                       "AGF",
		"agent_skill":                      "ASK",
		"auth_strategy":                    "AUTH",
		"brand":                            "BRA",
		"bucketing_strategy":               "BS",
		"capability":                       "CPB",
		"certificate":                      "CERT",
		"code_quality_metric":              "CQM",
		objects.FieldKeyComponent:          "COMP",
		"convergence_session":              "CVS",
		"department":                       "DEP",
		"division":                         "DIV",
		"evolution_management":             "EVOL",
		"glossary_term":                    "GLS",
		"glossary_term_relation":           "GTR",
		"impact_analysis":                  "IMP",
		"import_tracking":                  "IMPTRK",
		"important_date":                   "DATE",
		"job_listing":                      "JLI",
		"job_search_profile":               "JSP",
		"keystore_entry":                   "KEY",
		"kind_synonym":                     "SYN",
		"library":                          "LIB",
		"lifecycle":                        "LIF",
		"mcp_session":                      "MCP",
		"metrics_feedback":                 "MFB",
		"namespace":                        "NAMESPACE",
		"namespace_registry":               "NAMESPACE-REGISTRY",
		"object_spec":                      "OBJ",
		"organization":                     "ORG",
		"organizational_change":            "OCH",
		"partnership":                      "PAR",
		"process_hygiene_rule":             "PHR",
		"prompt_template":                  "PROMPT",
		"qa_success":                       "QAS",
		objects.FieldKeyRole:               "ROLE",
		"scheduler_handler_binding":        "SHB",
		"scheduler_health_metric":          "SHM",
		"stakeholder_profile":              "STK",
		"strategic_context":                "SC",
		"strategic_plan":                   "STRAT-PLAN",
		"team":                             "TEA",
		"verification_matrix":              "VMX",
		"vocabulary_scheme":                "VOC",
		"workstream_transition":            "WST",
		"application_record":               "APP",
		"economic_policy":                  "ECP",
		"ecosystem_overview":               "ECO",
		"field_registry":                   "FIE",
		objects.FieldKeyNarrative:          "NAR",
		"tailored_resume":                  "TAI",
		"vitality_report":                  "VIT",
		"zqk_session":                      "ZQK",
		"pipeline_definition":              "PLD",
		"pipeline_execution":               "PLX",
	}

	prefix, ok := kindPrefixes[kind]
	if !ok {
		// Fallback: use first 3 uppercase letters of kind
		if len(kind) >= 3 {
			prefix = strings.ToUpper(kind[:3])
		} else {
			prefix = strings.ToUpper(kind)
		}
	}

	// Account uses special format
	if kind == objects.KindAccount {
		return "account:test"
	}

	return fmt.Sprintf("%s-999", prefix)
}

// getInitialStatus returns a valid initial status for a given kind
func getInitialStatus(kind string) string {
	// Map kinds to their typical initial statuses (from lifecycle definitions)
	initialStatuses := map[string]string{
		objects.KindBacklogItem:        testStatusExploring,
		objects.KindGoal:               testStatusActive,
		objects.KindMilestone:          objects.ObjectStatusNotStarted,
		objects.KindWorkstream:         testStatusPlanned,
		objects.KindPriorityPlan:       testStatusActive,
		objects.KindCriteria:           objects.ObjectStatusNotStarted,
		objects.KindRequirement:        testStatusPlanned,
		objects.KindTestCase:           testStatusDraft,
		objects.KindRoadmap:            testStatusActive,
		objects.KindDecision:           testStatusDraft,
		objects.KindAccount:            testStatusActive,
		objects.KindMission:            testStatusDraft,
		objects.KindVision:             testStatusDraft,
		objects.KindAuditEvent:         testStatusPending,
		objects.KindQuestion:           testStatusOpen,
		objects.KindChangeJournalEntry: testStatusPending,
		objects.KindDocEntry:           testStatusDraft,
	}

	status, ok := initialStatuses[kind]
	if !ok {
		return testStatusPlanned // Default fallback
	}
	return status
}

// generateIntegerValue generates an integer value respecting constraints
//
//nolint:unused // Test helper - reserved for future use
func generateIntegerValue(validation map[string]any) int {
	var minVal, maxVal int
	switch v := validation["min"].(type) {
	case int:
		minVal = v
	case float64:
		minVal = int(v)
	}
	switch v := validation["max"].(type) {
	case int:
		maxVal = v
	case float64:
		maxVal = int(v)
	}

	// Generate value within bounds
	if maxVal > 0 && minVal >= 0 {
		// Both bounds specified - use middle value
		return (minVal + maxVal) / 2
	}
	if minVal > 0 {
		// Only min specified
		return minVal + 1
	}
	if maxVal > 0 {
		// Only max specified
		return maxVal / 2
	}

	// No constraints - use a reasonable test value
	return 42
}

// generateNumberValue generates a number/float value respecting constraints
//
//nolint:unused // Test helper - reserved for future use
func generateNumberValue(validation map[string]any) float64 {
	var minVal, maxVal float64
	switch v := validation["min"].(type) {
	case float64:
		minVal = v
	case int:
		minVal = float64(v)
	}
	switch v := validation["max"].(type) {
	case float64:
		maxVal = v
	case int:
		maxVal = float64(v)
	}

	// Generate value within bounds
	if maxVal > 0 && minVal >= 0 {
		// Both bounds specified - use middle value
		return (minVal + maxVal) / 2.0
	}
	if minVal > 0 {
		// Only min specified
		return minVal + 1.0
	}
	if maxVal > 0 {
		// Only max specified
		return maxVal / 2.0
	}

	// No constraints - use a reasonable test value
	return 42.0
}

// generateListValue generates a list/array value respecting constraints
//
//nolint:unused // Test helper - reserved for future use
func generateListValue(validation map[string]any) []string {
	var minLength, maxLength int
	switch v := validation["min_length"].(type) {
	case int:
		minLength = v
	case float64:
		minLength = int(v)
	}
	switch v := validation["max_length"].(type) {
	case int:
		maxLength = v
	case float64:
		maxLength = int(v)
	}

	// Generate list with appropriate length
	length := 2 // Default: 2 items
	if maxLength > 0 {
		length = maxLength / 2
		if length < minLength {
			length = minLength
		}
		if length > 10 {
			length = 5 // Cap at reasonable length
		}
	} else if minLength > 0 {
		length = minLength
	}

	result := make([]string, length)
	for i := 0; i < length; i++ {
		result[i] = fmt.Sprintf(testItemFmt, i+1)
	}
	return result
}

// generateEnumValue generates an enum value from allowed values
//
//nolint:unused // Test helper - reserved for future use
func generateEnumValue(validation map[string]any) string {
	if enum, ok := validation["enum"].([]any); ok && len(enum) > 0 {
		// Use first enum value
		if str, ok := enum[0].(string); ok {
			return str
		}
		// If not string, convert
		return fmt.Sprintf("%v", enum[0])
	}
	// No enum values - return default
	return "test"
}

// generateDateTimeValue generates a datetime value respecting constraints
func generateDateTimeValue(validation map[string]any) string {
	// Check for pattern to determine format
	if pattern := objects.GetString(validation, "pattern"); pattern != "" {
		if strings.Contains(pattern, ConstStreamTD2D2D2Z) {
			// Full datetime
			return testDateTimeFixed
		}
		// Date only
		return testDateFixed
	}
	// Default: full datetime
	return testDateTimeFixed
}

// generateObjectValue generates an object value
func generateObjectValue(_ map[string]any) map[string]any {
	// Simple object with test data
	return map[string]any{
		"key": "value",
		"num": 42,
	}
}

// generateTestValuesWithBoundaries generates multiple test values including boundary conditions
// Returns a slice of values to try, ordered from most likely to succeed to edge cases
func generateTestValuesWithBoundaries(fieldName string, fieldDef any) []any {
	values := []any{}

	// Extract field definition map
	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		return []any{testValueDefault} // Fallback
	}

	// Get field type
	fieldType, _ := fieldMap[objects.FieldKeyType].(string)

	// Extract validation constraints
	var validation map[string]any
	if val, ok := fieldMap["validation"].(map[string]any); ok {
		validation = val
	}

	// Check if field is required
	required := false
	if req, ok := validation["required"].(bool); ok {
		required = req
	}

	switch fieldType {
	case "string", "text":
		// Generate string values with boundaries
		values = generateStringBoundaryValues(fieldName, validation, required)
	case "integer":
		values = generateIntegerBoundaryValues(validation, required)
	case "number", "float":
		values = generateNumberBoundaryValues(validation, required)
	case "boolean":
		values = []any{true, false} // Test both boolean values
	case "list", "array":
		values = generateListBoundaryValues(validation, required)
	case "enum":
		// Use enum values from spec
		if enum, ok := validation["enum"].([]any); ok && len(enum) > 0 {
			values = append(values, enum...)
		}
		if len(values) == 0 {
			values = []any{"test"} // Fallback
		}
	case "datetime", "date":
		values = []any{generateDateTimeValue(validation)} // Datetime is usually constrained by pattern
	case "object":
		values = []any{generateObjectValue(validation)}
	default:
		values = []any{testValueDefault} // Fallback
	}

	// If no values generated, use a default
	if len(values) == 0 {
		values = []any{testValueDefault}
	}

	return values
}

// generateStringBoundaryValues generates string values testing boundaries
func generateStringBoundaryValues(fieldName string, validation map[string]any, required bool) []any {
	_ = fieldName // Reserved for future use
	values := []any{}

	// Get length constraints
	var minLength, maxLength int
	switch v := validation["min_length"].(type) {
	case int:
		minLength = v
	case float64:
		minLength = int(v)
	}
	switch v := validation["max_length"].(type) {
	case int:
		maxLength = v
	case float64:
		maxLength = int(v)
	}

	// Check for enum first
	if enum, ok := validation["enum"].([]any); ok && len(enum) > 0 {
		for _, val := range enum {
			if str, ok := val.(string); ok {
				values = append(values, str)
			}
		}
		if len(values) > 0 {
			return values // Use enum values if available
		}
	}

	// Check for pattern
	hasPattern := false
	if pattern := objects.GetString(validation, "pattern"); pattern != emptyValue {
		hasPattern = true
		// Generate pattern-matching values
		if strings.Contains(pattern, ConstStreamD4D2D2) {
			values = append(values, testDateFixed)
		}
		if strings.Contains(pattern, ConstStreamTD2D2D2Z) {
			values = append(values, testDateTimeFixed)
		}
		if strings.Contains(pattern, ConstStreamDDD) {
			values = append(values, objects.DefaultSchemaVersion)
		}
		if strings.Contains(pattern, "^[A-Z]+-\\d") {
			values = append(values, "TEST-001")
		}
		if strings.Contains(pattern, "account:") {
			values = append(values, "account:test")
		}
	}

	// Generate boundary values for length-constrained strings
	if maxLength > 0 || minLength > 0 {
		// 1. Value at min length (if min specified)
		if minLength > 0 {
			values = append(values, strings.Repeat("a", minLength))
		}
		// 2. Value at max length (if max specified)
		if maxLength > 0 {
			values = append(values, strings.Repeat("a", maxLength))
		}
		// 3. Value in middle of range
		if minLength > 0 && maxLength > 0 {
			mid := (minLength + maxLength) / 2
			values = append(values, strings.Repeat("a", mid))
		}
		// 4. Alphanumeric value within bounds
		if maxLength > 0 {
			length := maxLength / 2
			if length < minLength {
				length = minLength
			}
			if length > 50 {
				length = 20 // Reasonable length
			}
			values = append(values, fmt.Sprintf(ConstStreamTestValueIntAbc, length))
		}
	} else {
		// No length constraints - test various values
		if !required {
			values = append(values, "") // Empty string (if not required)
		}
		values = append(values,
			"test",                         // Short
			testValueNumbered,              // Alphanumeric
			ConstStreamTestValueWithSpaces, // With spaces
			"123456789",                    // Numeric string
			ConstStreamTestvalue123Abc)     // Mixed case alphanumeric
	}

	// If no values generated yet, use defaults
	if len(values) == 0 {
		if !required && !hasPattern {
			values = append(values, "") // Empty string
		}
		values = append(values, testValueNumbered)
	}

	return values
}

// generateIntegerBoundaryValues generates integer values testing boundaries
func generateIntegerBoundaryValues(validation map[string]any, required bool) []any {
	values := []int{}

	var minVal, maxVal int
	switch v := validation["min"].(type) {
	case int:
		minVal = v
	case float64:
		minVal = int(v)
	}
	switch v := validation["max"].(type) {
	case int:
		maxVal = v
	case float64:
		maxVal = int(v)
	}

	if maxVal > 0 && minVal >= 0 {
		// Both bounds specified
		values = append(values, minVal, maxVal, (minVal+maxVal)/2) // At minimum, maximum, and middle
		if minVal < maxVal {
			values = append(values, minVal+1, maxVal-1) // Just above min, just below max
		}
	}
	if maxVal <= 0 && minVal > 0 {
		// Only min specified
		values = append(values, minVal, minVal+1, minVal+10)
	}
	if maxVal > 0 && minVal < 0 {
		// Only max specified
		values = append(values, maxVal, maxVal-1, maxVal/2)
	}
	if maxVal <= 0 && minVal < 0 {
		// No constraints
		if !required {
			values = append(values, 0, 1, 42, 100) // Zero, small positive, typical, larger
		} else {
			values = append(values, 1, 42, 100) // Small positive, typical, larger
		}
	}

	// Convert to []any
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}

// generateNumberBoundaryValues generates number/float values testing boundaries
func generateNumberBoundaryValues(validation map[string]any, required bool) []any {
	values := []float64{}

	var minVal, maxVal float64
	switch v := validation["min"].(type) {
	case float64:
		minVal = v
	case int:
		minVal = float64(v)
	}
	switch v := validation["max"].(type) {
	case float64:
		maxVal = v
	case int:
		maxVal = float64(v)
	}

	if maxVal > 0 && minVal >= 0 {
		// Both bounds specified
		values = append(values, minVal, maxVal, (minVal+maxVal)/2.0) // At minimum, maximum, and middle
		if minVal < maxVal {
			values = append(values, minVal+0.1, maxVal-0.1) // Just above min, just below max
		}
	}
	if maxVal <= 0 && minVal > 0 {
		// Only min specified
		values = append(values, minVal, minVal+1.0, minVal+10.0)
	}
	if maxVal > 0 && minVal < 0 {
		// Only max specified
		values = append(values, maxVal, maxVal-1.0, maxVal/2.0)
	}
	if maxVal <= 0 && minVal < 0 {
		// No constraints
		if !required {
			values = append(values, 0.0, 1.0, 42.0, 100.0) // Zero, small positive, typical, larger
		} else {
			values = append(values, 1.0, 42.0, 100.0) // Small positive, typical, larger
		}
	}

	// Convert to []any
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}

// generateListBoundaryValues generates list/array values testing boundaries
func generateListBoundaryValues(validation map[string]any, required bool) []any {
	values := []any{}

	var minLength, maxLength int
	switch v := validation["min_length"].(type) {
	case int:
		minLength = v
	case float64:
		minLength = int(v)
	}
	switch v := validation["max_length"].(type) {
	case int:
		maxLength = v
	case float64:
		maxLength = int(v)
	}

	// Generate lists with different lengths
	if maxLength > 0 || minLength > 0 {
		// At minimum length
		if minLength >= 0 {
			minList := make([]string, minLength)
			for i := 0; i < minLength; i++ {
				minList[i] = fmt.Sprintf(testItemFmt, i+1)
			}
			values = append(values, minList)
		}
		// At maximum length
		if maxLength > 0 {
			maxList := make([]string, maxLength)
			for i := 0; i < maxLength; i++ {
				maxList[i] = fmt.Sprintf(testItemFmt, i+1)
			}
			values = append(values, maxList)
		}
		// Middle length
		if minLength > 0 && maxLength > 0 {
			mid := (minLength + maxLength) / 2
			midList := make([]string, mid)
			for i := 0; i < mid; i++ {
				midList[i] = fmt.Sprintf(testItemFmt, i+1)
			}
			values = append(values, midList)
		}
	} else {
		// No length constraints
		if !required {
			values = append(values,
				[]string{},                     // Empty list
				[]string{testItemPrefix + "1"}, // Single item
				[]string{testItemPrefix + "1", testItemPrefix + "2"},                       // Two items
				[]string{testItemPrefix + "1", testItemPrefix + "2", testItemPrefix + "3"}) // Three items
		} else {
			values = append(values,
				[]string{testItemPrefix + "1"},                                             // Single item
				[]string{testItemPrefix + "1", testItemPrefix + "2"},                       // Two items
				[]string{testItemPrefix + "1", testItemPrefix + "2", testItemPrefix + "3"}) // Three items
		}
	}

	// If no values generated, use default
	if len(values) == 0 {
		values = append(values, []string{testItemPrefix + "1", testItemPrefix + "2"})
	}

	return values
}

// setRequiredFieldsForTest sets required fields for test object creation
func setRequiredFieldsForTest(obj map[string]any, kind string) {
	switch kind {
	case objects.KindRequirement:
		// Requirement requires non-empty goal_refs and criteria_refs (minCount: 1)
		// Use GOL-999 and CRIT-999 to match the referenced objects created in setup
		obj[objects.FieldKeyGoalRefs] = []string{"GOL-999"}
		obj[objects.FieldKeyCriteriaRefs] = []string{"CRIT-999"}
	case objects.KindBaseMetric:
		obj[objects.FieldKeyMetricType] = testMetricTypePerf
		obj[objects.FieldKeyFirstSeen] = testDateTimeStart
		obj[objects.FieldKeyLastSeen] = testDateTimeEnd
		obj[objects.FieldKeyCollectionCount] = 1.0
	case objects.KindWorkstream:
		obj[objects.FieldKeyOwnerRef] = ConstStreamAccountTestuser // Use valid username that matches validation pattern
		obj[objects.FieldKeyEntryPoint] = "docs/workstreams/test.md"
	case objects.KindAccount:
		obj[objects.FieldKeyUsername] = "testuser"
	case objects.KindCriteria:
		obj[objects.FieldKeyCategory] = "functional"
	case objects.KindAuditEvent:
		obj[objects.FieldKeyEventType] = ConstStreamSystemConfigChange // Use valid enum value
		obj[objects.FieldKeyOperation] = ConstStreamTestOperation
		obj[objects.FieldKeyStatus] = objects.ObjectStatusCompleted // Override initial status to valid enum
	case objects.KindChangeJournalEntry:
		// Change journal entry requires object_ref (format: "kind:id") and change_type
		obj[objects.FieldKeyObjectRef] = objects.KindGoal + ":GOL-999" // Use "kind:id" format
		obj[objects.FieldKeyChangeType] = testChangeTypeCreate
		obj[objects.FieldKeyStatus] = testStatusPending // Override to valid initial status
	case objects.KindDocEntry:
		// Doc entry requires path and summary
		obj[objects.FieldKeyPath] = "docs/test.md"
		obj[objects.FieldKeySummary] = ConstStreamTestDocumentEntry
		obj[objects.FieldKeyStatus] = testStatusDraft // Override to valid initial status
	case objects.KindAuditAggregationMetric:
		// Use correct ID format (AAM- not AUD-)
		if id := objects.GetString(obj, objects.FieldKeyID); strings.HasPrefix(id, "AUD-") {
			obj[objects.FieldKeyID] = strings.Replace(id, "AUD-", "AAM-", 1)
		}
		obj[objects.FieldKeyStatus] = objects.ObjectStatusCompleted // Override initial status to valid lifecycle status
		obj[objects.FieldKeyMetricType] = "system"
		obj[objects.FieldKeyAggregationWindowStart] = testDateTimeStart
		obj[objects.FieldKeyAggregationWindowEnd] = testDateTimeEnd
		obj[objects.FieldKeyFirstSeen] = testDateTimeStart
		obj[objects.FieldKeyLastSeen] = testDateTimeEnd
		obj[objects.FieldKeyCollectionCount] = 1.0
		obj[objects.FieldKeyEventCount] = 10.0
		obj[objects.FieldKeyEventTypeCounts] = map[string]any{testChangeTypeCreate: 5, "update": 5}
	case objects.KindCommandMetric:
		// Use correct ID format (CMD- not COM-)
		if id := objects.GetString(obj, objects.FieldKeyID); strings.HasPrefix(id, "COM-") {
			obj[objects.FieldKeyID] = strings.Replace(id, "COM-", "CMD-", 1)
		}
		obj[objects.FieldKeyCommand] = "test_command"
		obj[objects.FieldKeyNormalizedCmd] = "test_command"
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
		obj[objects.FieldKeyMetricType] = testMetricTypePerf
		obj[objects.FieldKeyFirstSeen] = testDateTimeStart
		obj[objects.FieldKeyLastSeen] = testDateTimeEnd
		obj[objects.FieldKeyCollectionCount] = 1.0
	case objects.KindSynonym:
		// Use correct ID format (SYN- not KIN-)
		if id := objects.GetString(obj, objects.FieldKeyID); strings.HasPrefix(id, "KIN-") {
			obj[objects.FieldKeyID] = strings.Replace(id, "KIN-", "SYN-", 1)
		}
		obj[objects.FieldKeyStatus] = testStatusActive // Override initial status to valid lifecycle status
		obj["kind_name"] = objects.KindBacklogItem
		obj[objects.FieldKeySynonym] = "bli_test"
		obj[objects.FieldKeySourceType] = "built-in"
	case objects.KindMission:
		obj[objects.FieldKeyMissionStatement] = ConstStreamTestMissionStatement
	case objects.KindVision:
		obj[objects.FieldKeyNarrative] = ConstStreamTestVisionNarrative
	case objects.KindQuestion:
		obj[objects.FieldKeyQuestionText] = ConstStreamTestQuestion
	}
}

// normalizeValue normalizes values for comparison (handles []string vs []any)
func normalizeValue(v any) any {
	if v == nil {
		return nil
	}
	switch

	// Handle slices - convert []any to []string if all elements are strings
	v := v.(type) {
	case []any:
		result := make([]string, 0, len(v))
		allStrings := true
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				allStrings = false
				break
			}
		}
		if allStrings {
			return result
		}
		return v
	}

	// Handle []string - keep as is

	return v
}
