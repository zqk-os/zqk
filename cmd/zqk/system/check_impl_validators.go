package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"

	stdcontext "context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	"github.com/zqk-os/zqk/pkg/validation"
)

func checkRegistration(obj *parser.ParsedObject, kind string) []Issue {
	return systemcheck.CheckRegistration(obj, kind)
}

func looksHandCASMaterialized(obj *parser.ParsedObject) bool {
	return systemcheck.LooksHandCASMaterialized(obj)
}

func checkLifecycle(obj *parser.ParsedObject, kind string) []Issue {
	return systemcheck.CheckLifecycle(obj, kind)
}

func checkLifecycleWithLoader(obj *parser.ParsedObject, kind string, lifecycleLoader *objects.LifecycleLoader) []Issue {
	return systemcheck.CheckLifecycleWithLoader(obj, kind, lifecycleLoader)
}

func checkInstanceValidation(ctx *cli.Context, stdCtx stdcontext.Context, obj *parser.ParsedObject, kind string) []Issue {
	valCtx, issues := initializeInstanceValidationContext(ctx, stdCtx, obj, kind)
	if valCtx == nil {
		return issues
	}

	result, err := performValidation(valCtx)
	validationIssues := convertValidationResultsToIssues(result, err, obj.ID, kind, valCtx.ObjMap, valCtx.ProjectRoot, valCtx.StorageProvider)

	return validationIssues
}
func checkInstanceValidationWithValidator(ctx *cli.Context, stdCtx stdcontext.Context, obj *parser.ParsedObject, kind string, validator validation.Validator) []Issue {
	// Convert ParsedObject to map[string]any for validation
	objMap := make(map[string]any)
	if obj.Properties != nil {
		objMap = obj.Properties
	}
	return checkInstanceValidationWithValidatorAndData(ctx, stdCtx, obj, kind, validator, objMap, nil)
}

