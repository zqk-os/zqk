package mcp

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestQuerySecurity_FilterRestrictedFields tests that query filters on restricted fields are rejected
func TestQuerySecurity_FilterRestrictedFields(t *testing.T) {
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

	// Attempt to filter by restricted field
	filterField := "context" // Requires confidential

	// Check if user can access this field for filtering
	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyContext: "Sensitive",
	}

	// User should not be able to filter by restricted field
	canAccessForFilter := sac.HasFieldAccess(filterField, obj, secCtx, "read")
	if canAccessForFilter {
		t.Error("User should not be able to filter by restricted field")
	}

	// Even if filter is applied, results should not expose the field
	queryResult := map[string]any{
		"objects": []any{obj},
		"filters": map[string]any{filterField: "Sensitive"},
	}

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

	// Verify restricted field is not exposed
	for _, obj := range filtered {
		objMap, ok := obj.(map[string]any)
		if !ok {
			continue
		}

		if objMap[objects.FieldKeyContext] != nil {
			t.Error("Restricted field should not be exposed in query results")
		}
	}
}

// TestQuerySecurity_GroupByRestrictedFields tests that grouping by restricted fields is rejected
func TestQuerySecurity_GroupByRestrictedFields(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
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

	// Attempt to group by restricted field
	groupByField := "context" // Requires confidential

	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyContext: "Team Alpha",
	}

	// User should not be able to group by restricted field
	canAccessForGroup := sac.HasFieldAccess(groupByField, obj, secCtx, "read")
	if canAccessForGroup {
		t.Error("User should not be able to group by restricted field")
	}

	// Grouping should fail or use fallback
	filtered := sac.FilterObjectFields(obj, secCtx, "read")
	if filtered[objects.FieldKeyContext] != nil {
		t.Error("Restricted field should not be available for grouping")
	}
}

// TestQuerySecurity_SortByRestrictedFields tests that sorting by restricted fields is rejected
func TestQuerySecurity_SortByRestrictedFields(t *testing.T) {
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

	// Attempt to sort by restricted field
	sortByField := "context" // Requires confidential

	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyContext: "Zebra",
	}

	// User should not be able to sort by restricted field
	canAccessForSort := sac.HasFieldAccess(sortByField, obj, secCtx, "read")
	if canAccessForSort {
		t.Error("User should not be able to sort by restricted field")
	}

	// Sorting should fail or use fallback
	filtered := sac.FilterObjectFields(obj, secCtx, "read")
	if filtered[objects.FieldKeyContext] != nil {
		t.Error("Restricted field should not be available for sorting")
	}
}

// TestQuerySecurity_ValidateFilterFields validates that filter fields are accessible
func TestQuerySecurity_ValidateFilterFields(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			objects.FieldKeyContext: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"access:confidential"},
				},
			},
			objects.FieldKeyCreatedBy: map[string]any{
				objects.FieldKeyPermissions: "r-x",
				"access": map[string]any{
					"requires": []any{"access:audit"},
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

	// Test filter validation
	filters := map[string]any{
		objects.FieldKeyStatus:    "exploring",     // Allowed
		objects.FieldKeyTitle:     "Test",          // Allowed
		objects.FieldKeyContext:   "Sensitive",     // Restricted - should be rejected
		objects.FieldKeyCreatedBy: "account:admin", // Restricted - should be rejected
	}

	obj := map[string]any{
		objects.FieldKeyID:        "BLI-001",
		objects.FieldKeyKind:      "backlog_item",
		objects.FieldKeyStatus:    "exploring",
		objects.FieldKeyTitle:     "Test",
		objects.FieldKeyContext:   "Sensitive",
		objects.FieldKeyCreatedBy: "account:admin",
	}

	// Validate each filter field
	validFilters := make(map[string]any)
	for field, value := range filters {
		if sac.HasFieldAccess(field, obj, secCtx, "read") {
			validFilters[field] = value
		}
	}

	// Only accessible fields should be in validFilters
	if validFilters[objects.FieldKeyContext] != nil {
		t.Error("Restricted field should not be in valid filters")
	}
	if validFilters[objects.FieldKeyCreatedBy] != nil {
		t.Error("Restricted field should not be in valid filters")
	}
	if validFilters[objects.FieldKeyStatus] == nil {
		t.Error("Accessible field should be in valid filters")
	}
	if validFilters[objects.FieldKeyTitle] == nil {
		t.Error("Accessible field should be in valid filters")
	}
}

// TestQuerySecurity_ValidateGroupByField validates that group-by field is accessible
func TestQuerySecurity_ValidateGroupByField(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
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

	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyStatus:  "exploring",
		objects.FieldKeyContext: "Sensitive",
	}

	// Test accessible group-by field
	canGroupByStatus := sac.HasFieldAccess("status", obj, secCtx, "read")
	if !canGroupByStatus {
		t.Error("Should be able to group by accessible field")
	}

	// Test restricted group-by field
	canGroupByContext := sac.HasFieldAccess("context", obj, secCtx, "read")
	if canGroupByContext {
		t.Error("Should not be able to group by restricted field")
	}
}

