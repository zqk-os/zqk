package mcp

import (
	"fmt"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestSecurityModel_ObjectParentFieldCombinations(t *testing.T) {
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
	mockLoader.specs["requirement.yaml"] = &mockSpec{
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

	secCtx := pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "read:requirement"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(secCtx)

	// Parent object
	parentObj := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyTitle:   "Parent Item",
		objects.FieldKeyContext: "Parent context", // Restricted
	}

	// Child object (referenced by parent)
	childObj := map[string]any{
		objects.FieldKeyID:      "REQ-001",
		objects.FieldKeyKind:    "requirement",
		objects.FieldKeyTitle:   "Child Requirement",
		objects.FieldKeyContext: "Child context", // Restricted
	}

	// Test: Access parent field
	hasParentFieldAccess := sac.HasFieldAccess("title", parentObj, secCtx, "read")
	if !hasParentFieldAccess {
		t.Error("Expected access to parent.title")
	}

	// Test: Access parent restricted field (should be denied)
	hasParentRestrictedAccess := sac.HasFieldAccess("context", parentObj, secCtx, "read")
	if hasParentRestrictedAccess {
		t.Error("Expected access to parent.context to be denied")
	}

	// Test: Cascading access to child
	hasCascading := sac.HasCascadingAccess(childObj, parentObj, secCtx)
	if !hasCascading {
		t.Error("Expected cascading access to child")
	}

	// Test: Access child field via cascading
	hasChildFieldAccess := sac.HasFieldAccess("title", childObj, secCtx, "read")
	if !hasChildFieldAccess {
		t.Error("Expected access to child.title via cascading")
	}

	// Test: Access child restricted field (should still be denied even with cascading)
	hasChildRestrictedAccess := sac.HasFieldAccess("context", childObj, secCtx, "read")
	if hasChildRestrictedAccess {
		t.Error("Expected access to child.context to be denied even with cascading")
	}

	// Test: Filter parent with cascading child
	parentFiltered := sac.FilterObjectFields(parentObj, secCtx, "read")
	if parentFiltered[objects.FieldKeyContext] != nil {
		t.Error("Expected parent.context to be filtered")
	}

	childFiltered := sac.FilterObjectFields(childObj, secCtx, "read")
	if childFiltered[objects.FieldKeyContext] != nil {
		t.Error("Expected child.context to be filtered even with cascading access")
	}
}

// TestSecurityModel_PermissionTraversalAcyclic tests that permission traversals are acyclic
func TestSecurityModel_PermissionTraversalAcyclic(t *testing.T) {
	t.Parallel()
	// Test various permission dependency patterns
	tests := []struct {
		name        string
		deps        map[string][]string
		expectCycle bool
	}{
		{
			name: "simple chain",
			deps: map[string][]string{
				"role:admin":     {"role:executive"},
				"role:executive": {},
			},
			expectCycle: false,
		},
		{
			name: "direct cycle",
			deps: map[string][]string{
				"role:admin":     {"role:executive"},
				"role:executive": {"role:admin"},
			},
			expectCycle: true,
		},
		{
			name: "indirect cycle",
			deps: map[string][]string{
				"role:admin":     {"role:executive"},
				"role:executive": {"role:owner"},
				"role:owner":     {"role:admin"},
			},
			expectCycle: true,
		},
		{
			name: "no dependencies",
			deps: map[string][]string{
				"role:admin": {},
			},
			expectCycle: false,
		},
		{
			name: "multiple paths, no cycle",
			deps: map[string][]string{
				"role:admin":     {"role:executive", "role:owner"},
				"role:executive": {},
				"role:owner":     {},
			},
			expectCycle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check each permission for cycles
			for perm := range tt.deps {
				hasCycle := detectPermissionCycle(perm, tt.deps, make(map[string]bool))
				if hasCycle != tt.expectCycle {
					t.Errorf("Permission %s: expected cycle=%v, got %v", perm, tt.expectCycle, hasCycle)
				}
			}
		})
	}
}

