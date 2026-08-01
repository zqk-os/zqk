package system

import (
	"fmt"
	"maps"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"

	"github.com/lanceman/zqk/pkg/objects"
)

// ExecuteFixCommandOnly runs the issue's FixCommand (e.g. for registration, lifecycle, policy).
// Used when the issue has a fix command but is not instance_validation so it wasn't handled by FixInstanceValidationIssueWithStorage.
func (saf *SpecBasedAutoFixer) ExecuteFixCommandOnly(
	fixCtx *AutoFixContext,
	issue Issue,
	storageProvider storage.ObjectStorageProvider,
) (bool, string) {
	if storageProvider == nil || issue.FixCommand == emptyValue {
		return false, ""
	}
	return executeFixCommand(fixCtx.Ctx, fixCtx, issue, storageProvider, saf.specLoader)
}

// FixInstanceValidationIssue attempts to fix an instance validation issue using the spec
// Returns (success, fixMessage, updatedObject)
//
// Deprecated: Use FixInstanceValidationIssueWithStorage to avoid redundant storage factory creation
func (saf *SpecBasedAutoFixer) FixInstanceValidationIssue(
	fixCtx *AutoFixContext,
	issue Issue,
) (bool, string, map[string]any) {
	// Create storage provider for backward compatibility
	projectRoot := getProjectRootFromContext(saf.ctx)
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil || storageFactory == nil {
		logging.Fluent(saf.logger).Debug("Failed to create storage factory for fix command").
			String("object_id", fixCtx.Obj.ID).
			WithError(err).
			Log()
		return saf.fixInstanceValidationIssueLayer1(fixCtx, issue)
	}
	storageProvider := storageFactory.GetStorage()
	return saf.FixInstanceValidationIssueWithStorage(fixCtx, issue, storageProvider)
}

// FixInstanceValidationIssueWithStorage attempts to fix an instance validation issue using the spec
// Accepts a storage provider to avoid redundant creation
// Returns (success, fixMessage, updatedObject)
func (saf *SpecBasedAutoFixer) FixInstanceValidationIssueWithStorage(
	fixCtx *AutoFixContext,
	issue Issue,
	storageProvider storage.ObjectStorageProvider,
) (bool, string, map[string]any) {
	// Only handle instance_validation category
	if issue.Category != "instance_validation" {
		return false, "", nil
	}

	// Layer 2: Check if fix command exists (lifecycle violations, reference linking)
	if issue.FixCommand != emptyValue {
		// Execute fix command synchronously
		if storageProvider == nil {
			logging.Fluent(saf.logger).Debug("Storage provider is nil for fix command, skipping").
				String("object_id", fixCtx.Obj.ID).
				Log()
			// Fall through to Layer 1 (simple fixes)
		} else {
			logging.Fluent(saf.logger).Debug("Executing fix command").
				String("object_id", fixCtx.Obj.ID).
				String("command", issue.FixCommand).
				Log()
			success, msg := executeFixCommand(fixCtx.Ctx, fixCtx, issue, storageProvider, saf.specLoader)
			if success {
				logging.Fluent(saf.logger).Info("Fix command executed successfully").
					String("object_id", fixCtx.Obj.ID).
					String("message", msg).
					Log()
				// Fix command executed successfully - executeFixCommand already updated the object
				// Return nil to signal that applySpecFix should NOT be called
				return true, msg, nil
			} else {
				logging.Fluent(saf.logger).Debug("Fix command execution failed").
					String("object_id", fixCtx.Obj.ID).
					String("command", issue.FixCommand).
					Log()
			}
		}
		// If fix command execution fails, fall through to Layer 1 (simple fixes)
	}

	return saf.fixInstanceValidationIssueLayer1(fixCtx, issue)
}

