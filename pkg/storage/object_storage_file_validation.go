package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/when"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/validation"
)

// ============================================================================
// Validation Functions
// ============================================================================

// validateObjectBeforeCreation validates object before writing
func (f *FileObjectStorage) validateObjectBeforeCreation(ctx context.Context, obj map[string]any, kind string, secCtx *pkgctx.SecurityContext) error {
	// Check for blocking issues before write operations (except for automated kinds)
	id, _ := obj[objects.FieldKeyID].(string)
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, OpCreate, kind, id); err != nil {
		return err
	}

	// Ensure kind field is set correctly (preserve the kind passed in, not ontology)
	obj[objects.FieldKeyKind] = kind

	// Ensure metadata
	f.ensureObjectMetadata(obj, secCtx, true)

	// Validate object (spec, lifecycle, references)
	validateStart := time.Now()
	if err := f.validateObject(ctx, obj, kind, ""); err != nil {
		// Log the error but DO NOT block creation per "membrane" design pattern
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn("object validation failed before creation, but proceeding with write to keep object outside the membrane until resolved").
			Kind(kind).
			ObjectID(id).
			WithError(err).
			Log()

		var _err_83719230 = f.trackPersistenceStep(ctx, secCtx, OpValidateBeforeWrite, PersistenceStepValidateObject, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(validateStart), err)
		if _err_83719230 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83719230).Log()
		}
		// Do not return errfmt.Newf(ConstStreamValidationFailed).Wrap(err) here!
	}
	validateDuration := time.Since(validateStart)
	if validateDuration > 100*time.Millisecond {
		var _err_83719579 = f.trackPersistenceStep(ctx, secCtx, OpValidateBeforeWrite, PersistenceStepValidateObject, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(validateDuration.Nanoseconds()) / 1e6}, validateDuration, nil)
		if _err_83719579 != nil {
			logging.

				// checkForBlockingIssuesBeforeWrite checks for Tier 1 (blocking) issues before write operations
				// Uses the BlockingCheckContext pattern to trigger system-level checks
				// Now highly customizable via workflow profiles and per-kind/per-operation rules
				Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83719579).Log()
		}
	}

	return nil
}

func (f *FileObjectStorage) checkForBlockingIssuesBeforeWrite(ctx context.Context, operationKind, kind, id string) error {
	// Check if this kind should bypass blocking checks (from external config)
	config := GetGlobalBlockingCheckConfig()
	if config.ShouldBypassBlockingCheck(kind) {
		return nil
	}

	// Check if system-level blocking is required for this kind/operation
	if !config.ShouldRequireSystemLevelCheck(kind, operationKind) {
		return nil // Skip system-level check if not required
	}

	// Create blocking check context
	blockingCtx := pkgctx.NewBlockingCheckContext(f.projectRoot, operationKind, kind, id)
	blockingCtx = blockingCtx.WithContext(ctx)

	// Process the context through the listener pipeline
	// This will trigger the registered blocking check handler
	resultChan := pkgctx.ProcessContext(blockingCtx)
	result := <-resultChan

	// Check if processing failed
	if result.FinalError != nil {
		// If check fails, log warning but don't block (best effort)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectValidationBlockingCheckFailedWarn).
			Kind(kind).
			WithError(result.FinalError).
			Log()
		return nil // Don't block on check failures
	}

	// Check result from the handler
	if checkResult, ok := result.FinalResult.(*pkgctx.BlockingCheckResult); ok {
		if checkResult.HasBlockingIssues && len(checkResult.Issues) > 0 {
			// Format blocking issues for error message
			var issueMessages []string
			for i, issue := range checkResult.Issues {
				if i < 5 { // Limit to first 5 issues
					issueMessages = append(issueMessages, fmt.Sprintf("%s (%s): %s", issue.ObjectID, issue.ObjectKind, issue.Message))
				}
			}
			if len(checkResult.Issues) > 5 {
				issueMessages = append(issueMessages, fmt.Sprintf(ConstStreamEllipsisAndIntMoreBlockingIssues, len(checkResult.Issues)-5))
			}

			return errfmt.Errorf(
				ConstStreamBlockingIssuesDetectedResolveCriticalViolations+
					ConstStreamRunZqkSystemCheckToSeeAllIssuesN+
					"  Issues: %s",
				strings.Join(issueMessages, "; "))
		}
	}

	return nil
}

