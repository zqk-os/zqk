package objects

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// SnapableObjectQuery represents the object query configuration for snapable objects
// Note: ListFilter is defined in pkg/storage, but we use map[string]any here
// to avoid import cycles. Callers should convert to storage.ListFilter.
type SnapableObjectQuery struct {
	DefaultQuery map[string]any            // ListFilter structure (filters, sort_by, sort_asc, limit)
	QueryPresets map[string]map[string]any // Preset name -> ListFilter structure
}

// SnapableFieldHandler represents a data handler for a field
type SnapableFieldHandler struct {
	Handler   string         // Handler name (include, exclude, hash, redact, etc.)
	Params    map[string]any // Handler-specific parameters
	Condition map[string]any // Optional condition for when to apply
}

// SnapableFieldConfig represents field-level configuration for snapable fields
type SnapableFieldConfig struct {
	DataHandlers map[string]*SnapableFieldHandler
}

// GetSnapableObjectQuery retrieves the object query configuration for a snapable trait
func GetSnapableObjectQuery(traitRegistry *TraitRegistry) (*SnapableObjectQuery, error) {
	trait, err := traitRegistry.GetTrait("snapable")
	if err != nil {
		return nil, errfmt.Newf("snapable trait not found").Wrap(err)
	}

	if trait.Config == nil {
		return nil, errfmt.Errorf("snapable trait has no configuration")
	}

	objConfig, ok := trait.Config["object_config"].(map[string]any)
	if !ok {
		return nil, errfmt.Errorf("snapable trait missing object_config")
	}

	result := &SnapableObjectQuery{
		QueryPresets: make(map[string]map[string]any),
	}

	// Parse default_query
	if defaultQueryMap, ok := objConfig["default_query"].(map[string]any); ok {
		defaultQuery, err := parseListFilter(defaultQueryMap)
		if err != nil {
			return nil, errfmt.Newf("failed to parse default_query").Wrap(err)
		}
		result.DefaultQuery = defaultQuery
	}

	// Parse query_presets
	if presetsMap, ok := objConfig["query_presets"].(map[string]any); ok {
		for presetName, presetValue := range presetsMap {
			if presetMap, ok := presetValue.(map[string]any); ok {
				presetQuery, err := parseListFilter(presetMap)
				if err != nil {
					return nil, errfmt.Errorf("failed to parse query_preset '%s': %w", presetName, err)
				}
				result.QueryPresets[presetName] = presetQuery
			}
		}
	}

	return result, nil
}

// GetSnapableFieldHandlers retrieves field-level data handlers from a field definition
func GetSnapableFieldHandlers(fieldDef map[string]any) (*SnapableFieldConfig, error) {
	snapableConfig, ok := fieldDef["snapable"].(map[string]any)
	if !ok {
		return nil, nil // Field doesn't have snapable configuration
	}

	fieldConfig, ok := snapableConfig["field_config"].(map[string]any)
	if !ok {
		return nil, nil // No field_config in snapable
	}

	dataHandlersMap, ok := fieldConfig["data_handlers"].(map[string]any)
	if !ok {
		return nil, nil // No data_handlers
	}

	result := &SnapableFieldConfig{
		DataHandlers: make(map[string]*SnapableFieldHandler),
	}

	for handlerName, handlerValue := range dataHandlersMap {
		handlerMap, ok := handlerValue.(map[string]any)
		if !ok {
			continue
		}
		handler := &SnapableFieldHandler{}

		if handlerStr, ok := handlerMap["handler"].(string); ok {
			handler.Handler = handlerStr
		}

		if params, ok := handlerMap["params"].(map[string]any); ok {
			handler.Params = params
		}

		if condition, ok := handlerMap["condition"].(map[string]any); ok {
			handler.Condition = condition
		}

		result.DataHandlers[handlerName] = handler
	}

	return result, nil
}

// GetSnapableObjectQueryFromSpec retrieves object query configuration from a spec file
// This allows specs to override the default query from the trait definition
func GetSnapableObjectQueryFromSpec(spec *Spec) (*SnapableObjectQuery, error) {
	// Check if spec has snapable trait
	hasSnapable := false
	for _, trait := range spec.Traits {
		if trait == "snapable" {
			hasSnapable = true
			break
		}
	}

	if !hasSnapable {
		return nil, nil // Spec doesn't have snapable trait
	}

	// Look for snapable configuration in spec extensions or metadata
	// For now, we'll get it from trait registry
	// Future: Allow specs to override in spec file itself
	traitRegistry := NewTraitRegistry()
	return GetSnapableObjectQuery(traitRegistry)
}

