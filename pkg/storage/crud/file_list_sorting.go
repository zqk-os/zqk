package crud

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Uses typed fields from parsed objects for faster access during sorting
// Original objects are preserved on disk, so we only need to sort the parsed slice
func SortParsedObjectsSlice(parsedObjects []*objects.ParsedObject, sortBy string, sortAsc bool) {
	sort.Slice(parsedObjects, func(i, j int) bool {
		// Get field value from parsed object (faster typed access)
		valI, existsI := parsedObjects[i].GetField(sortBy)
		valJ, existsJ := parsedObjects[j].GetField(sortBy)

		// Handle nil/missing values (nil sorts last)
		if !existsI && !existsJ {
			return false
		}
		if !existsI || valI == nil {
			return false
		}
		if !existsJ || valJ == nil {
			return true
		}

		// Compare values based on type
		cmp := CompareValues(valI, valJ)
		if cmp == 0 {
			return false
		}
		less := cmp < 0
		if !sortAsc {
			less = !less
		}
		return less
	})
}

// sortObjects sorts objects by the specified field
// Legacy version that works with raw maps only
//
// sortObjects sorts objects by the specified field
//
//nolint:unused // Legacy function - reserved for backward compatibility
func SortObjects(objList []map[string]any, sortBy string, sortAsc bool) {
	sort.Slice(objList, func(i, j int) bool {
		valI := objList[i][sortBy]
		valJ := objList[j][sortBy]

		// Handle nil values (nil sorts last)
		if valI == nil && valJ == nil {
			return false
		}
		if valI == nil {
			return false
		}
		if valJ == nil {
			return true
		}

		// Compare values based on type
		cmp := CompareValues(valI, valJ)
		if cmp == 0 {
			return false
		}
		less := cmp < 0
		if !sortAsc {
			less = !less
		}
		return less
	})
}

// groupObjects groups objects by a field or comma-separated fields
func GroupObjects(objList []map[string]any, groupBy string, maxGroups int) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)

	groupFields := strings.Split(groupBy, ",")
	for i := range groupFields {
		groupFields[i] = strings.TrimSpace(groupFields[i])
	}

	for _, obj := range objList {
		var groupValueParts []string
		for _, field := range groupFields {
			if objects.IsKernelObjectRefField(field) {
				groupValueParts = append(groupValueParts, objects.KernelObjectRefGroupKey(obj, field))
				continue
			}
			if val, ok := obj[field]; ok && val != nil && fmt.Sprintf("%v", val) != "" {
				groupValueParts = append(groupValueParts, fmt.Sprintf("%v", val))
			} else {
				groupValueParts = append(groupValueParts, "") // Empty/null values grouped together
			}
		}
		groupValue := strings.Join(groupValueParts, ", ")

		groups[groupValue] = append(groups[groupValue], obj)
	}

	// Limit number of groups if specified
	if maxGroups > 0 && len(groups) > maxGroups {
		// Keep only the first maxGroups groups (sorted by key)
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		limited := make(map[string][]map[string]any)
		for i := 0; i < maxGroups && i < len(keys); i++ {
			limited[keys[i]] = groups[keys[i]]
		}
		return limited
	}

	return groups
}

// flattenGroups converts grouped objects back to a flat list
func FlattenGroups(groups map[string][]map[string]any) []map[string]any {
	var result []map[string]any
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		result = append(result, groups[key]...)
	}
	return result
}
