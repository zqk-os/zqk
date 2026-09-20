package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/when"
)

func (f *FileObjectStorage) detectBatchCreationFromCache(_ context.Context, obj map[string]any, _ string) bool {
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

			// Skip leftover non-kernel *_ref names (git hashes, paths, tokens).
			if objects.IsLeftoverNonKernelRefField(fieldName) {
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
					if _, err := fileutil.Stat(cachedFilePath); fileutil.IsNotExist(err) {
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

			// Skip leftover non-kernel *_ref names (git hashes, paths, tokens).
			if objects.IsLeftoverNonKernelRefField(fieldName) || fieldName == objects.FieldKeyCommitHashes {
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
						if _, err := fileutil.Stat(refFilePath); err == nil {
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

			// Cross-plane validation: CAS object cannot reference draft-plane-only object.
			subjStatus := objects.GetString(obj, objects.FieldKeyStatus)
			checker := objects.GetGlobalStatusChecker()
			isSubjectDraftPlane := (checker != nil && subjStatus != "" && checker.IsPreliminary(kind, subjStatus)) ||
				caspkg.ParkCriteriaWithoutCategory(kind, obj) ||
				caspkg.ParkObjectWithoutDescription(kind, obj)
			if !isSubjectDraftPlane && IsDraftPlaneOnly(f.projectRoot, refKind, actualRefID) {
				return errfmt.Errorf("CAS object %s (%s) cannot reference draft-plane object %s in field %s: cross-plane reference prohibited", objects.GetString(obj, objects.FieldKeyID), kind, refID, fieldName)
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
						if _, err := fileutil.Stat(cachedFilePath); err == nil {
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
			fileInfo, err := fileutil.Stat(refFilePath)
			if fileutil.IsNotExist(err) {
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
				if dirInfo, dirErr := fileutil.Stat(refDir); dirErr == nil && dirInfo.IsDir() {
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