// TestSecurityModel_ComplexAccessPatterns tests complex access patterns
func TestSecurityModel_ComplexAccessPatterns(t *testing.T) {
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
			objects.FieldKeyCreatedBy: map[string]any{
				objects.FieldKeyPermissions: "r-x",
				"access": map[string]any{
					"requires": []any{"access:audit"},
				},
			},
			objects.FieldKeyChangeLog: map[string]any{
				objects.FieldKeyPermissions: "r-x",
				"access": map[string]any{
					"requires": []any{"access:audit"},
				},
			},
			objects.FieldKeySpecAdherence: map[string]any{
				objects.FieldKeyPermissions: "r-x", // Read-only
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	// User with limited permissions
	limitedSecCtx := pkgctx.NewSecurityContext("account:limited", []string{"viewer"}, []string{"read:backlog_item"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(limitedSecCtx)

	// User with full permissions
	fullSecCtx := pkgctx.NewSecurityContext("account:full", []string{"developer"}, []string{"read:backlog_item", "access:confidential", "access:audit"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(fullSecCtx)

	// Object with multiple restricted fields
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Test Item",
		objects.FieldKeyContext:       "Confidential context",                         // Requires confidential
		objects.FieldKeyCreatedBy:     "account:admin",                                // Requires audit
		objects.FieldKeyChangeLog:     []any{"entry1", "entry2"},                      // Requires audit
		objects.FieldKeySpecAdherence: map[string]any{objects.FieldKeyStatus: "pass"}, // Read-only
	}

	// Test limited user
	limitedFiltered := sac.FilterObjectFields(obj, limitedSecCtx, "read")
	if limitedFiltered[objects.FieldKeyContext] != nil {
		t.Error("Limited user should not see context")
	}
	if limitedFiltered[objects.FieldKeyCreatedBy] != nil {
		t.Error("Limited user should not see created_by")
	}
	if limitedFiltered[objects.FieldKeyChangeLog] != nil {
		t.Error("Limited user should not see change_log")
	}
	if limitedFiltered[objects.FieldKeyTitle] == nil {
		t.Error("Limited user should see title")
	}

	// Test full user
	fullFiltered := sac.FilterObjectFields(obj, fullSecCtx, "read")
	if fullFiltered[objects.FieldKeyContext] == nil {
		t.Error("Full user should see context")
	}
	if fullFiltered[objects.FieldKeyCreatedBy] == nil {
		t.Error("Full user should see created_by")
	}
	if fullFiltered[objects.FieldKeyChangeLog] == nil {
		t.Error("Full user should see change_log")
	}
	if fullFiltered[objects.FieldKeyTitle] == nil {
		t.Error("Full user should see title")
	}

	// Test write operations
	canWriteContext := sac.HasFieldAccess("context", obj, limitedSecCtx, "write")
	if canWriteContext {
		t.Error("Limited user should not be able to write context")
	}

	canWriteTitle := sac.HasFieldAccess("title", obj, limitedSecCtx, "write")
	if !canWriteTitle {
		t.Error("Limited user should be able to write title (if they have write permission)")
	}
}

// TestSecurityModel_PrivilegeEscalation attempts privilege escalation attacks
func TestSecurityModel_PrivilegeEscalation(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["account.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTokens: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"role:admin"},
				},
			},
		},
	}
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyContext: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"access:confidential"},
				},
			},
			objects.FieldKeySpecAdherence: map[string]any{
				objects.FieldKeyPermissions: "r-x", // Read-only
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	// Low-privilege user
	lowPrivSecCtx := pkgctx.NewSecurityContext("account:lowpriv", []string{"viewer"}, []string{"read:backlog_item"})
	//nolint:errcheck // Cache build errors are non-critical in tests
	_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(lowPrivSecCtx)

	// Attempt to access admin-only field
	obj := map[string]any{
		objects.FieldKeyID:     "ACC-001",
		objects.FieldKeyKind:   "account",
		objects.FieldKeyTokens: []any{"token1", "token2"}, // Admin-only
	}

	hasAccess := sac.HasFieldAccess("tokens", obj, lowPrivSecCtx, "read")
	if hasAccess {
		t.Error("Low-privilege user should not access admin-only field")
	}

	// Attempt to access confidential field without permission
	obj2 := map[string]any{
		objects.FieldKeyID:      "BLI-001",
		objects.FieldKeyKind:    "backlog_item",
		objects.FieldKeyContext: "Confidential", // Requires confidential
	}

	hasAccess2 := sac.HasFieldAccess("context", obj2, lowPrivSecCtx, "read")
	if hasAccess2 {
		t.Error("Low-privilege user should not access confidential field")
	}

	// Attempt to write read-only field
	obj3 := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySpecAdherence: map[string]any{objects.FieldKeyStatus: "pass"}, // Read-only
	}

	canWrite := sac.HasFieldAccess("spec_adherence", obj3, lowPrivSecCtx, "write")
	if canWrite {
		t.Error("User should not be able to write read-only field")
	}
}

