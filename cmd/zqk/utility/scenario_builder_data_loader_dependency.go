package utility

import (
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
)

// objectRefs represents the references from an object
type objectRefs struct {
	objID      string
	kind       string
	obj        map[string]any
	references []string // List of referenced object IDs
}

// buildDependencyGraphAndSort builds a dependency graph from objects and returns them grouped by dependency layers
// Each layer contains objects that can be created in parallel (they have no dependencies on each other)
// Layers are returned in order (layer 0 first, then layer 1, etc.) - each layer must complete before the next starts
// This enables breadth-first parallel processing: objects in the same layer are created concurrently
func (sb *ScenarioBuilder) buildDependencyGraphAndSort(objList []map[string]any) ([][]map[string]any, error) {
	// Step 1: Build object ID map for quick lookup and extract references
	objectMap := make(map[string]map[string]any) // objID -> object
	objectRefsList := make([]objectRefs, 0, len(objList))
	yamlParser := parser.NewYAMLParser()

	for _, obj := range objList {
		// Get object ID (may be missing, will be generated later)
		objID, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)

		// Extract all references from this object
		refFields := yamlParser.ExtractReferenceFields(obj)
		references := make([]string, 0)

		// Collect all referenced IDs
		for _, refValue := range refFields {
			switch v := refValue.(type) {
			case string:
				if v != emptyValue {
					references = append(references, v)
				}
			case []any:
				for _, item := range v {
					if str, ok := item.(string); ok && str != emptyValue {
						references = append(references, str)
					}
				}
			case []string:
				references = append(references, v...)
			}
		}

		// Store object in map if it has an ID
		if objID != emptyValue {
			objectMap[objID] = obj
		}

		// Also index by account username for account:username format
		if kind == objects.KindAccount {
			if username, ok := obj[objects.FieldKeyUsername].(string); ok && username != emptyValue {
				accountID := "ACC-" + strings.ToUpper(username)
				objectMap[accountID] = obj
				if objID == emptyValue {
					obj[objects.FieldKeyID] = accountID
					objID = accountID
				}
			}
		}

		objectRefsList = append(objectRefsList, objectRefs{
			objID:      objID,
			kind:       kind,
			obj:        obj,
			references: references,
		})
	}

	// Step 2: Build dependency graph (which objects depend on which)
	// Graph: objID -> []objIDs that this object depends on
	dependencies := make(map[string][]string) // objID -> list of objIDs it depends on
	allObjIDs := make(map[string]bool)        // All object IDs in the data file

	for _, objRef := range objectRefsList {
		if objRef.objID != emptyValue {
			allObjIDs[objRef.objID] = true
			deps := make([]string, 0)
			for _, refID := range objRef.references {
				// Only add dependency if referenced object is in our data file
				if _, exists := objectMap[refID]; exists {
					deps = append(deps, refID)
				}
				// Also check account username format
				if strings.HasPrefix(refID, "account:") {
					username := strings.TrimPrefix(refID, "account:")
					if accountObj, exists := objectMap[refID]; exists {
						if accountID, ok := accountObj[objects.FieldKeyID].(string); ok && accountID != refID {
							// Account ID might be different, check both
							if _, exists2 := objectMap[accountID]; exists2 {
								deps = append(deps, accountID)
							}
						}
					} else if accountObj, exists := objectMap[username]; exists {
						// Check if username maps to an account object
						if k, ok := accountObj[objects.FieldKeyKind].(string); ok && k == objects.KindAccount {
							if accountID, ok := accountObj[objects.FieldKeyID].(string); ok {
								deps = append(deps, accountID)
							}
						}
					}
				}
			}
			if len(deps) > 0 {
				dependencies[objRef.objID] = deps
			}
		}
	}

	// Step 3: Group objects by dependency layers using breadth-first approach
	// Layer 0: Objects with no dependencies (can be created in parallel)
	// Layer 1: Objects that only depend on Layer 0 (can be created in parallel after Layer 0)
	// Layer 2: Objects that depend on Layer 0 or Layer 1 (can be created in parallel after Layer 1)
	// etc.
	layers := make([][]map[string]any, 0)
	inDegree := make(map[string]int) // objID -> number of dependencies this object has

	// Build reverse dependency graph: dep -> []objIDs that depend on it
	reverseDeps := make(map[string][]string) // dep -> list of objIDs that depend on it
	for objID, deps := range dependencies {
		for _, dep := range deps {
			reverseDeps[dep] = append(reverseDeps[dep], objID)
		}
	}

	// Initialize in-degree: count how many dependencies each object has
	// Sort keys for deterministic iteration
	allObjIDKeys := make([]string, 0, len(allObjIDs))
	for objID := range allObjIDs {
		allObjIDKeys = append(allObjIDKeys, objID)
	}
	sort.Strings(allObjIDKeys)

	for _, objID := range allObjIDKeys {
		if deps, exists := dependencies[objID]; exists {
			inDegree[objID] = len(deps)
		} else {
			inDegree[objID] = 0
		}
	}

	// Process in layers (breadth-first)
	processed := make(map[string]bool)
	for {
		// Find all nodes with no remaining dependencies for current layer
		currentLayer := make([]string, 0)
		for _, objID := range allObjIDKeys {
			if !processed[objID] && inDegree[objID] == 0 {
				currentLayer = append(currentLayer, objID)
			}
		}

		// If no nodes found, we're done (or have circular dependencies)
		if len(currentLayer) == 0 {
			break
		}

		// Sort layer for deterministic ordering
		sort.Strings(currentLayer)

		// Add objects in this layer to the result
		layerObjects := make([]map[string]any, 0, len(currentLayer))
		for _, objID := range currentLayer {
			processed[objID] = true
			if obj, exists := objectMap[objID]; exists {
				layerObjects = append(layerObjects, obj)
			}
		}

		// Add layer if it has objects
		if len(layerObjects) > 0 {
			layers = append(layers, layerObjects)
		}

		// Decrease in-degree of objects that depend on objects in this layer
		for _, objID := range currentLayer {
			dependentIDs := reverseDeps[objID]
			sort.Strings(dependentIDs)
			for _, dependentID := range dependentIDs {
				if _, exists := allObjIDs[dependentID]; exists && !processed[dependentID] {
					inDegree[dependentID]--
				}
			}
		}
	}

	// Add objects without IDs to a final layer (they have no dependencies and can be created in any order)
	// Also add objects with IDs that weren't processed (circular dependencies or errors)
	orphanLayer := make([]map[string]any, 0)
	for _, objRef := range objectRefsList {
		if objRef.objID == emptyValue {
			orphanLayer = append(orphanLayer, objRef.obj)
		} else if !processed[objRef.objID] {
			// Object has ID but wasn't processed (circular dependency or error in graph)
			// Add it anyway - will be validated during creation
			orphanLayer = append(orphanLayer, objRef.obj)
		}
	}
	if len(orphanLayer) > 0 {
		layers = append(layers, orphanLayer)
	}

	return layers, nil
}

// usesContentAddressableStorage checks if a kind uses content-addressable storage
// This is a simplified check - in reality, we'd need to query the storage layer
func (sb *ScenarioBuilder) usesContentAddressableStorage(kind string) bool {
	// CAS kinds: account, keystore_entry
	casKinds := map[string]bool{
		objects.KindAccount:       true,
		objects.KindKeystoreEntry: true,
	}
	return casKinds[kind]
}