// fixInstanceValidationIssueLayer1 handles Layer 1: Simple spec-based fixes (missing fields with defaults)
// Separated for clarity and reuse
func (saf *SpecBasedAutoFixer) fixInstanceValidationIssueLayer1(
	fixCtx *AutoFixContext,
	issue Issue,
) (bool, string, map[string]any) {
	// Parse the issue message to extract field name
	parts := strings.SplitN(issue.Message, ":", 2)
	if len(parts) < 2 {
		return false, "", nil
	}

	fieldName := strings.TrimSpace(parts[0])
	errorMessage := strings.TrimSpace(parts[1])

	// Load spec for the object kind
	fixKind := determineFixKind(fixCtx)
	if fixKind == emptyValue {
		return false, "", nil
	}

	spec, err := saf.specLoader.LoadSpecWithInheritance(fixKind + ".yaml")
	if err != nil {
		logging.Fluent(saf.logger).Debug("Cannot auto-fix: spec not found").
			Kind(fixKind).
			WithError(err).
			Log()
		return false, "", nil
	}

	// Check if field exists in spec
	fieldDef, fieldExists := spec.ResolvedFields[fieldName]
	if !fieldExists {
		return false, "", nil
	}

	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		return false, "", nil
	}

	// Convert object to map for modification
	// Start with a copy of the full object so we preserve all fields
	objMap := make(map[string]any)
	maps.Copy(objMap, fixCtx.Obj.Properties)

	// Ensure we have the object ID and kind in the map (needed for Update)
	if fixCtx.Obj.ID != emptyValue {
		objMap[objects.FieldKeyID] = fixCtx.Obj.ID
	}
	if fixCtx.Obj.Kind != emptyValue {
		objMap[objects.FieldKeyKind] = fixCtx.Obj.Kind
	}

	// Layer 1: Simple spec-based fixes (missing fields with defaults)
	// Use a helper to attempt fixes in order until one succeeds
	fixed, fixMessage := saf.attemptFixes(fixCtx, fixKind, fieldName, errorMessage, objMap, fieldMap)

	if !fixed {
		logging.Fluent(saf.logger).Debug("Could not auto-fix issue").
			String("object_id", fixCtx.Obj.ID).
			String("field", fieldName).
			String("error_message", errorMessage).
			String("category", issue.Category).
			Log()
		return false, "", nil
	}

	logging.Fluent(saf.logger).Info("Spec-based auto-fix prepared").
		String("object_id", fixCtx.Obj.ID).
		String("field", fieldName).
		String("message", fixMessage).
		Log()

	return true, fixMessage, objMap
}