// checkInstanceValidationWithValidatorAndData validates object instance using provided data map.
// When storageProvider is non-nil (e.g. async check path), it is reused to avoid creating a new
// StorageFactory and FileObjectStorage per object, which was causing extreme memory use (e.g. 17GB+).
func checkInstanceValidationWithValidatorAndData(ctx *cli.Context, stdCtx stdcontext.Context, obj *parser.ParsedObject, kind string, validator validation.Validator, objMap map[string]any, storageProvider storage.ObjectStorageProvider) []Issue {
	var issues []Issue

	// DEBUG: Log validator input for POL-DEBUG-001
	isDebugObject := obj.ID == "POL-DEBUG-001"
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	if isDebugObject {
		logging.Fluent(logger).Info("=== POL-DEBUG-001 DEBUG: VALIDATOR INPUT ===").
			Int("objMap_len", len(objMap)).
			Kind(kind).
			Log()
		if len(objMap) > 0 {
			bodyVal, bodyExists := objMap[objects.FieldKeyBody]
			categoryVal, categoryExists := objMap[objects.FieldKeyCategory]
			policyTypeVal, policyTypeExists := objMap[objects.FieldKeyPolicyType]
			logging.Fluent(logger).Info("Validator input objMap state").
				String("body_exists", fmt.Sprintf("%v", bodyExists)).
				String("body_type", fmt.Sprintf("%T", bodyVal)).
				String("category_exists", fmt.Sprintf("%v", categoryExists)).
				String("category_value", fmt.Sprintf("%v", categoryVal)).
				String("policy_type_exists", fmt.Sprintf("%v", policyTypeExists)).
				String("policy_type_value", fmt.Sprintf("%v", policyTypeVal)).
				Log()
		}
	}

	// Get current state from object (if available)
	currentState := ""
	if status, ok := objMap[objects.FieldKeyStatus].(string); ok {
		currentState = status
	}

	// Resolve storageProvider early for lookup
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if storageProvider == nil {
		storageProvider = getStorageProviderForCache(projectRoot)
	}

	// Prepare validation options
	options := validation.DefaultValidationOptions()
	options.CurrentState = currentState
	lookupProvider := storageProvider
	if hybrid, ok := storageProvider.(*storage.HybridObjectStorage); ok {
		lookupProvider = hybrid.GetPrimary()
	}
	// Use provided context or background if not set (needed before DependentsLookup closure).
	validateCtx := stdCtx
	if validateCtx == nil {
		validateCtx = pkgctx.NewSystemContext()
	}
	options.ObjectLookup = func(id string) (map[string]any, error) {
		secCtx := pkgctx.NewSystemSecurityContext()
		// Prefer hybrid/factory provider so file-CAS objects resolve during
		// DependentsLookup status checks (graph-primary unwrap can miss them).
		sp := storageProvider
		if sp == nil {
			sp = lookupProvider
		}
		return sp.Read(validateCtx, secCtx, id)
	}
	options.ObjectStatusLookup = func(id string) (string, error) {
		obj, err := options.ObjectLookup(id)
		if err != nil {
			return "", err
		}
		status, _ := obj[objects.FieldKeyStatus].(string)
		return status, nil
	}
	// Wire reverse refs so status-hold preconditions that use DependentsLookup
	// (e.g. priority_plan complete → all linked backlog_items terminal) are not
	// fail-closed false positives during system check. Same helper as promote/demote/save.
	// Use the hybrid/factory provider (not GetPrimary alone): graph-primary unwrap can
	// miss file CAS BLIs or surface stale graph edges and falsely fail the hold check.
	// TRACK: TDE-1785808957221945000-fcd15e47 — keep check/promote/storage lookup wiring aligned.
	depsProvider := storageProvider
	if depsProvider == nil {
		depsProvider = lookupProvider
	}
	options.DependentsLookup = func(id string) []string {
		return storage.DependentsForID(validateCtx, depsProvider, id)
	}
	options.ProjectRoot = projectRoot
	options.IsDraftPlaneOnly = func(id string) bool {
		return storage.IsDraftPlaneOnly(projectRoot, "", id)
	}

	// Validate instance using cached validator
	result, err := validator.Validate(validateCtx, objMap, kind, options)

	if isDebugObject {
		if err != nil {
			logging.Fluent(logger).Warn("Validator returned error").
				WithError(err).
				Log()
		} else {
			logging.Fluent(logger).Info("=== POL-DEBUG-001 DEBUG: VALIDATOR RESULT ===").
				Int("errors_count", len(result.Errors)).
				Int("warnings_count", len(result.Warnings)).
				Log()
			for i, validationError := range result.Errors {
				logging.Fluent(logger).Info("Validation error").
					Int("index", i).
					String("field", validationError.Field).
					String("message", validationError.Message).
					String("validation_rule", validationError.Rule).
					Log()
			}
		}
	}
	if err != nil {
		// Validation error (e.g., spec not found) - add as warning
		issues = append(issues, Issue{
			Tier:     2,
			Category: "instance_validation",
			Message:  fmt.Sprintf("Instance validation error: %v", err),
		})
		return issues
	}

	// Convert validation errors to issues using shared function.
	validationIssues := convertValidationResultsToIssues(result, err, obj.ID, kind, objMap, projectRoot, storageProvider)

	// Invariant: field 'title' must not contain raw YAML markup prefix (e.g. "id:" or "kind:")
	if title, ok := objMap[objects.FieldKeyTitle].(string); ok {
		trimmedTitle := strings.TrimSpace(title)
		if strings.HasPrefix(trimmedTitle, "id:") || strings.HasPrefix(trimmedTitle, "kind:") {
			validationIssues = append(validationIssues, Issue{
				Tier:        1,
				Category:    "instance_validation",
				Message:     fmt.Sprintf("Title contains raw YAML markup prefix: %q", trimmedTitle),
				AutoFixable: false,
			})
		}
	}

	// Invariant: priority plans in_progress where all child BLIs are terminal (complete/archived)
	if kind == objects.KindPriorityPlan && currentState == objects.ObjectStatusInProgress {
		deps := options.DependentsLookup(obj.ID)
		hasBLIs := false
		hasOpenBLIs := false
		totalBLIs := 0
		for _, depID := range deps {
			depObj, readErr := options.ObjectLookup(depID)
			if readErr != nil || depObj == nil {
				continue
			}
			depKind, _ := depObj[objects.FieldKeyKind].(string)
			if depKind != objects.KindBacklogItem && !strings.HasPrefix(depID, "BLI-") {
				continue
			}
			hasBLIs = true
			totalBLIs++
			depStatus, _ := depObj[objects.FieldKeyStatus].(string)
			if depStatus != objects.ObjectStatusComplete && depStatus != objects.ObjectStatusArchived {
				hasOpenBLIs = true
				break
			}
		}
		if hasBLIs && !hasOpenBLIs {
			validationIssues = append(validationIssues, Issue{
				Tier:        1,
				Category:    "lifecycle",
				Message:     fmt.Sprintf("Priority plan %s is in_progress but all linked backlog items are terminal (%d complete/archived); plan should be transitioned to complete", obj.ID, totalBLIs),
				AutoFixable: false,
				FixCommand:  fmt.Sprintf("zqk object promote %s", obj.ID),
			})
		}
	}

	// Invariant: test criteria must be bound to at least one test case
	if kind == objects.KindCriteria && currentState != objects.ObjectStatusArchived && currentState != "rejected" {
		category, _ := objMap[objects.FieldKeyCategory].(string)
		vMethod, _ := objMap["validation_method"].(string)
		if strings.EqualFold(category, "test") || strings.EqualFold(vMethod, "automated_test") {
			deps := options.DependentsLookup(obj.ID)
			hasTestCase := false
			for _, depID := range deps {
				if strings.HasPrefix(depID, "TST-") {
					hasTestCase = true
					break
				}
				if depObj, readErr := options.ObjectLookup(depID); readErr == nil && depObj != nil {
					if depKind, _ := depObj[objects.FieldKeyKind].(string); depKind == objects.KindTestCase {
						hasTestCase = true
						break
					}
				}
			}
			if !hasTestCase {
				validationIssues = append(validationIssues, Issue{
					Tier:        1,
					Category:    "traceability",
					Message:     fmt.Sprintf("Test criterion %s has category=%q / validation_method=%q but is not bound to any test_case object", obj.ID, category, vMethod),
					AutoFixable: false,
				})
			}
		}
	}

	// Invariant: requirements in active/proposed status where all criteria and test cases are complete
	if kind == objects.KindRequirement && (currentState == objects.ObjectStatusActive || currentState == "proposed") {
		critRefs := lifecycle.StringRefsFromAny(objMap[objects.FieldKeyCriteriaRefs])
		if len(critRefs) > 0 {
			allCriteriaMet := true
			checker := objects.GetGlobalStatusChecker()
			for _, cid := range critRefs {
				cObj, cErr := options.ObjectLookup(cid)
				if cErr != nil || cObj == nil {
					allCriteriaMet = false
					break
				}
				cStatus, _ := cObj[objects.FieldKeyStatus].(string)
				if !checker.IsSatisfied(objects.KindCriteria, cStatus) {
					allCriteriaMet = false
					break
				}
			}
			if allCriteriaMet {
				deps := options.DependentsLookup(obj.ID)
				allTestsComplete := true
				for _, depID := range deps {
					if strings.HasPrefix(depID, "TST-") {
						tObj, tErr := options.ObjectLookup(depID)
						if tErr == nil && tObj != nil {
							tStatus, _ := tObj[objects.FieldKeyStatus].(string)
							if tStatus != objects.ObjectStatusComplete && tStatus != objects.ObjectStatusArchived {
								allTestsComplete = false
								break
							}
						}
					}
				}
				if allTestsComplete {
					validationIssues = append(validationIssues, Issue{
						Tier:        1,
						Category:    "lifecycle",
						Message:     fmt.Sprintf("Requirement %s is %s but all criteria and linked test cases are complete; requirement must be transitioned to complete", obj.ID, currentState),
						AutoFixable: false,
						FixCommand:  fmt.Sprintf("zqk object promote %s", obj.ID),
					})
				}
			}
		}
	}

	return validationIssues
}
func checkPolicy(obj *parser.ParsedObject, kind string) []Issue {
	var issues []Issue

	// TODO: Implement policy compliance checking
	// This requires loading policy rules and validating against them

	return issues
}
func checkReferencesWithCache(ctx *cli.Context, obj *parser.ParsedObject, kind string, objectIDCache *ObjectIDCache, storageProvider storage.ObjectStorageProvider) []Issue {
	// Get storage provider if not provided (best effort)
	if storageProvider == nil {
		projectRoot := ctx.ProjectRoot
		projectRoot = ProjectRootOrResolve(projectRoot)
		storageProvider = getStorageProviderForCache(projectRoot)
	}

	refCtx := initializeReferenceCheckContext(ctx, obj, kind, storageProvider)

	var issues []Issue
	for fieldName, refValue := range refCtx.RefFields {
		if refValue == nil {
			continue
		}
		fieldIssues := processReferenceField(refCtx, fieldName, refValue, objectIDCache, true)
		issues = append(issues, fieldIssues...)
	}

	return issues
}
func checkReferences(ctx *cli.Context, obj *parser.ParsedObject, kind string) []Issue {
	refCtx := initializeReferenceCheckContext(ctx, obj, kind, nil)

	var issues []Issue
	for fieldName, refValue := range refCtx.RefFields {
		if refValue == nil {
			continue
		}
		fieldIssues := processReferenceField(refCtx, fieldName, refValue, nil, false)
		issues = append(issues, fieldIssues...)
	}

	return issues
}
func checkDuplicateIDs(_ *cli.Context, objectID, _, filePath string, objectIDCache *ObjectIDCache) []Issue {
	var issues []Issue

	if objectIDCache == nil || objectID == emptyValue {
		return issues
	}

	// Scope to this ID only — never GetEntriesByKind (allocates/copies the whole kind bucket;
	// doc_entry alone is 2k+ entries and contended with async validation under RLock).
	duplicateEntries := objectIDCache.EntriesForID(objectID)
	if len(duplicateEntries) <= 1 {
		return issues
	}

	// Found duplicates - identify which file is the "correct" one
	// Strategy: The file with the most recent mtime is likely the correct one
	// Files with older mtimes or missing required fields are likely stale
	var latestEntry *ObjectIDCacheEntry
	var latestMTime time.Time
	for _, entry := range duplicateEntries {
		if entry.MTime.After(latestMTime) {
			latestMTime = entry.MTime
			latestEntry = entry
		}
	}

	// Check if current file is a duplicate (not the latest)
	if latestEntry != nil && filePath != latestEntry.FilePath {
		issues = append(issues, Issue{
			Tier:        1, // Blocking tier for duplicate-id orphans
			Category:    "registration",
			Message:     fmt.Sprintf("Duplicate object ID '%s' detected. This file appears to be a duplicate of '%s' (which is newer). Quarantine via cleanup-duplicates or zqk object delete --unlink-references; never silent auto-delete.", objectID, latestEntry.FilePath),
			AutoFixable: false,
		})
	}

	return issues
}

