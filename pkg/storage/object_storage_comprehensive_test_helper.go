package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
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

func createTestObjectsForKind(t *testing.T, storage *FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, kindFields *objects.KindFields) []map[string]any {
	// Create a few test objects with varied field values
	testObjects := []map[string]any{}

	// Get initial status for this kind
	initialStatus := getInitialStatusForKind(kind)

	// Create 5 test objects with different values to test various scenarios
	for i := 0; i < 5; i++ {
		obj := make(map[string]any)
		obj[objects.FieldKeyKind] = kind
		obj[objects.FieldKeyID] = generateComprehensiveTestID(kind, i+1)
		obj[objects.FieldKeyTitle] = fmt.Sprintf(ConstStreamFixtureStrInt, kind, i+1)
		obj[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion

		// Set status with valid values for this kind
		if hasField(kindFields, "status") {
			statuses := getValidStatusesForKind(kind)
			if len(statuses) > 0 {
				// Use valid status from the list, cycling through them
				obj[objects.FieldKeyStatus] = statuses[i%len(statuses)]
			} else {
				// Fallback to initial status if no valid statuses found
				obj[objects.FieldKeyStatus] = initialStatus
			}
			// Override status for specific kinds that have strict enum requirements
			switch kind {
			case objects.KindAuditEvent:
				// audit_event only allows: pending, completed, failed, reverted
				validAuditStatuses := []string{objects.ObjectStatusPending, objects.ObjectStatusCompleted, objects.ObjectStatusFailed, "reverted"}
				obj[objects.FieldKeyStatus] = validAuditStatuses[i%len(validAuditStatuses)]
			case objects.KindBacklogItem:
				// backlog_item only allows: exploring, validated, planned, in_progress, complete, archived, rejected
				validBacklogStatuses := []string{compTestStatusExploring, objects.ObjectStatusValidated, objects.ObjectStatusPlanned, objects.ObjectStatusInProgress, testStatusComplete, objects.ObjectStatusArchived, testStatusRejected}
				obj[objects.FieldKeyStatus] = validBacklogStatuses[i%len(validBacklogStatuses)]
			}
		}

		// Set priority if field exists
		if hasField(kindFields, "priority") {
			priorities := []string{testFieldLow, "medium", "high", "critical", testFieldLow}
			obj[objects.FieldKeyPriority] = priorities[i%len(priorities)]
		}

		// Set category if field exists
		if hasField(kindFields, "category") {
			// Use valid enum values based on kind
			switch kind {
			case objects.KindCriteria:
				// criteria category enum: functional, non-functional, acceptance, test, performance, security, compliance
				validCategories := []string{"functional", ConstStreamNonFunctional, "acceptance", "test", testCategoryPerf, "security", "compliance"}
				obj[objects.FieldKeyCategory] = validCategories[i%len(validCategories)]
			case objects.KindWorkstream:
				// workstream category enum: application, system, feature, component, ops, tooling
				validCategories := []string{testDomainApplication, testDomainSystem, testDomainFeature, workstreamCategoryComponent, testDomainOps, testDomainTooling}
				obj[objects.FieldKeyCategory] = validCategories[i%len(validCategories)]
			default:
				// For other kinds, use generic categories
				categories := []string{testCategoryA, testCategoryB, "CategoryC", testCategoryA, testCategoryB}
				obj[objects.FieldKeyCategory] = categories[i%len(categories)]
			}
		}

		// Set priority_plan_ref if field exists (for grouping tests)
		// Note: We skip this for now since it requires referenced objects to exist
		// TODO: Create referenced objects first or make this optional
		// if hasField(kindFields, "priority_plan_ref") {
		// 	planRefs := []string{"PRIO-001", "PRIO-002", "PRIO-001", "PRIO-003", "PRIO-002"}
		// 	obj["priority_plan_ref"] = planRefs[i%len(planRefs)]
		// }

		// Set kind-specific required fields
		setKindSpecificFields(obj, kind, i)

		// Create referenced objects if needed (e.g., account for workstream)
		if kind == objects.KindWorkstream {
			// Create referenced account if it doesn't exist
			accountID := "ACC-001"
			accountObj := map[string]any{
				objects.FieldKeyID:            accountID,
				objects.FieldKeyKind:          objects.KindAccount,
				objects.FieldKeyTitle:         ConstStreamTestAccountForWorkstream,
				objects.FieldKeyUsername:      "testuser",
				objects.FieldKeyStatus:        compTestStatusActive,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			}
			// Try to create account (ignore if already exists)
			_, err := storage.Read(ctx, secCtx, accountID)
			if err != nil {
				// Account doesn't exist, create it
				if createErr := storage.Create(ctx, secCtx, accountObj); createErr != nil {
					if createErr != ErrObjectExists {
						t.Logf(ConstStreamWarningCouldNotCreateReferencedAccountStrVal, accountID, createErr)
					}
				}
			}
		}

		// Create object
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Logf(ConstStreamFailedToCreateTestObjectStrForKindStrValSkipping, obj[objects.FieldKeyID], kind, err)
			continue
		}

		testObjects = append(testObjects, obj)
	}

	return testObjects
}