// attemptFixes tries various fix strategies in order until one succeeds
func (saf *SpecBasedAutoFixer) attemptFixes(
	fixCtx *AutoFixContext,
	fixKind, fieldName, errorMessage string,
	objMap, fieldMap map[string]any,
) (bool, string) {
	// Define fix strategies with their error message patterns and fix functions
	type fixStrategy struct {
		keywords  []string
		fixFunc   func() any
		msgFunc   func(isArray bool) string
		skipCheck bool // For special cases that don't set fixed=true
	}

	strategies := []fixStrategy{
		// 1. Missing required field: spec default, then internal-metric safe defaults (METRIC_TIER1_AUTOFIX_DESIGN)
		{
			keywords: []string{"required", "is required"},
			fixFunc: func() any {
				if v := extractDefaultValue(fieldMap); v != nil {
					return v
				}
				if v, ok := safeDefaultForInternalMetricKind(fixKind, fieldName); ok {
					return v
				}
				return nil
			},
			msgFunc: func(bool) string {
				return fmt.Sprintf("Added missing required field '%s' with default value from spec", fieldName)
			},
		},
		// 2. Type mismatch (basic coercion)
		{
			keywords: []string{"type", "invalid type", "datatype"},
			fixFunc: func() any {
				return coerceType(fieldName, objMap[fieldName], fieldMap, saf.logger)
			},
			msgFunc: func(bool) string {
				return fmt.Sprintf("Coerced field '%s' to expected type from spec", fieldName)
			},
		},
		// 3. Enum validation
		{
			keywords: []string{"invalid value", "not in enum", "must be one of"},
			fixFunc: func() any {
				return extractEnumValue(fieldMap)
			},
			msgFunc: func(bool) string {
				return fmt.Sprintf("Fixed field '%s' to use valid enum value from spec", fieldName)
			},
		},
		// 4. Pattern violation
		{
			keywords: []string{"pattern", "does not match"},
			fixFunc: func() any {
				return fixPatternViolation(fieldName, objMap[fieldName], fieldMap, saf.logger)
			},
			msgFunc: func(bool) string {
				return fmt.Sprintf("Fixed field '%s' to match required pattern from spec", fieldName)
			},
		},
		// 5. Display length - skip (informational only)
		{
			keywords: []string{"display_length", "exceeds"},
			fixFunc: func() any {
				logging.Fluent(saf.logger).Debug("Skipping display_length auto-fix - display fields created on the fly for table views").
					String("object_id", fixCtx.Obj.ID).
					String("field", fieldName).
					Log()
				return nil
			},
			skipCheck: true,
		},
		// 6. Min length - string or array
		{
			keywords: []string{"too short", "minimum length", "requires at least"},
			fixFunc: func() any {
				// Check if it's an array/list issue
				if strings.Contains(errorMessage, "requires at least") && strings.Contains(errorMessage, "items") {
					return padArrayToMinLength(fieldName, objMap[fieldName], fieldMap, saf.logger)
				}
				return padToMinLength(fieldName, objMap[fieldName], fieldMap, saf.logger)
			},
			msgFunc: func(isArray bool) string {
				if isArray {
					return fmt.Sprintf("Padded array field '%s' to meet minimum length constraint from spec", fieldName)
				}
				return fmt.Sprintf("Padded field '%s' to meet minimum length constraint from spec", fieldName)
			},
		},
		// 7. Max length - string or array
		{
			keywords: []string{"too long", "maximum length", "exceeds maximum"},
			fixFunc: func() any {
				// Check if it's an array/list issue
				if strings.Contains(errorMessage, "exceeds maximum") && strings.Contains(errorMessage, "items") {
					return truncateArrayToMaxLength(fieldName, objMap[fieldName], fieldMap, saf.logger)
				}
				return truncateToMaxLength(fieldName, objMap[fieldName], fieldMap, saf.logger)
			},
			msgFunc: func(isArray bool) string {
				if isArray {
					return fmt.Sprintf("Truncated array field '%s' to maximum length constraint from spec", fieldName)
				}
				return fmt.Sprintf("Truncated field '%s' to maximum length constraint from spec", fieldName)
			},
		},
	}

	// Try each strategy in order
	for _, strategy := range strategies {
		// Check if error message matches this strategy's keywords
		matches := false
		for _, keyword := range strategy.keywords {
			if strings.Contains(errorMessage, keyword) {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}

		// Attempt the fix
		fixedValue := strategy.fixFunc()
		// Fallback for required fields with no spec default: use system account for account/session attribution
		if fixedValue == nil && strings.Contains(errorMessage, "required") {
			if fieldName == "account_id" || fieldName == "created_by" || fieldName == "updated_by" {
				fixedValue = pkgctx.SystemAccountID
			}
		}
		if fixedValue != nil {
			// Dereference *string pointers to store as string (validation expects string, not *string)
			strPtr, strPtrOk := fixedValue.(*string)
			if strPtrOk && strPtr == nil {
				// Pattern fix returned nil (e.g. value already valid) - do not set objMap to nil pointer
				continue
			}
			when.When(func() bool { return strPtrOk && strPtr != nil }).Then(func() {
				objMap[fieldName] = *strPtr
			}).OrElse(func() {
				objMap[fieldName] = fixedValue
			}).Run()
			isArray := strings.Contains(errorMessage, "items")
			return true, strategy.msgFunc(isArray)
		} else if strategy.skipCheck {
			// Special case: strategy ran but didn't fix (e.g., display_length skip)
			continue
		}
	}

	return false, ""
}
