package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestSecurityModel_QueryLogic(t *testing.T) {
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

	// Simulate query result
	queryResult := map[string]any{
		"objects": []any{
			map[string]any{
				objects.FieldKeyID:      "BLI-001",
				objects.FieldKeyKind:    "backlog_item",
				objects.FieldKeyTitle:   "Item 1",
				objects.FieldKeyContext: "Sensitive 1",
			},
			map[string]any{
				objects.FieldKeyID:      "BLI-002",
				objects.FieldKeyKind:    "backlog_item",
				objects.FieldKeyTitle:   "Item 2",
				objects.FieldKeyContext: "Sensitive 2",
			},
		},
		"count": 2,
	}

	// Apply access control (simulating cli_bridge.applyObjectAccessControl)
	objSlice, ok := queryResult["objects"].([]any)
	if !ok {
		t.Fatal("Expected objects array")
	}

	filtered := make([]any, 0)
	for _, obj := range objSlice {
		objMap, ok := obj.(map[string]any)
		if !ok {
			continue
		}

		// Check object-level access
		if !sac.HasObjectAccess(objMap, secCtx) {
			continue
		}

		// Filter fields
		filteredObj := sac.FilterObjectFields(objMap, secCtx, "read")
		filtered = append(filtered, filteredObj)
	}

	// Verify filtered results
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered objects, got %d", len(filtered))
	}

	for _, obj := range filtered {
		objMap, ok := obj.(map[string]any)
		if !ok {
			t.Error("Expected filtered object to be map")
			continue
		}

		// Should not contain restricted fields
		if objMap[objects.FieldKeyContext] != nil {
			t.Error("Expected context to be filtered out")
		}

		// Should contain unrestricted fields
		if objMap[objects.FieldKeyTitle] == nil {
			t.Error("Expected title to be present")
		}
	}
}

// TestSecurityModel_CascadingAccess tests cascading access patterns
func TestSecurityModel_CascadingAccess(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
		},
	}
	mockLoader.specs["requirement.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	secCtx := pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(secCtx)

	// Parent object (user has access)
	parentObj := map[string]any{
		objects.FieldKeyID:   "BLI-001",
		objects.FieldKeyKind: "backlog_item",
	}

	// Referenced object (user doesn't have direct access, but should via cascading)
	referencedObj := map[string]any{
		objects.FieldKeyID:   "REQ-001",
		objects.FieldKeyKind: "requirement",
	}

	// Test cascading access
	hasCascading := sac.HasCascadingAccess(referencedObj, parentObj, secCtx)
	if !hasCascading {
		t.Error("Expected cascading access to be granted")
	}

	// Cascading access should still respect field-level restrictions
	filtered := sac.FilterObjectFields(referencedObj, secCtx, "read")
	if filtered[objects.FieldKeyID] == nil {
		t.Error("Expected id to be included in cascading access")
	}
}

// TestSecurityModel_PermissionCycles tests for cycles in permission dependencies
func TestSecurityModel_PermissionCycles(t *testing.T) {
	t.Parallel()
	// Test that permission requirements don't create cycles
	// Example: role A requires role B, role B requires role A (should be detected)

	// Create a permission dependency graph
	dependencies := map[string][]string{
		"role:admin":     {"role:executive"}, // Admin requires executive
		"role:executive": {"role:admin"},     // Executive requires admin (CYCLE!)
	}

	// Check for cycles
	hasCycle := detectPermissionCycle("role:admin", dependencies, make(map[string]bool))
	if !hasCycle {
		t.Error("Expected to detect permission cycle")
	}

	// Test acyclic dependencies
	acyclicDeps := map[string][]string{
		"role:admin":     {"role:executive"},
		"role:executive": {}, // No dependency
	}

	hasCycle = detectPermissionCycle("role:admin", acyclicDeps, make(map[string]bool))
	if hasCycle {
		t.Error("Expected no cycle in acyclic dependencies")
	}
}
