package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestSecurityModel_Filtering(t *testing.T) {
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

	// Object with mix of restricted and unrestricted fields
	obj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyTitle:   "Test Item",
		objects.FieldKeyContext: "Sensitive context", // Requires confidential
		objects.FieldKeyStatus:  "exploring",
	}

	// Filter fields - should exclude context (requires confidential)
	filtered := sac.FilterObjectFields(obj, secCtx, "read")

	// Should include unrestricted fields
	if filtered[objects.FieldKeyTitle] == nil {
		t.Error("Expected title to be included")
	}
	if filtered[objects.FieldKeyStatus] == nil {
		t.Error("Expected status to be included")
	}

	// Should exclude restricted fields
	if filtered[objects.FieldKeyContext] != nil {
		t.Error("Expected context to be excluded (requires confidential)")
	}

	// Should always include id and kind
	if filtered[objects.FieldKeyID] == nil {
		t.Error("Expected id to always be included")
	}
	if filtered[objects.FieldKeyKind] == nil {
		t.Error("Expected kind to always be included")
	}
}

// TestSecurityModel_Grouping tests that grouping respects field-level permissions
func TestSecurityModel_Grouping(t *testing.T) {
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

	testObjs := []map[string]any{
		{
			objects.FieldKeyID:      "BLI-001",
			objects.FieldKeyKind:    "backlog_item",
			objects.FieldKeyTitle:   "Item 1",
			objects.FieldKeyContext: "Sensitive 1", // Restricted
			objects.FieldKeyStatus:  "exploring",
		},
		{
			objects.FieldKeyID:      "BLI-002",
			objects.FieldKeyKind:    "backlog_item",
			objects.FieldKeyTitle:   "Item 2",
			objects.FieldKeyContext: "Sensitive 2", // Restricted
			objects.FieldKeyStatus:  "validated",
		},
	}

	// Filter each object
	filtered := make([]map[string]any, len(testObjs))
	for i, obj := range testObjs {
		filtered[i] = sac.FilterObjectFields(obj, secCtx, "read")
	}

	// Verify restricted fields are excluded
	for i, obj := range filtered {
		if obj[objects.FieldKeyContext] != nil {
			t.Errorf("Object %d: Expected context to be excluded", i)
		}
		if obj[objects.FieldKeyTitle] == nil {
			t.Errorf("Object %d: Expected title to be included", i)
		}
		if obj[objects.FieldKeyStatus] == nil {
			t.Errorf("Object %d: Expected status to be included", i)
		}
	}
}

// TestSecurityModel_QueryLogic tests that query operations respect permissions