// validateObject validates an object against its spec, lifecycle, and references
// CRITICAL: This is called before ALL write operations (Create, Update, Move, Migration)
// Validation errors are immediately registered and prevent persistence of invalid objects
func (f *FileObjectStorage) validateObject(ctx context.Context, obj map[string]any, kind, currentState string) error {
	// Early detection: Check for minimal required fields before full validation
	// This catches partial/incomplete data immediately
	if err := f.detectPartialData(obj, kind); err != nil {
		// Register validation error immediately for partial/incomplete data
		f.registerValidationError(ctx, obj, kind, "partial_data", err.Error())
		return errfmt.Newf(ConstStreamPartialOrIncompleteDataDetected).Wrap(err)
	}

	// Un-spoofable Agent Task Security Gate
	if kind == "agent_task" {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if objects.GetGlobalStatusChecker().IsTerminal(kind, status) {
			commitHash, _ := obj[objects.FieldKeyCommitHash].(string)
			if commitHash == "" {
				return errfmt.Errorf("Security Gate: agent_task cannot transition to terminal status without a valid commit_hash")
			}

			// Verify commit hash exists in git
			cmd := exec.CommandContext(ctx, "git", "cat-file", "-t", commitHash)
			cmd.Dir = f.projectRoot
			out, err := cmd.Output()
			if err != nil || strings.TrimSpace(string(out)) != "commit" {
				return errfmt.Errorf("Security Gate: commit_hash '%s' is not a valid commit in the repository", commitHash)
			}

			// Verify all task_steps are completed
			if steps, ok := obj[objects.FieldKeyTaskSteps].([]any); ok {
				for i, stepAny := range steps {
					if stepMap, ok := stepAny.(map[string]any); ok {
						stepStatus, _ := stepMap[objects.FieldKeyStatus].(string)
						if !objects.GetGlobalStatusChecker().IsTerminal("agent_task", stepStatus) && stepStatus != "skipped" {
							return errfmt.Errorf("Security Gate: agent_task cannot transition to terminal status because task_step %d is '%s'", i+1, stepStatus)
						}
					}
				}
			}
		}
	}

	// 1. Spec validation using GoValidator
	// ITEM-621: When --force is set on "zqk object update", skip lifecycle transition validation so status can be overridden
	validateLifecycle := !pkgctx.GetForceLifecycleOverride(ctx)
	options := &validation.ValidationOptions{
		CurrentState:          currentState,
		ValidateLifecycle:     validateLifecycle,
		ValidateSemanticTypes: true,
	}
	options.ObjectLookup = func(id string) (map[string]any, error) {
		secCtx := pkgctx.NewSystemSecurityContext()
		return f.Read(ctx, secCtx, id)
	}
	options.ObjectStatusLookup = func(id string) (string, error) {
		obj, err := options.ObjectLookup(id)
		if err != nil {
			return "", err
		}
		status, _ := obj[objects.FieldKeyStatus].(string)
		return status, nil
	}
	if fn := pkgctx.GetValidationProgress(ctx); fn != nil {
		options.ProgressCallback = func(stage, message string) { fn(stage, message) }
	}

	// Inject hook for shockwave cascade deletion of dependent objects
	options.OnValidationFailure = func(failObj map[string]any, failKind string, errors []validation.ValidationError) {
		// Removed asynchronous cascade delete of the object itself to prevent
		// race conditions during tests and accidental deletion of user data.
	}

	result, err := f.validator.Validate(ctx, obj, kind, options)
	if err != nil {
		// Register validation error immediately
		f.registerValidationError(ctx, obj, kind, ConstStreamValidationFailure, err.Error())
		return errfmt.Newf(ConstStreamSpecValidationFailed).Wrap(err)
	}

	if !result.IsValid {
		// Check tier configuration to determine which errors should block saves
		// Use the blocking check config which now includes validation tier blocking
		blockingConfig := GetGlobalBlockingCheckConfig()
		blockingErrors := blockingConfig.GetBlockingValidationErrors(result.Errors, kind, "")
		nonBlockingErrors := getNonBlockingValidationErrors(result.Errors, blockingConfig, kind, "")

		// Register all validation errors immediately (both blocking and non-blocking)
		// Log all errors for debugging (especially in test scenarios)
		for _, validationErr := range result.Errors {
			errorMsg := fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Message)
			// Register each validation error immediately
			f.registerValidationError(ctx, obj, kind, validationErr.Rule, errorMsg)
			// Also log to help with debugging (especially in tests)
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Debug(LogEventStorageObjectValidationErrorDetailDebug).
				String("field", validationErr.Field).
				String(ConstStreamValidationRule, validationErr.Rule).
				String("message", validationErr.Message).
				Kind(kind).
				ObjectID(getObjectID(obj)).
				Log()
		}

		// Check per-kind rules for allowing non-blocking errors
		allowNonBlocking := false
		if kindRules, ok := blockingConfig.PerKindRules[kind]; ok {
			allowNonBlocking = kindRules.AllowNonBlockingErrors
		}

		// If there are blocking errors, prevent save (UNLESS we are creating)
		if len(blockingErrors) > 0 {
			// Per user directive: don't fail 'create' unless vital info missing (which is checked in detectPartialData).
			// Validation should happen during promotion, so we downgrade blocking errors to non-blocking during creation.
			// Invalid create statuses are coerced to lifecycle origin in ensureCreateLifecycleStatus before validate.
			if currentState == "" {
				nonBlockingErrors = append(nonBlockingErrors, blockingErrors...)
			} else {
				var blockingMessages []string
				for _, err := range blockingErrors {
					tier := blockingConfig.GetTierForRule(err.Rule)
					blockingMessages = append(blockingMessages, fmt.Sprintf(ConstStreamStrTierIntStr, err.Field, tier, err.Message))
				}
				return errfmt.Errorf(ConstStreamValidationErrorsBlockSaveStr, strings.Join(blockingMessages, "; "))
			}
		}

		// If there are only non-blocking errors, log them but allow save (unless per-kind rules override)
		if len(nonBlockingErrors) > 0 && allowNonBlocking {
			var nonBlockingMessages []string
			for _, err := range nonBlockingErrors {
				tier := blockingConfig.GetTierForRule(err.Rule)
				nonBlockingMessages = append(nonBlockingMessages, fmt.Sprintf(ConstStreamStrTierIntStr, err.Field, tier, err.Message))
			}
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectValidationNonBlockingWorkflowWarn).
				ObjectID(getObjectID(obj)).
				Kind(kind).
				String("errors", strings.Join(nonBlockingMessages, "; ")).
				Log()
		}

		// Allow save to proceed (only non-blocking errors)
		return nil
	}

	// 2. Reference validation
	refValidateStart := time.Now()
	status, _ := obj[objects.FieldKeyStatus].(string)
	if status != "error" {
		if err := f.validateReferences(obj, kind); err != nil {
			var _err_83726556 = f.trackPersistenceStep(ctx, nil, OpValidateObject, PersistenceStepValidateReferences, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: getObjectID(obj)}, nil, time.Since(refValidateStart), err)
			if _err_83726556 !=
				// Register reference validation error immediately
				nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83726556).Log()
			}

			f.registerValidationError(ctx, obj, kind, "reference", err.Error())

			// Check if reference validation errors should block saves
			blockingConfig := GetGlobalBlockingCheckConfig()
			tier := blockingConfig.GetTierForRule("reference")

			// Auto-detect batch creation mode:
			// 1. Cache checker is set (explicit batch mode via --cascade or scenario builder)
			// 2. Referenced objects exist in cache but not on disk (being created in same batch)
			// Lock-free read using atomic.Value
			var cacheCheckerValue func(string) (string, bool)
			if val := cacheChecker.Load(); val != nil {
				cacheCheckerValue = val.(func(string) (string, bool))
			}
			isBatchMode := cacheCheckerValue != nil
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Info(LogEventStorageObjectValidationBatchModeInfo).
				ObjectID(getObjectID(obj)).
				Kind(kind).
				Bool("is_batch_mode", isBatchMode).
				Bool(ConstStreamCacheCheckerSet, cacheCheckerValue != nil).
				String(ConstStreamCacheCheckerAddr, fmt.Sprintf("%p", cacheCheckerValue)).
				Log()
			if !isBatchMode {
				// Check if referenced objects are in cache but not yet on disk
				// This indicates they're being created in the same batch
				// Use the captured cacheCheckerValue for consistency
				isBatchMode = f.detectBatchCreationFromCache(ctx, obj, kind)
			}

			// During batch creation, reference validation is non-blocking
			// Objects are created in hierarchical order, so references will resolve as the batch completes
			if isBatchMode {
				// Batch creation mode - log but allow save (references will resolve in hierarchical order)
				StorageLog(logger).Warn(LogEventStorageObjectValidationBatchRefWarn).
					ObjectID(getObjectID(obj)).
					Kind(kind).
					WithError(err).
					Log()
				return nil
			}

			// Normal mode - check tier configuration
			if blockingConfig.IsBlockingTier(tier, kind, "") {
				return errfmt.Errorf(ConstStreamReferenceValidationFailedTierIntBlocksSaveErr, tier, err)
			}

			// Non-blocking reference error - log but allow save
			StorageLog(logger).Warn(LogEventStorageObjectValidationNonBlockingRefWarn).
				ObjectID(getObjectID(obj)).
				Kind(kind).
				WithError(err).
				Log()
			return nil
		}
	}
	refValidateDuration := time.Since(refValidateStart)
	if refValidateDuration > 100*time.Millisecond {
		var _err_83729117 = f.trackPersistenceStep(ctx, nil, OpValidateObject, PersistenceStepValidateReferences, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: getObjectID(obj)}, map[string]any{"duration_ms": float64(refValidateDuration.Nanoseconds()) / 1e6}, refValidateDuration, nil)
		if _err_83729117 != nil {
			logging.

				// detectBatchCreationFromCache checks if referenced objects exist in cache but not on disk
				// This indicates they're being created in the same batch, so we should allow the reference
				// Uses cacheChecker if available, otherwise returns false (can't access cache without circular dependency)
				Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83729117).Log()
		}
	}

	return nil
}

