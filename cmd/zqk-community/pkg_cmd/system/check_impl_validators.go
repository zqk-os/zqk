package system

import (
	"github.com/lanceman/zqk/pkg/datacell"

	stdcontext "context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

func checkRegistration(obj *parser.ParsedObject, kind string) []Issue {
	var issues []Issue

	// Check required fields
	if obj.ID == emptyValue {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "registration",
			Message:  "Missing required field: id",
		})
	}

	if obj.Kind == emptyValue {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "registration",
			Message:  "Missing required field: kind",
		})
	}

	// Normalize kind for comparison
	// Extract just the kind part (before any " - " separator that might contain extra data)
	objKindValue := strings.TrimSpace(obj.Kind)
	// Look for " - " separator (with spaces) or just " -" or "- " as fallbacks
	separators := []string{" - ", " -", "- "}
	for _, sep := range separators {
		if idx := strings.Index(objKindValue, sep); idx > 0 {
			objKindValue = strings.TrimSpace(objKindValue[:idx])
			break
		}
	}
	objKindNormalized := strings.ToLower(objKindValue)
	expectedKindNormalized := strings.ToLower(strings.TrimSpace(kind))

	// Check if normalized kinds match
	if objKindNormalized == expectedKindNormalized {
		// Kinds match, but check if original had extra text (data quality issue)
		if strings.TrimSpace(obj.Kind) != strings.TrimSpace(kind) {
			issues = append(issues, Issue{
				Tier:     3, // Informational - data quality issue
				Category: "registration",
				Message:  fmt.Sprintf("Kind field contains extra text: %s (expected: %s)", obj.Kind, kind),
			})
		}
		// If they match, no mismatch issue
	} else if objKindNormalized != emptyValue {
		// Kinds don't match - check if directory supports multiple kinds
		// Some directories (e.g., "metrics") support multiple kinds (base_metric, audit_aggregation_metric, command_metric)
		// In this case, use the object's actual kind if it's valid for the directory
		objKindDir := objects.GetDirectoryFromKind(objKindValue)
		expectedKindDir := objects.GetDirectoryFromKind(kind)

		// If both kinds map to the same directory, they're both valid for that directory
		if objKindDir != emptyValue && objKindDir == expectedKindDir {
			// Both kinds are valid for this directory - no mismatch
			// Update the kind to use the object's actual kind
		} else {
			// Kinds don't match and don't share the same directory - report as warning
			issues = append(issues, Issue{
				Tier:     2,
				Category: "registration",
				Message:  fmt.Sprintf("Kind mismatch: expected %s, got %s", kind, obj.Kind),
			})
		}
	}

	// Check ID format matches kind using configurable validator
	// NOTE: Patterns should be loaded once at the start of the check command, not per-object
	// LoadPatterns() will only load if not already loaded, so it's safe to call here as fallback
	idValidator := validation.GetIDValidator()
	// Load patterns if not already loaded (LoadPatterns is idempotent)
	_ = idValidator.LoadPatterns() //nolint:errcheck // Ignore error - will use defaults if load fails
	valid, err := idValidator.ValidateID(obj.ID, kind)
	if err != nil {
		issues = append(issues, Issue{
			Tier:     2,
			Category: "registration",
			Message:  fmt.Sprintf("ID validation error: %v", err),
		})
	} else if !valid {
		validPrefixes := idValidator.GetValidPrefixes(kind)
		prefixMsg := ""
		if len(validPrefixes) > 0 {
			prefixMsg = fmt.Sprintf(" (valid prefixes: %v)", validPrefixes)
		}
		issues = append(issues, Issue{
			Tier:     2,
			Category: "registration",
			Message:  fmt.Sprintf("ID format does not match kind %s%s", kind, prefixMsg),
		})
	}

	return issues
}
func checkLifecycle(obj *parser.ParsedObject, kind string) []Issue {
	var issues []Issue

	// Check status field exists
	status, ok := obj.Properties[objects.FieldKeyStatus].(string)
	if !ok || status == emptyValue {
		issues = append(issues, Issue{
			Tier:     2,
			Category: objects.KindLifecycle,
			Message:  "Missing or invalid status field",
		})
		return issues
	}

	// Use the validator to check lifecycle validity
	// Note: Full lifecycle validation (transitions, preconditions) is done in checkInstanceValidation
	// This function provides a focused lifecycle check
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("") // Default validator
	if validator != nil {
		objMap := make(map[string]any)
		if obj.Properties != nil {
			objMap = obj.Properties
		}

		options := validation.DefaultValidationOptions()
		options.ValidateLifecycle = true
		options.CurrentState = status // Check against itself (no transition)

		// Use background context for lifecycle validation (no context parameter in this function)
		result, err := validator.Validate(pkgctx.NewSystemContext(), objMap, kind, options)
		if err == nil {
			// Convert lifecycle-specific validation errors to issues
			for _, validationError := range result.Errors {
				if validationError.Rule == objects.KindLifecycle {
					tier := 1 // Blocking for lifecycle violations
					issues = append(issues, Issue{
						Tier:     tier,
						Category: objects.KindLifecycle,
						Message:  fmt.Sprintf("%s: %s", validationError.Field, validationError.Message),
					})
				}
			}
		}
	}

	return issues
}
func checkLifecycleWithLoader(obj *parser.ParsedObject, kind string, lifecycleLoader *objects.LifecycleLoader) []Issue {
	var issues []Issue

	// Check status field exists
	status, ok := obj.Properties[objects.FieldKeyStatus].(string)
	if !ok || status == emptyValue {
		issues = append(issues, Issue{
			Tier:     2,
			Category: objects.KindLifecycle,
			Message:  "Missing or invalid status field",
		})
		return issues
	}

	// Use the lifecycle loader to check lifecycle validity
	valid, err := lifecycleLoader.IsValidStatus(kind, status)
	if err == nil && !valid {
		issue := Issue{
			Tier:     1,
			Category: objects.KindLifecycle,
			Message:  fmt.Sprintf("Invalid lifecycle status '%s' for kind '%s'", status, kind),
		}
		// Suggest a fix using a valid status (origin or first available)
		if obj.ID != emptyValue {
			suggested, _ := lifecycleLoader.GetOriginStatus(kind)
			if suggested == emptyValue {
				if lc, loadErr := lifecycleLoader.LoadLifecycle(kind); loadErr == nil && len(lc.Statuses) > 0 {
					suggested = lc.Statuses[0].Value
				}
			}
			if suggested != emptyValue {
				issue.FixCommand = fmt.Sprintf("%s object update %s --field status=%s", paths.CLICommandName, obj.ID, suggested)
				issue.AutoFixable = true
			}
		}
		issues = append(issues, issue)
	}

	return issues
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

	// DEBUG: Log validator input for POLICY-DEBUG-001
	isDebugObject := obj.ID == "POLICY-DEBUG-001"
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	if isDebugObject {
		logging.Fluent(logger).Info("=== POLICY-DEBUG-001 DEBUG: VALIDATOR INPUT ===").
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
	options.ObjectLookup = func(id string) (map[string]any, error) {
		secCtx := pkgctx.NewSystemSecurityContext()
		return lookupProvider.Read(stdCtx, secCtx, id)
	}
	options.ObjectStatusLookup = func(id string) (string, error) {
		obj, err := options.ObjectLookup(id)
		if err != nil {
			return "", err
		}
		status, _ := obj[objects.FieldKeyStatus].(string)
		return status, nil
	}

	// Use provided context or background if not set
	validateCtx := stdCtx
	if validateCtx == nil {
		validateCtx = pkgctx.NewSystemContext()
	}

	// Validate instance using cached validator
	result, err := validator.Validate(validateCtx, objMap, kind, options)

	if isDebugObject {
		if err != nil {
			logging.Fluent(logger).Warn("Validator returned error").
				WithError(err).
				Log()
		} else {
			logging.Fluent(logger).Info("=== POLICY-DEBUG-001 DEBUG: VALIDATOR RESULT ===").
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

	if objectIDCache == nil {
		return issues
	}

	// Get all cache entries and find duplicates by scanning for same ID
	allEntries := objectIDCache.GetAll()
	var duplicateEntries []*ObjectIDCacheEntry
	for _, entry := range allEntries {
		if entry.ID == objectID {
			duplicateEntries = append(duplicateEntries, entry)
		}
	}

	if len(duplicateEntries) <= 1 {
		// No duplicates
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
	if filePath != latestEntry.FilePath {
		issues = append(issues, Issue{
			Tier:        2, // Warning tier
			Category:    "registration",
			Message:     fmt.Sprintf("Duplicate object ID '%s' detected. This file appears to be a duplicate of '%s' (which is newer). Consider deleting this file or updating its ID.", objectID, latestEntry.FilePath),
			AutoFixable: true, // Can be auto-fixed by deleting the duplicate file
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
