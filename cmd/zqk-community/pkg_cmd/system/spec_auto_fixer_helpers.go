package system

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
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
	// Try checklist.default first
	if checklist, ok := fieldMap["checklist"].(map[string]any); ok {
		if defaultValue, exists := checklist["default"]; exists && defaultValue != nil {
			return defaultValue
		}
	}

	// Try validation.default as fallback
	if validation, ok := getValidationMap(fieldMap); ok {
		if defaultValue, exists := validation["default"]; exists && defaultValue != nil {
			return defaultValue
		}
		// Try enum default (first enum value if available)
		if enum, ok := validation["enum"].([]any); ok && len(enum) > 0 {
			return enum[0] // Use first enum value as default
		}
	}

	return nil
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

	secCtx := &pkgctx.SecurityContext{
		AccountID:   fixCtx.Ctx.Profile,
		Roles:       []string{"admin"},
		Permissions: []string{"write:*"},
	}
	if secCtx.AccountID == emptyValue {
		secCtx.AccountID = "system"
	}

	isDemotingToError := false
	if status, ok := updatedObj[objects.FieldKeyStatus].(string); ok && status == "error" {
		if originalStatus, ok := originalObj[objects.FieldKeyStatus].(string); !ok || originalStatus != "error" {
			isDemotingToError = true
		}
	}

	if isDemotingToError {
		logging.Fluent(fixCtx.Logger).Info("Persisting status demotion to 'error' without validation pre-check").
			String("object_id", fixCtx.Obj.ID).
			Log()
	} else {
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
		if status, ok := updatedObj[objects.FieldKeyStatus].(string); ok {
			options.CurrentState = status
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
				origResult, origErr := validator.Validate(stdctx, originalObj, fixCtx.Kind, options)
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
	}

	updateCtx := stdctx
	if isDemotingToError {
		updateCtx = pkgctx.WithForceLifecycleOverride(stdctx)
	}
	updateErr := storageProvider.Update(updateCtx, secCtx, fixCtx.Obj.ID, updatedObj)
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
