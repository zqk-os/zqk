// Extracted from object_storage_comprehensive_test_helper.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

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
					if !errors.Is(createErr, ErrObjectExists) {
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

func generateComprehensiveTestID(kind string, index int) string {
	// Use ID validator to get the correct prefix dynamically
	validator := validation.GetIDValidator()
	if err := validator.LoadPatterns(); err != nil {
		// Fallback to hardcoded if validator fails
		return generateComprehensiveTestIDFallback(kind, index)
	}

	// Special handling for policy IDs: POL-CATEGORY-### format
	if kind == objects.KindPolicy {
		// Policy requires a category (POL-CODE-001, POL-SEC-001, etc.)
		// Use POL-CODE- as the default category for tests
		return fmt.Sprintf("POL-CODE-%03d", index)
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
	// Remove trailing dash if present (validator returns "BLI-", we need "BLI")
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
				// Remove trailing dash if present (validator returns "BLI-", we need "BLI")
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