func (f *FileObjectStorage) detectBatchCreationFromCache(ctx context.Context, obj map[string]any, kind string) bool {
	// If cacheChecker is available, use it to check if referenced objects are in cache
	// This is the primary auto-detection mechanism
	// Lock-free read using atomic.Value
	var cacheCheckerValue func(string) (string, bool)
	if val := cacheChecker.Load(); val != nil {
		cacheCheckerValue = val.(func(string) (string, bool))
	}
	if cacheCheckerValue != nil {
		// Extract reference fields
		yamlParser := parser.NewYAMLParser()
		refFields := yamlParser.ExtractReferenceFields(obj)

		// Check if any referenced objects are in cache but file doesn't exist on disk
		for fieldName, refValue := range refFields {
			if refValue == nil {
				continue
			}

			// Skip commit_refs (Git commit hashes, not object IDs)
			if fieldName == objects.FieldKeyCommitRefs {
				continue
			}

			// Handle both single references (_ref) and lists (_refs)
			var refIDs []string
			switch v := refValue.(type) {
			case string:
				if v != emptyValue {
					refIDs = []string{v}
				}
			case []any:
				for _, item := range v {
					if str, ok := item.(string); ok && str != emptyValue {
						refIDs = append(refIDs, str)
					}
				}
			case []string:
				refIDs = v
			}

			// Check each referenced ID
			for _, refID := range refIDs {
				// Use cacheChecker to check if object is in cache
				cachedFilePath, existsInCache := cacheCheckerValue(refID)
				if existsInCache && cachedFilePath != emptyValue {
					// Object is in cache - check if file exists on disk
					if _, err := os.Stat(cachedFilePath); os.IsNotExist(err) {
						// Object is in cache but file doesn't exist yet
						// This indicates it's being created in the same batch
						return true
					}
				}
			}
		}
	}

	// Can't access cache directly without circular dependency
	// Return false - explicit cascade mode or cacheChecker setup is required
	return false
}