func hasField(kindFields *objects.KindFields, fieldName string) bool {
	for i := range kindFields.AllFields {
		field := kindFields.AllFields[i]
		if field.Name == fieldName {
			return true
		}
	}
	return false
}

func generateComprehensiveTestID(kind string, index int) string {
	// Use ID validator to get the correct prefix dynamically
	validator := validation.GetIDValidator()
	if err := validator.LoadPatterns(); err != nil {
		// Fallback to hardcoded if validator fails
		return generateComprehensiveTestIDFallback(kind, index)
	}

	// Special handling for policy IDs: POLICY-CATEGORY-### format
	if kind == objects.KindPolicy {
		// Policy requires a category (POLICY-CODE-001, POLICY-SEC-001, etc.)
		// Use POLICY-CODE- as the default category for tests
		return fmt.Sprintf("POLICY-CODE-%03d", index)
	}

	// Special handling for agent_architecture: AGENT-ARCH-### format
	if kind == objects.KindAgentArchitecture {
		return fmt.Sprintf(ConstStreamAgentArch03d, index)
	}

	// Special handling for agent_feed: AGF-### format
	if kind == objects.KindAgentFeed {
		return fmt.Sprintf("AGF-%03d", index)
	}

	// Special handling for agent_onboarding_preparation: AGENT-PREP-### format (per spec)
	if kind == objects.KindAgentOnboardingPreparation {
		return fmt.Sprintf(ConstStreamAgentPrep03d, index)
	}

	// Special handling for rollback_report: RBR-### format (to avoid conflict with role's ROL-)
	if kind == objects.KindRollbackReport {
		return fmt.Sprintf("RBR-%03d", index)
	}

	// prompt_template: inference rule would yield PRO-001 which is not registered for this kind; use PROMPT- (see id_prefixes_config).
	if kind == kindPromptTemplate {
		return fmt.Sprintf("PROMPT-%03d", index)
	}

	// Get valid prefixes for this kind
	prefixes := validator.GetValidPrefixes(kind)
	if len(prefixes) == 0 {
		// Fallback if no pattern found
		return generateComprehensiveTestIDFallback(kind, index)
	}

	// Use the first prefix from the pattern
	// Remove trailing dash if present (validator returns "ITEM-", we need "BLI")
	prefix := strings.TrimSuffix(prefixes[0], "-")

	// ID format must be PREFIX-001 (at least 3 digits)
	return fmt.Sprintf("%s-%03d", prefix, index)
}

// generateComprehensiveTestIDFallback provides fallback prefixes from cached validator defaults
// This uses the ID validator's default patterns as a fallback when dynamic loading fails
var (
	fallbackPrefixCache     map[string]string
	fallbackPrefixCacheOnce sync.Once
)

// statusCache provides cached initial statuses from lifecycle definitions
// Loaded during initialization from lifecycle loader, with fallback for built-in objects
var (
	statusCache     map[string]string
	statusCacheOnce sync.Once
	// comprehensiveTestCacheMu protects dynamic writes to statusCache / validStatusesCache after
	// initialize* Once() completes (parallel subtests call get* concurrently).
	comprehensiveTestCacheMu sync.RWMutex
)

// validStatusesCache provides cached valid statuses from lifecycle definitions
// Loaded during initialization from lifecycle loader, with fallback for built-in objects
var (
	validStatusesCache     map[string][]string
	validStatusesCacheOnce sync.Once
)

