package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestSecurityModel_QueryFiltering(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			objects.FieldKeyStatus: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			objects.FieldKeyContext: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"access:confidential"},
				},
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	secCtx := pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(secCtx)

	// Query with filter on restricted field
	queryResult := map[string]any{
		"objects": []any{
			map[string]any{
				objects.FieldKeyID:      "ITEM-001",
				objects.FieldKeyKind:    "backlog_item",
				objects.FieldKeyTitle:   "Item 1",
				objects.FieldKeyContext: "Sensitive 1", // Restricted
				objects.FieldKeyStatus:  "exploring",
			},
		},
		"filters": map[string]any{
			objects.FieldKeyContext: "Sensitive", // Filter on restricted field
		},
	}

	// Apply access control
	objSlice, _ := queryResult["objects"].([]any)
	filtered := make([]any, 0)
	for _, obj := range objSlice {
		objMap, ok := obj.(map[string]any)
		if !ok {
			continue
		}

		if !sac.HasObjectAccess(objMap, secCtx) {
			continue
		}

		filteredObj := sac.FilterObjectFields(objMap, secCtx, "read")
		filtered = append(filtered, filteredObj)
	}

	// Verify: Even if query filtered by restricted field, results should not expose it
	for _, obj := range filtered {
		objMap, ok := obj.(map[string]any)
		if !ok {
			continue
		}

		if objMap[objects.FieldKeyContext] != nil {
			t.Error("Query results should not expose restricted fields even if used in filter")
		}
	}
}

// TestSecurityModel_GroupingRestrictedFields tests grouping on restricted fields
func TestSecurityModel_GroupingRestrictedFields(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			objects.FieldKeyContext: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"access:confidential"},
				},
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	secCtx := pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(secCtx)

	groupObjs := []map[string]any{
		{
			objects.FieldKeyID:      "ITEM-001",
			objects.FieldKeyKind:    "backlog_item",
			objects.FieldKeyTitle:   "Item 1",
			objects.FieldKeyContext: "Team Alpha", // Restricted, but used for grouping
		},
		{
			objects.FieldKeyID:      "ITEM-002",
			objects.FieldKeyKind:    "backlog_item",
			objects.FieldKeyTitle:   "Item 2",
			objects.FieldKeyContext: "Team Beta", // Restricted, but used for grouping
		},
	}

	// Group by restricted field - should still filter it from results
	grouped := make(map[string][]map[string]any)
	for _, obj := range groupObjs {
		filtered := sac.FilterObjectFields(obj, secCtx, "read")

		// Even if grouping by context, it should be filtered from results
		context, _ := filtered[objects.FieldKeyContext].(string)
		if context != emptyValue {
			t.Error("Restricted field should be filtered even when used for grouping")
		}

		// Group by a non-restricted field instead
		title, _ := filtered[objects.FieldKeyTitle].(string)
		if title != emptyValue {
			grouped[title] = append(grouped[title], filtered)
		}
	}

	// Verify grouping worked on non-restricted fields
	if len(grouped) == 0 {
		t.Error("Expected grouping to work on non-restricted fields")
	}
}

// TestSecurityModel_SortRestrictedFields tests sorting on restricted fields
func TestSecurityModel_SortRestrictedFields(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			objects.FieldKeyContext: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"access:confidential"},
				},
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	secCtx := pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(secCtx)

	sortObjs := []map[string]any{
		{
			objects.FieldKeyID:      "ITEM-001",
			objects.FieldKeyKind:    "backlog_item",
			objects.FieldKeyTitle:   "Item 1",
			objects.FieldKeyContext: "Zebra", // Restricted
		},
		{
			objects.FieldKeyID:      "ITEM-002",
			objects.FieldKeyKind:    "backlog_item",
			objects.FieldKeyTitle:   "Item 2",
			objects.FieldKeyContext: "Alpha", // Restricted
		},
	}

	// Sort by restricted field - should fail or use fallback
	// In practice, sorting should only work on accessible fields
	for _, obj := range sortObjs {
		filtered := sac.FilterObjectFields(obj, secCtx, "read")

		// Restricted field should not be available for sorting
		if filtered[objects.FieldKeyContext] != nil {
			t.Error("Restricted field should not be available for sorting")
		}

		// Non-restricted field should be available
		if filtered[objects.FieldKeyTitle] == nil {
			t.Error("Non-restricted field should be available for sorting")
		}
	}
}

// TestSecurityModel_ObjectParentFieldCombinations tests acyclic access patterns
// for object.field and object.parent.field combinations
