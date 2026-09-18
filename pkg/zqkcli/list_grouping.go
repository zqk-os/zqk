package internal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// groupResults groups objects by a specified field
// For visibility grouping, resolves visibility from object spec if not present on object
func groupResults(result *storage.QueryResult, groupBy string) *storage.QueryResult {
	if result == nil || len(result.Objects) == 0 {
		return result
	}

	groups := make(map[string][]map[string]any)

	// Initialize spec loader for visibility resolution (only if needed)
	var specLoader *objects.SpecLoader
	if groupBy == objects.FieldKeyVisibility {
		specLoader = objects.NewSpecLoader("")
	}

	for _, obj := range result.Objects {
		// Ensure object has an ID (for consistency)
		ensureObjectID(obj)

		// Get the value of the field to group by
		objKind, _ := obj[objects.FieldKeyKind].(string)

		groupFields := strings.Split(groupBy, ",")
		for i := range groupFields {
			groupFields[i] = strings.TrimSpace(groupFields[i])
		}

		var groupKeyParts []string
		for _, field := range groupFields {
			var part string
			// Special case: when grouping by visibility, lifecycle definitions and object_specs
			// are ALWAYS internal system objects, regardless of what visibility they define
			// for the objects they describe. They are system objects, not user objects.
			if field == objects.FieldKeyVisibility && (objKind == internalKindLifecycle || objKind == internalKindObjectSpec) {
				part = internalSourceInternal
				obj[objects.FieldKeyVisibility] = internalSourceInternal // Override any existing visibility
			} else if val, ok := obj[field]; ok && val != nil && val != emptyValue {
				if str, ok := val.(string); ok {
					part = str
				} else {
					part = fmt.Sprintf("%v", val)
				}
			} else {
				// Field not present - try to resolve from spec if grouping by visibility
				if field == objects.FieldKeyVisibility {
					// Special case: lifecycle definitions and object_specs are always internal system objects
					// They are system objects regardless of what visibility they define for the objects they describe
					if objKind == internalKindLifecycle || objKind == internalKindObjectSpec {
						part = internalSourceInternal
						obj[objects.FieldKeyVisibility] = internalSourceInternal
					} else if specLoader != nil && objKind != emptyValue {
						// For other objects, try to load spec and get visibility
						specFile := objKind + ".yaml"
						if spec, err := specLoader.LoadSpecWithInheritance(specFile); err == nil && spec != nil {
							if spec.Visibility != emptyValue {
								part = spec.Visibility
								// Also set it on the object for future reference
								obj[objects.FieldKeyVisibility] = spec.Visibility
							} else {
								// If spec has no visibility, check if it extends something
								// Visibility might be inherited from parent spec
								if spec.Extends != emptyValue {
									parentFile := spec.Extends + ".yaml"
									if parentSpec, err := specLoader.LoadSpecWithInheritance(parentFile); err == nil && parentSpec != nil {
										if parentSpec.Visibility != emptyValue {
											part = parentSpec.Visibility
											obj[objects.FieldKeyVisibility] = parentSpec.Visibility
										} else {
											part = "(none)"
										}
									} else {
										part = "(none)"
									}
								} else {
									part = "(none)"
								}
							}
						} else {
							part = "(none)"
						}
					} else {
						part = "(none)"
					}
				} else {
					part = "(none)"
				}
			}
			groupKeyParts = append(groupKeyParts, part)
		}
		groupKey := strings.Join(groupKeyParts, ", ")

		if groups[groupKey] == nil {
			groups[groupKey] = make([]map[string]any, 0)
		}
		groups[groupKey] = append(groups[groupKey], obj)
	}

	// Create new result with groups
	groupedResult := &storage.QueryResult{
		Objects: result.Objects, // Keep original objects
		Groups:  groups,
		Meta:    result.Meta,
	}

	if groupedResult.Meta == nil {
		groupedResult.Meta = make(map[string]any)
	}
	groupedResult.Meta["total_groups"] = len(groups)

	return groupedResult
}

// ensureObjectID ensures an object has an 'id' field
// If missing, tries to generate one from other fields or uses a fallback
func ensureObjectID(obj map[string]any) {
	// Check if ID already exists
	if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
		return // ID already present
	}

	// Try to get ID from various possible fields
	// Some objects might use different field names
	if id, ok := obj["_id"].(string); ok && id != emptyValue {
		obj[objects.FieldKeyID] = id
		return
	}

	// For lifecycle definitions, use the filename-based ID
	if kind, _ := obj[objects.FieldKeyKind].(string); kind == internalKindLifecycle {
		if filePath, ok := obj[objects.FieldKeyFilePath].(string); ok {
			// Extract ID from filename
			baseName := filepath.Base(filePath)
			id := strings.TrimSuffix(baseName, filepath.Ext(baseName))
			if id != emptyValue {
				obj[objects.FieldKeyID] = id
				return
			}
		}
		// Fallback: use object_type + "_lifecycle"
		if objectType, ok := obj[objects.FieldKeyObjectType].(string); ok && objectType != emptyValue {
			obj[objects.FieldKeyID] = objectType + "_lifecycle"
			return
		}
	}

	// For object specs, use ontology as ID
	if kind, _ := obj[objects.FieldKeyKind].(string); kind == internalKindObjectSpec {
		if ontology, ok := obj[objects.FieldKeyOntology].(string); ok && ontology != emptyValue {
			obj[objects.FieldKeyID] = ontology
			return
		}
		// Fallback: use filename
		if filePath, ok := obj[objects.FieldKeyFilePath].(string); ok {
			baseName := filepath.Base(filePath)
			id := strings.TrimSuffix(baseName, filepath.Ext(baseName))
			if id != emptyValue {
				obj[objects.FieldKeyID] = id
				return
			}
		}
	}

	// Last resort: use kind + a hash or index if available
	// For now, we'll leave it as missing rather than generating a fake ID
	// This ensures we don't mask the problem
}