// TestQuerySecurity_ValidateSortByField validates that sort-by field is accessible
func TestQuerySecurity_ValidateSortByField(t *testing.T) {
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

	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyTitle:   "Test",
		objects.FieldKeyContext: "Sensitive",
	}

	// Test accessible sort-by field
	canSortByTitle := sac.HasFieldAccess("title", obj, secCtx, "read")
	if !canSortByTitle {
		t.Error("Should be able to sort by accessible field")
	}

	// Test restricted sort-by field
	canSortByContext := sac.HasFieldAccess("context", obj, secCtx, "read")
	if canSortByContext {
		t.Error("Should not be able to sort by restricted field")
	}
}

// TestQuerySecurity_ComplexQueryOperations tests complex query operations with mixed permissions
func TestQuerySecurity_ComplexQueryOperations(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
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

	// Complex query with filters, sorting, and grouping
	query := map[string]any{
		"filters": map[string]any{
			objects.FieldKeyStatus:  "exploring",                         // Allowed
			objects.FieldKeyTitle:   map[string]any{"$contains": "Test"}, // Allowed
			objects.FieldKeyContext: "Sensitive",                         // Restricted - should be rejected
		},
		"sort_by":  "title",  // Allowed
		"group_by": "status", // Allowed
	}

	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyStatus:  "exploring",
		objects.FieldKeyTitle:   "Test Item",
		objects.FieldKeyContext: "Sensitive",
	}

	// Validate query parameters
	validFilters := make(map[string]any)
	if filters, ok := query["filters"].(map[string]any); ok {
		for field, value := range filters {
			if sac.HasFieldAccess(field, obj, secCtx, "read") {
				validFilters[field] = value
			}
		}
	}

	// Restricted filter should be removed
	if validFilters[objects.FieldKeyContext] != nil {
		t.Error("Restricted filter should be removed from query")
	}

	// Allowed filters should remain
	if validFilters[objects.FieldKeyStatus] == nil {
		t.Error("Allowed filter should remain in query")
	}
	if validFilters[objects.FieldKeyTitle] == nil {
		t.Error("Allowed filter should remain in query")
	}

	// Validate sort_by
	if sortBy, ok := query["sort_by"].(string); ok {
		canSort := sac.HasFieldAccess(sortBy, obj, secCtx, "read")
		if !canSort {
			t.Error("Sort field should be accessible")
		}
	}

	// Validate group_by
	if groupBy, ok := query["group_by"].(string); ok {
		canGroup := sac.HasFieldAccess(groupBy, obj, secCtx, "read")
		if !canGroup {
			t.Error("Group field should be accessible")
		}
	}
}
