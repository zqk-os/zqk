package system

import (
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// internalMetricKindsWithSafeDefaults are kinds that allow safe defaults for schema_version, metric_type, event_type_counts.
var internalMetricKindsWithSafeDefaults = map[string]bool{
	objects.KindAuditAggregationMetric: true,
}

// metricTier1NoDefaultFields are required fields that must not be auto-filled (computed or regeneration only).
var metricTier1NoDefaultFields = map[string]bool{
	objects.FieldKeyAggregationWindowStart: true,
	objects.FieldKeyAggregationWindowEnd:   true,
	objects.FieldKeyFirstSeen:              true,
}

// documentationDefaultProse are checklist/validation "default" strings that are operator docs, not fill values.
// Materializing them (e.g. title: "required at creation") produces active objects with nonsense titles.
var documentationDefaultProse = map[string]bool{
	"required at creation":        true,
	"none (required at creation)": true,
	"required":                    true,
	"optional":                    true,
	"TBD":                         true,
	"tbd":                         true,
}

func isDocumentationDefaultProse(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(s)
	return documentationDefaultProse[s]
}

// safeDefaultForInternalMetricKind returns a safe default for a missing required field on an internal metric kind.
// Returns (nil, false) for fields that must not be defaulted (e.g. aggregation_window_*, first_seen) or for unknown kind/field.
func safeDefaultForInternalMetricKind(kind, fieldName string) (any, bool) {
	if !internalMetricKindsWithSafeDefaults[kind] {
		return nil, false
	}
	if metricTier1NoDefaultFields[fieldName] {
		return nil, false
	}
	switch fieldName {
	case objects.FieldKeySchemaVersion:
		return objects.DefaultSchemaVersion, true
	case storage.MetricFieldMetricType:
		return storage.StorageMetricTypeSystem, true
	case objects.FieldKeyEventTypeCounts:
		return map[string]any{}, true
	default:
		return nil, false
	}
}

// extractDefaultValue extracts the default value from a field definition
// Checks the checklist.default item first, then falls back to validation.default
func extractDefaultValue(fieldMap map[string]any) any {
	v, _, _ := resolveRequiredFieldDefault("", "", fieldMap)
	return v
}

// resolveRequiredFieldDefault picks a fill value for a missing required field and names the source
// for operator-facing autofix messages (lifecycle vs field checklist vs enum — not a vague "from spec").
func resolveRequiredFieldDefault(kind, fieldName string, fieldMap map[string]any) (value any, source, detail string) {
	// status is owned by the kind lifecycle origin, not object_spec field defaults/enums.
	if fieldName == objects.FieldKeyStatus {
		if origin := lifecycleOriginStatus(kind); origin != "" {
			return origin, "lifecycle_origin",
				fmt.Sprintf("lifecycle origin status %q for kind %s", origin, kind)
		}
	}

	if checklist, ok := fieldMap["checklist"].(map[string]any); ok {
		if defaultValue, exists := checklist["default"]; exists && defaultValue != nil && defaultValue != "" && defaultValue != "null" {
			// checklist.default is often human prose ("required at creation"), not a machine fill value.
			// refuse documentation placeholders as autofill.
			if isDocumentationDefaultProse(defaultValue) {
				// skip — do not materialize prose into instances
			} else {
				return defaultValue, "field_checklist_default",
					fmt.Sprintf("field checklist.default on %s.%s", kind, fieldName)
			}
		}
	}

	if validationMap, ok := getValidationMap(fieldMap); ok {
		if defaultValue, exists := validationMap["default"]; exists && defaultValue != nil && defaultValue != "" {
			if isDocumentationDefaultProse(defaultValue) {
				// skip prose placeholders
			} else {
				return defaultValue, "field_validation_default",
					fmt.Sprintf("field validation.default on %s.%s", kind, fieldName)
			}
		}
		if enum, ok := validationMap["enum"].([]any); ok && len(enum) > 0 {
			return enum[0], "field_enum_first",
				fmt.Sprintf("first allowed enum value on %s.%s", kind, fieldName)
		}
	}

	if v, ok := safeDefaultForInternalMetricKind(kind, fieldName); ok {
		return v, "metric_safe_default",
			fmt.Sprintf("safe default for internal metric kind %s field %s", kind, fieldName)
	}

	return nil, "", ""
}

func lifecycleOriginStatus(kind string) string {
	if kind == "" {
		return ""
	}
	// Resolve against the current project root. GetGlobalLifecycleLoader caches
	// lifecyclesDir on first use, so an earlier isolated TEST_ROOT with no
	// lifecycle files would make every later status fill fall through to enum.
	loader := objects.NewLifecycleLoader("")
	origin, err := loader.GetOriginStatus(kind)
	if err != nil || origin == "" {
		return ""
	}
	return origin
}

func describeWireTypeNormalization(fieldName string, before any, after any) string {
	refHint := ""
	if strings.HasSuffix(fieldName, "_ref") || strings.HasSuffix(fieldName, "_refs") {
		refHint = " (reference ID wire type)"
	}
	return fmt.Sprintf("Normalized field %q from %T to %T%s — required wire shape for persistence/validation, not a business category",
		fieldName, before, after, refHint)
}

// getValidationMap extracts the validation map from a field definition
func getValidationMap(fieldMap map[string]any) (map[string]any, bool) {
	validation, ok := fieldMap["validation"].(map[string]any)
	return validation, ok
}

// extractEnumValue extracts the first valid enum value from field definition
func extractEnumValue(fieldMap map[string]any) any {
	validation, ok := getValidationMap(fieldMap)
	if !ok {
		return nil
	}

	enumValues, ok := validation["enum"].([]any)
	if !ok || len(enumValues) == 0 {
		return nil
	}

	// Return first enum value as default
	return enumValues[0]
}

// applySpecFix applies a spec-based fix by updating the object file.
// If storageProvider is non-nil it is used (same instance as check path); otherwise a new one is created.
func applySpecFix(fixCtx *AutoFixContext, originalObj map[string]any, updatedObj map[string]any, storageProvider storage.ObjectStorageProvider) bool {
	stdctx := pkgctx.NewSystemContext()
	if storageProvider == nil {
		projectRoot := getProjectRootFromContext(fixCtx.Ctx)
		var err error
		var storageFactory *storage.StorageFactory
		storageFactory, err = storage.NewStorageFactory(stdctx, projectRoot)
		if err != nil {
			logging.Fluent(fixCtx.Logger).Warn("Failed to create storage factory for spec fix").
				String("object_id", fixCtx.Obj.ID).
				WithError(err).
				Log()
			return false
		}
		storageProvider = storageFactory.GetStorage()
	}
	if storageProvider == nil {
		logging.Fluent(fixCtx.Logger).Warn("Storage provider is nil for spec fix").
			String("object_id", fixCtx.Obj.ID).
			Log()
		return false
	}
	if updatedObj == nil {
		logging.Fluent(fixCtx.Logger).Warn("Spec fix has nil updated object properties; aborting persistence").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixCtx.Kind).
			Log()
		return false
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	isDemotingToError := false
	if status, ok := updatedObj[objects.FieldKeyStatus].(string); ok && status == "error" {
		if originalStatus, ok := originalObj[objects.FieldKeyStatus].(string); !ok || originalStatus != "error" {
			isDemotingToError = true
		}
	}

	// Allow autofix demote→error when the kind supports it; break_glass is set below with
	// the preserved check reason. Still refuse if "error" is not a valid lifecycle status.
	// TRACK: shockwave Layer-1 demote clarity.
	if isDemotingToError {
		if valid, err := objects.GetGlobalLifecycleLoader().IsValidStatus(fixCtx.Kind, "error"); err != nil || !valid {
			logging.Fluent(fixCtx.Logger).Warn("Refusing autofix status demotion to 'error' — kind has no error status").
				String("object_id", fixCtx.Obj.ID).
				Kind(fixCtx.Kind).
				Log()
			delete(updatedObj, objects.FieldKeyStatus)
			isDemotingToError = false
		}
	}

	// Before persisting, ensure the updated object actually passes instance validation.
	// This prevents creating system objects that remain misaligned from spec (even after "fixes").
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")
	if validator == nil {
		logging.Fluent(fixCtx.Logger).Warn("Instance validation validator not available for spec fix validation").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixCtx.Kind).
			Log()
		return false
	}

	options := validation.DefaultValidationOptions()
	options.ProjectRoot = getProjectRootFromContext(fixCtx.Ctx)
	if status, ok := updatedObj[objects.FieldKeyStatus].(string); ok {
		options.CurrentState = status
	}
	if storageProvider != nil {
		storage.BindValidationLookups(options, stdctx, storageProvider, secCtx)
	}

	result, valErr := validator.Validate(stdctx, updatedObj, fixCtx.Kind, options)
	if valErr != nil {
		logging.Fluent(fixCtx.Logger).Warn("Instance validation failed during spec fix pre-check; aborting persistence").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixCtx.Kind).
			WithError(valErr).
			Log()
		return false
	}
	updatedErrCount := len(result.Errors)
	if updatedErrCount > 0 {
		originalErrCount := -1
		if originalObj != nil {
			origOptions := *options
			if origStatus, ok := originalObj[objects.FieldKeyStatus].(string); ok {
				origOptions.CurrentState = origStatus
			}
			origResult, origErr := validator.Validate(stdctx, originalObj, fixCtx.Kind, &origOptions)
			if origErr == nil && origResult != nil {
				originalErrCount = len(origResult.Errors)
			}
		}

		// Progress persistence: if the object was already invalid but fixes reduced the number of
		// validation errors, persist so we don't reprocess the same resolved issues forever.
		if originalErrCount >= 0 && updatedErrCount < originalErrCount {
			logging.Fluent(fixCtx.Logger).Warn("Spec fix pre-check still has validation errors, but persisting progress (error count reduced)").
				String("object_id", fixCtx.Obj.ID).
				Kind(fixCtx.Kind).
				Int("original_error_count", originalErrCount).
				Int("updated_error_count", updatedErrCount).
				Log()
		} else {
			logging.Fluent(fixCtx.Logger).Warn("Spec fix pre-check found remaining instance validation errors; aborting persistence").
				String("object_id", fixCtx.Obj.ID).
				Kind(fixCtx.Kind).
				Int("error_count", updatedErrCount).
				Int("original_error_count", originalErrCount).
				Log()
			return false
		}
	}

	updateCtx := stdctx
	if origStatus, _ := originalObj[objects.FieldKeyStatus].(string); origStatus != "" {
		if newStatus, _ := updatedObj[objects.FieldKeyStatus].(string); newStatus != "" && newStatus != origStatus {
			reason := "spec auto-fixer lifecycle status repair"
			if isDemotingToError {
				if notes, ok := updatedObj[objects.FieldKeyNotes].(string); ok && notes != "" {
					reason = notes
				}
				reason = "autofix demote to error: " + reason
			}
			updateCtx = pkgctx.WithLifecycleBreakGlass(stdctx, reason)
		}
	}
	updatesToPersist := make(map[string]any, len(updatedObj))
	for k, v := range updatedObj {
		updatesToPersist[k] = v
	}
	if originalObj != nil {
		for k := range originalObj {
			if _, exists := updatedObj[k]; !exists {
				updatesToPersist[k] = storage.FieldUnset
			}
		}
	}
	updateErr := storageProvider.Update(updateCtx, secCtx, fixCtx.Obj.ID, updatesToPersist)
	if updateErr != nil {
		logging.Fluent(fixCtx.Logger).Warn("Failed to apply spec fix via storage provider").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixCtx.Obj.Kind).
			Int("updated_fields_count", len(updatedObj)).
			WithError(updateErr).
			Log()
		return false
	}

	// CRITICAL: Invalidate validation cache for this object after successful update
	// This ensures the next check sees the updated object and new hash
	// The hash registry was updated by storageProvider.Update, but the validation cache
	// still has the old state, which would cause hash mismatches on the next check
	// This must happen synchronously to ensure cache and hash registry stay in sync
	invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)

	logging.Fluent(fixCtx.Logger).Info("Spec-based fix applied successfully").
		String("object_id", fixCtx.Obj.ID).
		Int("updated_fields", len(updatedObj)).
		Log()

	return true
}