func generateComprehensiveTestIDFallback(kind string, index int) string {
	// Initialize fallback cache from validator defaults
	// This cache is populated from the ID validator's default patterns
	fallbackPrefixCacheOnce.Do(func() {
		fallbackPrefixCache = make(map[string]string)

		// Load default patterns from validator
		validator := validation.GetIDValidator()
		if err := validator.LoadPatterns(); err == nil {
			// Get all known kinds from the validator's loaded patterns
			// The validator loads defaults, so we can query it for any kind
			// We'll cache prefixes as we encounter them
		}
	})

	// Try to get prefix from cache first
	prefix, ok := fallbackPrefixCache[kind]
	if !ok {
		// If kind not in cache, try to get it from validator
		validator := validation.GetIDValidator()
		if err := validator.LoadPatterns(); err == nil {
			prefixes := validator.GetValidPrefixes(kind)
			if len(prefixes) > 0 {
				// Remove trailing dash if present (validator returns "ITEM-", we need "BLI")
				prefix = strings.TrimSuffix(prefixes[0], "-")
				// Cache it for future use
				fallbackPrefixCache[kind] = prefix
			}
		}
	}

	if prefix == emptyValue {
		// Generate prefix from kind name (fallback generation strategy)
		// This creates a deterministic prefix based on the kind name
		parts := strings.Split(kind, "_")
		if len(parts) > 1 {
			// Multi-word: use first letter of each word, but limit to 3-4 chars total
			prefix = ""
			for _, part := range parts {
				if part != emptyValue {
					prefix += strings.ToUpper(part[:1])
					// Limit to 3-4 chars for compatibility with ID format
					if len(prefix) >= 3 {
						break
					}
				}
			}
			// If we got less than 3 chars, pad with more letters from first word
			if len(prefix) < 3 && len(parts) > 0 && len(parts[0]) >= 3 {
				needed := 3 - len(prefix)
				prefix = strings.ToUpper(parts[0][:needed]) + prefix[1:]
			}
		} else {
			// Single word: use first 3-4 letters
			if len(kind) >= 4 {
				prefix = strings.ToUpper(kind[:4])
			} else {
				prefix = strings.ToUpper(kind)
			}
		}
		// Cache the generated prefix (identified as generated fallback)
		fallbackPrefixCache[kind] = prefix
	}

	// ID format must be PREFIX-001 (at least 3 digits)
	return fmt.Sprintf("%s-%03d", prefix, index)
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
func initializeStatusCache() {
	statusCacheOnce.Do(func() {
		statusCache = make(map[string]string)

		// Load from lifecycle loader (original read)
		lifecycleLoader := objects.NewLifecycleLoader("")

		// Try to load lifecycle for common kinds
		commonKinds := []string{
			objects.KindBacklogItem, objects.KindGoal, objects.KindMilestone, objects.KindComponent, objects.KindPriorityPlan,
			objects.KindRequirement, objects.KindCriteria, objects.KindTestCase, objects.KindRoadmap, objects.KindDecision,
			objects.KindAccount, objects.KindWorkstream, objects.KindRelease, objects.KindRole, objects.KindMission, objects.KindVision,
		}

		for _, kind := range commonKinds {
			lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
			if err == nil && lifecycle != nil {
				// Find initial status from lifecycle definition
				for _, status := range lifecycle.Statuses {
					if status.Origin {
						statusCache[kind] = status.Value
						break
					}
				}
			}
		}

		// Fallback for built-in objects (immutable without admin intervention)
		// Built-ins may have different statuses than regular objects
		// These are used when lifecycle doesn't define an initial status
		builtInStatusFallback := map[string]string{
			objects.KindBacklogItem:  compTestStatusExploring,
			objects.KindGoal:         compTestStatusActive,
			objects.KindMilestone:    testStatusNotStarted,
			objects.KindWorkstream:   compTestStatusActive,
			objects.KindPriorityPlan: compTestStatusDraft,
			objects.KindRequirement:  compTestStatusDraft,
			objects.KindTestCase:     compTestStatusDraft,
			objects.KindRoadmap:      compTestStatusDraft,
			objects.KindDecision:     testStatusProposed,
			objects.KindComponent:    testStatusCreated,
			// Lifecycle allows only draft | active | archived; avoid generic "not_started" default.
			objects.KindVerificationMatrix: compTestStatusDraft,
		}

		// Only use fallback if lifecycle didn't provide a status
		// This handles conflicts: lifecycle takes precedence, but built-ins
		// can override if lifecycle is missing
		for kind, fallbackStatus := range builtInStatusFallback {
			if _, exists := statusCache[kind]; !exists {
				statusCache[kind] = fallbackStatus
			}
		}
	})
}

// getInitialStatusForKind returns a valid initial status for a given kind
// Uses cached statuses from lifecycle definitions, with fallback for built-in objects
func getInitialStatusForKind(kind string) string {
	// Initialize cache if not already done (happens during lifecycle initialization)
	initializeStatusCache()

	comprehensiveTestCacheMu.RLock()
	if status, ok := statusCache[kind]; ok {
		comprehensiveTestCacheMu.RUnlock()
		return status
	}
	comprehensiveTestCacheMu.RUnlock()

	// If not in cache, try to load lifecycle dynamically (outside lock — I/O)
	lifecycleLoader := objects.NewLifecycleLoader("")
	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	var resolved string
	if err == nil && lifecycle != nil {
		for _, status := range lifecycle.Statuses {
			if status.Origin {
				resolved = status.Value
				break
			}
		}
	}
	if resolved == emptyValue {
		resolved = testStatusNotStarted
	}

	comprehensiveTestCacheMu.Lock()
	defer comprehensiveTestCacheMu.Unlock()
	if status, ok := statusCache[kind]; ok {
		return status
	}
	statusCache[kind] = resolved
	return resolved
}

// initializeValidStatusesCache loads valid statuses from lifecycle definitions
// This should be called during test initialization to establish fallback cache
// Lifecycles and built-in objects may have conflicting statuses because built-ins
// are immutable without admin intervention
func initializeValidStatusesCache() {
	validStatusesCacheOnce.Do(func() {
		validStatusesCache = make(map[string][]string)

		// Load from lifecycle loader (original read)
		lifecycleLoader := objects.NewLifecycleLoader("")

		// Try to load lifecycle for common kinds
		commonKinds := []string{
			objects.KindBacklogItem, objects.KindGoal, objects.KindMilestone, objects.KindComponent, objects.KindPriorityPlan,
			objects.KindRequirement, objects.KindCriteria, objects.KindTestCase, objects.KindRoadmap, objects.KindDecision,
			objects.KindAccount, objects.KindWorkstream, objects.KindRelease, objects.KindRole, objects.KindMission, objects.KindVision,
		}

		for _, kind := range commonKinds {
			lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
			if err == nil && lifecycle != nil {
				// Extract all valid statuses from lifecycle definition
				statuses := make([]string, 0, len(lifecycle.Statuses))
				for _, status := range lifecycle.Statuses {
					statuses = append(statuses, status.Value)
				}
				if len(statuses) > 0 {
					validStatusesCache[kind] = statuses
				}
			}
		}

		// Fallback for built-in objects (immutable without admin intervention)
		// Built-ins may have different statuses than regular objects
		// These are used when lifecycle doesn't define statuses
		builtInStatusesFallback := map[string][]string{
			objects.KindBacklogItem:        {compTestStatusExploring, testStatusValidated, "planned", testStatusInProgress, testStatusComplete, testStatusArchived},
			objects.KindGoal:               {compTestStatusActive, testStatusComplete, testStatusDeferred},
			objects.KindMilestone:          {testStatusNotStarted, testStatusInProgress, "blocked", testStatusComplete, testStatusDeferred},
			objects.KindWorkstream:         {compTestStatusActive, testStatusComplete, "paused"},
			objects.KindPriorityPlan:       {compTestStatusDraft, compTestStatusActive, testStatusComplete},
			objects.KindRequirement:        {compTestStatusDraft, "approved", "implemented", "verified"},
			objects.KindTestCase:           {compTestStatusDraft, "ready", "passed", "failed"},
			objects.KindRoadmap:            {compTestStatusDraft, "published", testStatusArchived},
			objects.KindDecision:           {testStatusProposed, "approved", testStatusRejected, "superseded"},
			objects.KindComponent:          {testStatusCreated, testStatusValidated, "placed", "rendered"},
			objects.KindVerificationMatrix: {compTestStatusDraft, compTestStatusActive, testStatusArchived},
		}

		// Only use fallback if lifecycle didn't provide statuses
		// This handles conflicts: lifecycle takes precedence, but built-ins
		// can override if lifecycle is missing
		for kind, fallbackStatuses := range builtInStatusesFallback {
			if _, exists := validStatusesCache[kind]; !exists {
				validStatusesCache[kind] = fallbackStatuses
			}
		}
	})
}

// getValidStatusesForKind returns valid status values for a given kind
// Uses cached statuses from lifecycle definitions, with fallback for built-in objects
func getValidStatusesForKind(kind string) []string {
	// Initialize cache if not already done (happens during lifecycle initialization)
	initializeValidStatusesCache()

	comprehensiveTestCacheMu.RLock()
	if statuses, ok := validStatusesCache[kind]; ok {
		comprehensiveTestCacheMu.RUnlock()
		return statuses
	}
	comprehensiveTestCacheMu.RUnlock()

	lifecycleLoader := objects.NewLifecycleLoader("")
	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	var resolved []string
	if err == nil && lifecycle != nil {
		statuses := make([]string, 0, len(lifecycle.Statuses))
		for _, status := range lifecycle.Statuses {
			statuses = append(statuses, status.Value)
		}
		if len(statuses) > 0 {
			resolved = statuses
		}
	}
	if len(resolved) == 0 {
		resolved = []string{testStatusNotStarted, testStatusInProgress, testStatusComplete}
	}

	comprehensiveTestCacheMu.Lock()
	defer comprehensiveTestCacheMu.Unlock()
	if statuses, ok := validStatusesCache[kind]; ok {
		return statuses
	}
	validStatusesCache[kind] = resolved
	return resolved
}

// setKindSpecificFields sets required fields specific to certain kinds
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