// TestSecurityModel_FieldPermissionCombinations tests all permission combinations
func TestSecurityModel_FieldPermissionCombinations(t *testing.T) {
	t.Parallel()
	operations := []string{"read", "write", "execute"}
	permissions := []UnixPermission{
		NoAccess, ReadOnly, WriteOnly, ExecOnly,
		ReadWrite, ReadExec, WriteExec, ReadWriteExec,
	}

	for _, perm := range permissions {
		for _, op := range operations {
			t.Run(fmt.Sprintf("perm_%s_op_%s", perm.String(), op), func(t *testing.T) {
				hasAccess := perm.CheckPermission(op)

				// Verify expected behavior
				expected := false
				switch op {
				case "read":
					expected = perm.HasRead()
				case "write":
					expected = perm.HasWrite()
				case "execute":
					expected = perm.HasExec()
				}

				if hasAccess != expected {
					t.Errorf("Permission %s, operation %s: got %v, want %v", perm.String(), op, hasAccess, expected)
				}
			})
		}
	}
}

// TestSecurityModel_RequirementCombinations tests various requirement combinations
func TestSecurityModel_RequirementCombinations(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	tests := []struct {
		name      string
		secCtx    *pkgctx.SecurityContext
		requires  []string
		wantAllow bool
	}{
		{
			name:      "single requirement, matches",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:confidential"}),
			requires:  []string{"access:confidential"},
			wantAllow: true,
		},
		{
			name:      "single requirement, no match",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			requires:  []string{"access:confidential"},
			wantAllow: false,
		},
		{
			name:      "multiple requirements, all match",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"admin"}, []string{"read:backlog_item", "access:confidential"}),
			requires:  []string{"access:confidential", "role:admin"},
			wantAllow: true,
		},
		{
			name:      "multiple requirements, one missing",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:confidential"}),
			requires:  []string{"access:confidential", "role:admin"},
			wantAllow: false,
		},
		{
			name:      "wildcard access, single requirement",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:*"}),
			requires:  []string{"access:confidential"},
			wantAllow: true,
		},
		{
			name:      "wildcard access, multiple requirements",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:*"}),
			requires:  []string{"access:confidential", "access:audit"},
			wantAllow: true,
		},
		{
			name:      "no requirements",
			secCtx:    pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			requires:  nil,
			wantAllow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:errcheck // Cache build errors are non-critical in tests
			_ = pc.BuildPermissionCache //nolint:errcheck // Cache build errors are non-critical in tests(tt.secCtx)

			fp := &FieldPermission{
				Permission: ReadWriteExec,
				Requires:   tt.requires,
			}

			hasAccess := fp.HasFieldAccess(true, "read", tt.secCtx) // kind-level read = true
			if hasAccess != tt.wantAllow {
				t.Errorf("HasFieldAccess() = %v, want %v", hasAccess, tt.wantAllow)
			}
		})
	}
}
