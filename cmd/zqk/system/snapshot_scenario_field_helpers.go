package system

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// checkObjectHasSnapableTrait checks if an object spec has the snapable trait
func checkObjectHasSnapableTrait(spec *objects.Spec) bool {
	if spec == nil {
		return false
	}
	for _, trait := range spec.Traits {
		if trait == "snapable" {
			return true
		}
	}
	return false
}

// isSnapshotMetadataField checks if a field is a metadata field that should be copied as-is in snapshots
func isSnapshotMetadataField(key string) bool {
	return key == "id" || key == "kind" || key == "created_at" || key == "updated_at" ||
		key == "created_by" || key == "updated_by" || key == "schema_version" ||
		key == "status" || key == objects.FieldKeyOriginProject || key == objects.FieldKeyOriginSystem
}

// checkFieldHasSnapableTrait checks if a field has the snapable trait
func checkFieldHasSnapableTrait(fieldDef map[string]any) bool {
	fieldTraits := objects.ExtractFieldTraits(fieldDef)
	for _, trait := range fieldTraits {
		if trait == "snapable" {
			return true
		}
	}
	return false
}

// applyFieldHandlersToValue applies data handlers to a field value
func applyFieldHandlersToValue(value any, fieldConfig *objects.SnapableFieldConfig, fieldDef map[string]any, fieldName string, logger logging.Logger) (any, bool) {
	if fieldConfig == nil || len(fieldConfig.DataHandlers) == 0 {
		return value, false // No handlers, include as-is
	}

	// Apply handlers in order (first matching handler wins)
	processedValue := value
	excluded := false

	for _, handler := range fieldConfig.DataHandlers {
		result, err := objects.ApplyFieldHandler(processedValue, handler, fieldDef)
		if err != nil {
			logging.Fluent(logger).Warn("Handler failed").
				String("field", fieldName).
				String("handler", handler.Handler).
				WithError(err).
				Log()
			continue
		}

		// Check if handler excluded the field
		if result == nil && handler.Handler == "exclude" {
			excluded = true
			break
		}

		processedValue = result
	}

	return processedValue, excluded
}

// processField processes a single field based on spec and handlers
func processField(key string, value any, spec *objects.Spec, logger logging.Logger) (any, bool, bool) {
	// Get field definition from ResolvedFields
	fieldDefAny, ok := spec.ResolvedFields[key]
	if !ok {
		// Field not in spec, include as-is
		return value, true, false
	}

	fieldDef, ok := fieldDefAny.(map[string]any)
	if !ok {
		// Field definition is not a map, include as-is
		return value, true, false
	}

	// Check if field has snapable trait
	if !checkFieldHasSnapableTrait(fieldDef) {
		// Field not snapable, exclude from snapshot
		return nil, false, false
	}

	// Get field handlers
	fieldConfig, err := objects.GetSnapableFieldHandlers(fieldDef)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to get field handlers").
			String("field", key).
			WithError(err).
			Log()
		return value, true, false // Include as-is on error
	}

	// Apply handlers
	processedValue, excluded := applyFieldHandlersToValue(value, fieldConfig, fieldDef, key, logger)
	return processedValue, !excluded, false
}