// getObjectID extracts object ID from object map
func getObjectID(obj map[string]any) string {
	if obj == nil {
		return "<unknown>"
	}
	id, _ := obj[objects.FieldKeyID].(string)
	if id == emptyValue {
		return "<unknown>"
	}
	return id
}

// getNonBlockingValidationErrors filters validation errors to only those that should NOT block saves
func getNonBlockingValidationErrors(validationErrors []validation.ValidationError, blockingConfig *BlockingCheckConfig, kind, operation string) []validation.ValidationError {
	var nonBlockingErrors []validation.ValidationError
	for _, err := range validationErrors {
		tier := blockingConfig.GetTierForRule(err.Rule)
		if !blockingConfig.IsBlockingTier(tier, kind, operation) {
			nonBlockingErrors = append(nonBlockingErrors, err)
		}
	}
	return nonBlockingErrors
}

// detectPartialData detects partial or incomplete data early
// This catches issues before full validation runs
func (f *FileObjectStorage) detectPartialData(obj map[string]any, kind string) error {
	// Check for required top-level fields
	if kind == emptyValue {
		return errfmt.Errorf(ConstStreamObjectMissingRequiredFieldKind)
	}

	id, _ := obj[objects.FieldKeyID].(string)
	if id == emptyValue {
		return errfmt.Errorf(ConstStreamObjectMissingRequiredFieldId)
	}

	// Check if object is empty (shouldn't happen, but be safe)
	if len(obj) == 0 {
		return errfmt.Errorf(ConstStreamObjectIsNilOrEmpty)
	}

	// Check if kind matches expected kind (if kind was inferred)
	if objKind := objects.GetString(obj, objects.FieldKeyKind); objKind != emptyValue && objKind != kind {
		return errfmt.Errorf(ConstStreamKindMismatchObjectHasKindStrButExpectedStr, objKind, kind)
	}

	return nil
}