// parseListFilter parses a map into a ListFilter structure (returns map to avoid import cycle)
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func parseListFilter(filterMap map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	// Parse filters
	if filtersMap, ok := filterMap["filters"].(map[string]any); ok {
		result["filters"] = filtersMap
	} else {
		result["filters"] = make(map[string]any)
	}

	// Parse sort_by
	if sortBy, ok := filterMap["sort_by"].(string); ok && sortBy != emptyValue && sortBy != "null" {
		result["sort_by"] = sortBy
	}

	// Parse sort_asc
	if sortAsc, ok := filterMap["sort_asc"].(bool); ok {
		result["sort_asc"] = sortAsc
	} else {
		result["sort_asc"] = true // Default
	}

	// Parse limit
	if limit, ok := filterMap["limit"].(int); ok {
		result["limit"] = limit
	} else {
		result["limit"] = 0 // Default: no limit
	}

	return result, nil
}

// ApplyFieldHandler applies a data handler to a field value
func ApplyFieldHandler(value any, handler *SnapableFieldHandler, fieldDef map[string]any) (any, error) {
	if handler == nil {
		return value, nil // No handler, return as-is
	}

	// Check condition if present
	if handler.Condition != nil {
		if !evaluateCondition(handler.Condition, fieldDef) {
			return value, nil // Condition not met, return as-is
		}
	}

	// Apply handler based on type
	switch handler.Handler {
	case "include":
		return value, nil
	case "exclude":
		return nil, nil // Signal to exclude field
	case "null":
		return nil, nil
	case "hash":
		return applyHashHandler(value, handler.Params)
	case "redact":
		return applyRedactHandler(value, handler.Params)
	case "reference":
		return applyReferenceHandler(value, handler.Params)
	case "timestamp":
		return applyTimestampHandler(value, handler.Params)
	case "transform":
		return applyTransformHandler(value, handler.Params)
	default:
		return value, nil // Unknown handler, return as-is
	}
}

// evaluateCondition evaluates a handler condition against field definition
func evaluateCondition(condition map[string]any, fieldDef map[string]any) bool {
	// Simple condition evaluation
	// Supports field_tags, security, etc.
	for key, expectedValue := range condition {
		switch key {
		case "field_tags":
			// Check if field has tag
			if tags, ok := fieldDef["field_tags"].([]any); ok {
				if expectedMap, ok := expectedValue.(map[string]any); ok {
					if hasOp, ok := expectedMap["$has"].(string); ok {
						for _, tag := range tags {
							if tagStr, ok := tag.(string); ok && tagStr == hasOp {
								return true
							}
						}
					}
				}
			}
			// Also check "tags" for backward compatibility
			if tags, ok := fieldDef[FieldKeyTags].([]any); ok {
				if expectedMap, ok := expectedValue.(map[string]any); ok {
					if hasOp, ok := expectedMap["$has"].(string); ok {
						for _, tag := range tags {
							if tagStr, ok := tag.(string); ok && tagStr == hasOp {
								return true
							}
						}
					}
				}
			}
		case "security":
			// Check security field
			if security, ok := fieldDef["security"].(string); ok {
				if expectedStr, ok := expectedValue.(string); ok {
					return security == expectedStr
				}
			}
		}
	}
	return false
}

// applyHashHandler applies hash transformation
func applyHashHandler(_ any, params map[string]any) (any, error) {
	algorithm := "sha256"
	if alg, ok := params[FieldKeyAlgorithm].(string); ok {
		algorithm = alg
	}

	// For now, return placeholder - full implementation would hash the value
	// TODO: Implement actual hashing
	return fmt.Sprintf("[%s_hash]", algorithm), nil
}

// applyRedactHandler applies redaction transformation
func applyRedactHandler(value any, params map[string]any) (any, error) {
	replacement := "***"
	if repl, ok := params["replacement"].(string); ok {
		replacement = repl
	}

	if _, ok := value.(string); ok {
		// Simple redaction - replace with replacement string
		// TODO: Implement pattern-based redaction
		return replacement, nil
	}

	return value, nil
}

// applyReferenceHandler applies reference transformation (ID remapping)
func applyReferenceHandler(value any, params map[string]any) (any, error) {
	// For now, return as-is - full implementation would remap IDs
	// TODO: Implement ID remapping based on params
	return value, nil
}

// applyTimestampHandler applies timestamp normalization
func applyTimestampHandler(value any, params map[string]any) (any, error) {
	// For now, return as-is - full implementation would normalize format
	// TODO: Implement timestamp normalization
	return value, nil
}

// applyTransformHandler applies custom transformation
func applyTransformHandler(value any, params map[string]any) (any, error) {
	// For now, return as-is - full implementation would call custom function
	// TODO: Implement custom transformation function lookup and execution
	return value, nil
}
