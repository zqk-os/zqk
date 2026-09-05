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
		"policy":                           "POL-OBS",
		"test_audit_aggregation_metric":    "TAM",
		"command_spec":                     "CMS",
		"digital_asset":                    "DAS",
		"agent_architecture":               "AGENT-ARCH",
		"agent_instruction":                "AGI",
		"agent_onboarding_preparation":     "AGE",
		"domain_registry":                  "DOMAIN-REG",
		"base_sampler":                     "BSA",
		"capacity_advertisement":           "CADV",
		"compute_advertisement":            "CAD",
		"inference_heuristic":              "IFH",
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
		"visual_plan":                      "VPL",
		"vocabulary_scheme":                "VOC",
		"workstream_transition":            "WST",
		"ecosystem_overview":               "ECO",
		"field_registry":                   "FIE",
		objects.FieldKeyNarrative:          "NAR",
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
		return "ACC-TEST"
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

// generateDateTimeValue generates a datetime value respecting constraints
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