// registerValidationError registers a validation error immediately
// This ensures validation errors are tracked even if the object is not persisted
func (f *FileObjectStorage) registerValidationError(_ context.Context, obj map[string]any, kind, rule, message string) {
	// Extract object ID for logging
	objectID, _ := obj[objects.FieldKeyID].(string)
	if objectID == emptyValue {
		objectID = "<unknown>"
	}

	// Log validation error immediately
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn(LogEventStorageObjectValidationBeforePersistWarn).
		ObjectID(objectID).
		Kind(kind).
		String(ConstStreamValidationRule, rule).
		String("message", message).
		Log()

	// TODO: In the future, we could also update validation state cache here
	// to track validation errors for objects that failed validation
	// This would help with reporting and debugging
}

// validateReferences validates that all referenced objects exist
func (f *FileObjectStorage) validateReferences(obj map[string]any, kind string) error {
	// Extract reference fields
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)

	// Track which objects we've already checked
	checkedRefs := make(map[string]bool)

	// Validate each reference field
	for fieldName, refValue := range refFields {
		if refValue == nil {
			continue
		}

		// Handle both single references (_ref) and lists (_refs)
		var refIDs []string
		switch v := refValue.(type) {
		case string:
			if v != emptyValue {
				refIDs = []string{v}
			}
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && str != emptyValue {
					refIDs = append(refIDs, str)
				}
			}
		case []string:
			refIDs = v
		}

		// Validate each referenced ID
		for _, refID := range refIDs {
			// Skip if we've already checked this reference
			if checkedRefs[refID] {
				continue
			}
			checkedRefs[refID] = true

			// Skip reference validation for commit_refs (Git commit hashes, not object IDs)
			if fieldName == objects.FieldKeyCommitRefs {
				continue
			}

			// For code_reference objects, be lenient about missing work item references
			// (work items may be created later, or references may be to external items)
			if kind == objects.KindCodeReference && (fieldName == objects.FieldKeyBacklogItemRefs ||
				fieldName == objects.FieldKeyMilestoneRefs || fieldName == objects.FieldKeyGoalRefs ||
				fieldName == objects.FieldKeyWorkstreamRefs || fieldName == objects.FieldKeyRequirementRefs) {
				// Check if reference exists, but don't fail if it doesn't
				refKind := f.idValidator.InferKindFromID(refID)
				if refKind != emptyValue {
					refFilePath, err := f.getObjectFilePath(refID, refKind)
					if err == nil {
						if _, err := os.Stat(refFilePath); err == nil {
							// Reference exists, continue validation
							continue
						}
					}
					// Reference doesn't exist, but that's okay for code_reference work item refs
					continue
				}
			}

			// For change_journal_entry objects, reference validation is deferred to system check
			// (referenced objects may not exist yet during creation, or may have been deleted)
			if kind == objects.KindChangeJournalEntry && fieldName == objects.FieldKeyObjectRef {
				// Skip reference validation for change_journal_entry.object_ref
				// These are created automatically and validation is deferred to system check
				// where all objects exist and can be properly validated
				continue
			}

			// Infer kind from ID, handling namespaced references
			refKind := ""
			actualRefID := refID                   // Use the full reference string by default
			var parsed *validation.ParsedNamespace // Declare parsed outside the else block for use later

			// Special case: Account references must be handled BEFORE ParseNamespace
			// because ParseNamespace treats "account:username" as short format and extracts
			// ObjectID="username", but we need the full "account:username" for getObjectFilePath
			// to construct the correct filename (account-{username}.yaml)
			if strings.HasPrefix(refID, "account:") {
				// Account reference (special case: "account:username" format)
				refKind = objects.KindAccount
				// For accounts, keep the full ID with "account:" prefix for getObjectFilePath
				// getObjectFilePath needs it to construct the correct filename (account-{username}.yaml)
				actualRefID = refID // Keep full ID for accounts
			} else {
				// First, check if this is a namespaced reference (e.g., "zqk:kernel:goal:GOAL-123")
				parsed = validation.ParseNamespace(refID)
				when.When(func() bool { return parsed != nil && parsed.ObjectType != emptyValue }).Then(func() {
					// Namespaced reference - use the object type directly
					refKind = parsed.ObjectType
					actualRefID = parsed.ObjectID
				}).OrElseWhen(func() bool { return strings.Contains(refID, ":") }).Then(func() {
					// Handle "kind:id" format (e.g., "audit_event:AUD-32", "goal:GOL-001")
					// This is used by change_journal_entry.object_ref and other reference fields
					// Check if it's a short namespace format (e.g., "goal:GOAL-123" which assumes zqk:kernel)
					parts := strings.SplitN(refID, ":", 2)
					if len(parts) == 2 {
						// Could be "kind:id" or short namespace format
						// Try to infer kind from the first part
						refKind = parts[0]
						actualRefID = parts[1]
						// Validate that this is a valid kind (not a namespace layer)
						// Use configurable namespace layers instead of hardcoded values
						config := validation.GetGlobalNamespacesConfig()
						isValidLayer := false
						if config != nil {
							isValidLayer = config.IsValidNamespaceLayer(refKind)
						} else {
							// Fallback for backward compatibility - use config-driven lookup
							// Check if refKind matches known namespace layers from config
							// For now, use hardcoded fallback but this should be config-driven
							isValidLayer = (refKind == "zqk" || refKind == "domain" || refKind == "integration")
						}
						if !isValidLayer {
							// Likely "kind:id" format, use as-is
						} else {
							// This might be a namespace layer, fall back to inferring from ID
							refKind = f.idValidator.InferKindFromID(refID)
							actualRefID = refID
						}
					} else {
						// Fallback to inferring from ID
						refKind = f.idValidator.InferKindFromID(refID)
					}
				}).OrElse(func() {
					// Legacy format - no namespace, just ID
					refKind = f.idValidator.InferKindFromID(refID)
				}).Run()
			}

			if refKind == emptyValue {
				// Treat as a validation issue, not a hard failure.
				// In real datasets we sometimes see placeholder strings (e.g., "required") or
				// other non-ID values in *_ref fields. Failing hard here aborts persistence and
				// makes system check appear hung. Skip reference validation for this value.
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Warn(LogEventStorageObjectValidationSkipInferRefKindWarn).
					String("ref_id", refID).
					String("field", fieldName).
					String("object_kind", kind).
					String("note", ConstStreamReferenceValueIsNotAValidObjectIdOrNamespaced).
					Log()
				continue
			}

			// Cross-namespace validation (basic check)
			// TODO: Full cross-namespace validation requires loading namespace objects from registry
			// to check integration rules. For now, we allow all cross-namespace references.
			// Enhanced validation can be added when namespace objects are loaded from storage.
			if parsed != nil && parsed.NamespaceID != emptyValue {
				// Namespaced reference detected - could add validation here
				// For now, allow all cross-namespace references (backward compatibility)
			}

			// Check if referenced object exists
			// First, check ObjectIDCache (for objects created earlier in the same batch)
			// This allows batch creation where objects reference each other
			// Objects are created in hierarchical order, so earlier objects should be in cache
			// Lock-free read using atomic.Value
			var cacheCheckerValue func(string) (string, bool)
			if val := cacheChecker.Load(); val != nil {
				cacheCheckerValue = val.(func(string) (string, bool))
			}
			if cacheCheckerValue != nil {
				cachedFilePath, existsInCache := cacheCheckerValue(actualRefID)
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Debug(LogEventStorageObjectValidationCacheCheckerResultDebug).
					String("ref_id", refID).
					String("actual_ref_id", actualRefID).
					String("field_name", fieldName).
					Bool(ConstStreamExistsInCache, existsInCache).
					String(ConstStreamCachedFilePath, cachedFilePath).
					Log()
				if existsInCache {
					// Object exists in cache (idStream, referenceCache, or ObjectIDCache)
					// In batch creation mode, this means the object will be created
					if cachedFilePath != emptyValue {
						// We have a file path - verify file exists at cached path
						if _, err := os.Stat(cachedFilePath); err == nil {
							// Reference exists and file is accessible - validation passes
							continue
						}
						// Cache entry exists but file doesn't - this is normal in batch creation mode
						// (file being written concurrently). Allow the reference - batch mode detection
						// will handle it properly. Continue to next reference.
						continue
					}
					// Object exists in cache but path is unknown (e.g., in referenceCache but not yet created)
					// In batch creation mode, this is expected - object will be created in hierarchical order
					// Allow the reference - validation tier config will determine if it blocks
					// Skip file check since object is in cache
					continue
				}
				// If cache checker is available but object not in cache, we're in batch creation mode
				// The object will be created later in the batch (hierarchical order), so allow the reference
				// Return error but validation tier config will determine if it blocks (Tier 1,2 can be non-blocking)
			}

			// Strategic kinds (goal, priority_plan, etc.) are database-only and do not have files.
			// The graph storage layer validates their references, so the file layer should skip them.
			if isStrategicKind(refKind) {
				continue
			}

			// Fallback: Use getObjectFilePath + os.Stat for direct file check (avoids kind inference issues)
			refFilePath, err := f.getObjectFilePath(actualRefID, refKind)
			if err != nil {
				return errfmt.Errorf(ConstStreamInvalidReferenceStrInFieldStrErr, refID, fieldName, err)
			}

			// Check if file exists
			fileInfo, err := os.Stat(refFilePath)
			if os.IsNotExist(err) {
				// If cache checker is available, we're in batch creation mode
				// Object may be created later in the batch (hierarchical order), so allow the reference
				// Return error but validation tier config will determine if it blocks
				if cacheCheckerValue != nil {
					// Cache checker available = batch creation mode
					// Object will be created later, so allow reference (validation tier will decide if it blocks)
					// This enables batch creation where objects reference each other in hierarchical order
				}
				// Provide diagnostic information to help debug the issue
				var diagnosticMsg strings.Builder
				fmt.Fprintf(&diagnosticMsg, ConstStreamReferencedObjectStrKindStrInFieldStrDoesNotExist, refID, refKind, fieldName, refFilePath)

				// Check if directory exists but file doesn't
				refDir := filepath.Dir(refFilePath)
				if dirInfo, dirErr := os.Stat(refDir); dirErr == nil && dirInfo.IsDir() {
					fmt.Fprintf(&diagnosticMsg, ConstStreamNDirectoryExistsStr, refDir)
					fmt.Fprintf(&diagnosticMsg, ConstStreamNExpectedFileStr, filepath.Base(refFilePath))
					diagnosticMsg.WriteString(ConstStreamNDiagnosticSteps)
					fmt.Fprintf(&diagnosticMsg, ConstStreamN1CheckIfFileExistsLsLaStr, refDir)
					fmt.Fprintf(&diagnosticMsg, ConstStreamN2VerifyObjectExistsZqkObjectGetStr, refID)
					config := GetStorageConfig()
					fmt.Fprintf(&diagnosticMsg, ConstStreamN3SearchForFileFindStrNameStrstr, paths.ProcessDir, refID, config.YAMLExtension)
					diagnosticMsg.WriteString(ConstStreamNIfObjectExistsButFileDoesnTThisMayIndicate)
					diagnosticMsg.WriteString(ConstStreamNObjectIsStoredInGraphBackendInsteadOfFileBackend)
					diagnosticMsg.WriteString(ConstStreamNFilePathResolutionIssueCheckNamespaceOrBucketing)
					diagnosticMsg.WriteString(ConstStreamNObjectWasCreatedOutsideFileStorage)
				} else {
					fmt.Fprintf(&diagnosticMsg, ConstStreamNDirectoryDoesNotExistStr, refDir)
					diagnosticMsg.WriteString(ConstStreamNDiagnosticSteps)
					fmt.Fprintf(&diagnosticMsg, ConstStreamN1VerifyObjectExistsZqkObjectGetStr, refID)
					config := GetStorageConfig()
					fmt.Fprintf(&diagnosticMsg, ConstStreamN2SearchForFileFindStrNameStrstr, paths.ProcessDir, refID, config.YAMLExtension)
					fmt.Fprintf(&diagnosticMsg, ConstStreamN3CheckIfKindDirectoryExistsLsLaStr, filepath.Dir(refDir))
				}

				return errfmt.Errorf("%s", diagnosticMsg.String())
			} else if err != nil {
				// File path exists but there's another error (permissions, etc.)
				return errfmt.Errorf(ConstStreamReferencedObjectStrKindStrInFieldStrExistsAtStr, refID, refKind, fieldName, refFilePath, err)
			}

			// File exists - verify it's actually a file (not a directory)
			if fileInfo != nil && fileInfo.IsDir() {
				return errfmt.Errorf(ConstStreamReferencedObjectStrKindStrInFieldStrPathStr, refID, refKind, fieldName, refFilePath)
			}
		}
	}

	return nil
}