// validateAllSpecs validates all object specifications for complete checklists with criteria traceability
func validateAllSpecs(ctx *cli.Context, _ *cobra.Command, projectRoot string, specLoader *objects.SpecLoader) ([]CheckResult, error) {
	stdCtx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(ctx.Profile)

	// Get storage provider using factory (auto-detects backend)
	storageFactory, err := storage.NewStorageFactory(stdCtx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to create storage provider").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create criteria lookup function for traceability
	criteriaLookup := storage.NewCriteriaLookupFunc(storageProvider, true) // internalOnly=true for kernel criteria

	// Discover all object kinds
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	kinds := discoverObjectKinds(processDir)

	var allResults []CheckResult

	// Validate specs for each kind
	for _, kind := range kinds {
		// Load spec with inheritance
		spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
		if err != nil {
			// Spec file doesn't exist or can't be loaded - skip
			logging.Fluent(logger).Debug("Skipping spec validation").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}

		// Same validation path as update-specs --validate and SpecValidator tests (REQ-019 field vetting).
		validationErrors := objects.ValidateLoadedSpec(stdCtx, specLoader, spec, criteriaLookup)

		// Convert validation errors to CheckResult
		if len(validationErrors) > 0 {
			var issues []Issue
			for _, ve := range validationErrors {
				// Determine issue tier based on missing item
				// Missing field_profile_code or critical items are blocking (tier 1)
				// Other missing items are warnings (tier 2)
				tier := 2 // Default to warning
				if ve.MissingItem == "field_profile_code" || ve.MissingItem == "purpose" || ve.MissingItem == "system_usage" {
					tier = 1 // Blocking
				}

				message := ve.Message
				if ve.CriteriaRef != emptyValue {
					message = fmt.Sprintf("%s (see %s)", message, ve.CriteriaRef)
				}

				issues = append(issues, Issue{
					Tier:        tier,
					Category:    "spec_checklist",
					Message:     message,
					AutoFixable: false,
				})
			}

			// Create a CheckResult for the spec (use spec file path as identifier)
			specFilePath := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir, kind+".yaml")
			allResults = append(allResults, CheckResult{
				ObjectID:   kind + ".yaml",
				ObjectKind: "spec",
				FilePath:   specFilePath,
				Issues:     issues,
			})
		}
	}

	return allResults, nil
}

// isHashMismatchFixMode checks if we're in a mode where hash mismatches will be fixed
// This is used to determine if we should check objects sequentially to avoid race conditions
//
//nolint:unused // Helper function - reserved for future use

// isInternalKind checks if a kind has visibility: internal in its spec
// Internal kinds include audit_event, change_journal_entry, metrics, etc.

// Helper functions are now in check_impl_helpers.go
// Output functions are now in check_impl_output.go
// Audit buffer functions are now in check_impl_audit_buffer.go