// normalizeObjectValues normalizes values in an object to ensure correct types
// This is especially important for number fields where ints should be float64
func (f *FileObjectStorage) normalizeObjectValues(obj map[string]any, kind string) {
	fieldRegistry := objects.GetGlobalFieldRegistry()
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		// If fields cannot be loaded, skip normalization (best effort)
		return
	}

	// Create a map of field name to field info for quick lookup
	fieldMap := make(map[string]*objects.FieldInfo)
	for i := range kindFields.AllFields {
		field := &kindFields.AllFields[i]
		fieldMap[field.Name] = field
	}

	// Normalize each field value
	for fieldName, value := range obj {
		fieldInfo, ok := fieldMap[fieldName]
		if !ok {
			continue // Field not found in spec, skip
		}

		// Convert int to float64 for number/float types
		if fieldInfo.Type == "number" || fieldInfo.Type == "float" {
			switch v := value.(type) {
			case int:
				obj[fieldName] = float64(v)
			case int32:
				obj[fieldName] = float64(v)
			case int64:
				obj[fieldName] = float64(v)
			}
		}

		// Convert time.Time to string for timestamp types
		// YAML unmarshaler (gopkg.in/yaml.v3) automatically converts ISO 8601 strings to time.Time,
		// but our validation and many parts of the system expect RFC3339 strings.
		if fieldInfo.SemanticType == "timestamp" || fieldInfo.Type == "string" {
			if t, ok := value.(time.Time); ok {
				obj[fieldName] = t.Format(time.RFC3339)
			}
		}
	}
}
