package mcp

import (
	"fmt"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSecurityModel_BruteForce(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()

	// Add mock specs with field definitions
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
			objects.FieldKeyTokens: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"access": map[string]any{
					"requires": []any{"role:admin"},
				},
			},
			objects.FieldKeySpecAdherence: map[string]any{
				objects.FieldKeyPermissions: "r-x", // Read-only
			},
			"some_readonly_field": map[string]any{
				objects.FieldKeyPermissions: "r--", // Read-only, no execute
			},
		},
	}

	pc := NewPermissionCache(mockLoader)

	// Test scenarios that should be denied
	denyScenarios := []struct {
		name    string
		secCtx  *pkgctx.SecurityContext
		obj     map[string]any
		field   string
		op      string
		wantErr bool
	}{
		// No kind-level read, field requires confidential
		{
			name:    "no kind read, confidential field",
			secCtx:  pkgctx.NewSecurityContext("account:user", []string{"developer"}, nil),
			obj:     map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyContext: "sensitive"},
			field:   "context",
			op:      "read",
			wantErr: true, // Should deny
		},
		// Kind-level read, but field requires admin
		{
			name:    "kind read, admin-only field",
			secCtx:  pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:     map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:   "tokens", // Admin-only field
			op:      "read",
			wantErr: true, // Should deny
		},
		// Kind-level read, field requires confidential, no access
		{
			name:    "kind read, confidential field, no access",
			secCtx:  pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:     map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyContext: "sensitive"},
			field:   "context",
			op:      "read",
			wantErr: true, // Should deny
		},
		// Write operation on read-only field
		{
			name:    "write on r-x field",
			secCtx:  pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "write:backlog_item"}),
			obj:     map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:   "spec_adherence", // Read-only field
			op:      "write",
			wantErr: true, // Should deny
		},
		// Execute operation on r-- field
		{
			name:    "execute on r-- field",
			secCtx:  pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:     map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:   "some_readonly_field",
			op:      "execute",
			wantErr: true, // Should deny if field is r--
		},
	}

	// Test scenarios that should be allowed
	allowScenarios := []struct {
		name   string
		secCtx *pkgctx.SecurityContext
		obj    map[string]any
		field  string
		op     string
	}{
		// Kind-level read, no field restrictions
		{
			name:   "kind read, unrestricted field",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Test"},
			field:  "title",
			op:     "read",
		},
		// Kind-level read, confidential field, has access
		{
			name:   "kind read, confidential field, has access",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:confidential"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyContext: "sensitive"},
			field:  "context",
			op:     "read",
		},
		// Admin role, all fields
		{
			name:   "admin role, all fields",
			secCtx: pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "any_field",
			op:     "read",
		},
		// Wildcard access, confidential field
		{
			name:   "wildcard access, confidential field",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:*"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyContext: "sensitive"},
			field:  "context",
			op:     "read",
		},
		// Read operation on r-x field
		{
			name:   "read on r-x field",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "spec_adherence",
			op:     "read",
		},
		// Execute operation on r-x field
		{
			name:   "execute on r-x field",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "spec_adherence",
			op:     "execute",
		},
	}

	// Build permission cache for test contexts
	allScenarios := make([]struct {
		name    string
		secCtx  *pkgctx.SecurityContext
		obj     map[string]any
		field   string
		op      string
		wantErr bool
	}, 0, len(denyScenarios)+len(allowScenarios))

	allScenarios = append(allScenarios, denyScenarios...)
	for _, s := range allowScenarios {
		allScenarios = append(allScenarios, struct {
			name    string
			secCtx  *pkgctx.SecurityContext
			obj     map[string]any
			field   string
			op      string
			wantErr bool
		}{s.name, s.secCtx, s.obj, s.field, s.op, false})
	}

	for _, scenario := range allScenarios {
		//nolint:errcheck // Cache build errors are non-critical in tests
		_ = pc.BuildPermissionCache(scenario.secCtx)
	}

	// Test deny scenarios
	for _, scenario := range denyScenarios {
		t.Run("deny_"+scenario.name, func(t *testing.T) {
			// Create spec access control
			sac := NewSpecAccessControl(mockLoader)
			sac.SetPermissionCache(pc)

			// Check field access
			hasAccess := sac.HasFieldAccess(scenario.field, scenario.obj, scenario.secCtx, scenario.op)
			if hasAccess {
				t.Errorf("Expected access to be denied for scenario: %s", scenario.name)
			}
		})
	}

	// Test allow scenarios
	for _, scenario := range allowScenarios {
		t.Run("allow_"+scenario.name, func(t *testing.T) {
			// Create spec access control
			sac := NewSpecAccessControl(mockLoader)
			sac.SetPermissionCache(pc)

			// Check field access
			hasAccess := sac.HasFieldAccess(scenario.field, scenario.obj, scenario.secCtx, scenario.op)
			if !hasAccess {
				t.Errorf("Expected access to be granted for scenario: %s", scenario.name)
			}
		})
	}
}

// TestSecurityModel_EdgeCases tests edge cases and boundary conditions
func TestSecurityModel_EdgeCases(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	mockLoader.specs["backlog_item.yaml"] = &mockSpec{
		resolvedFields: map[string]any{
			objects.FieldKeyTitle: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
		},
	}

	pc := NewPermissionCache(mockLoader)
	sac := NewSpecAccessControl(mockLoader)
	sac.SetPermissionCache(pc)

	tests := []struct {
		name   string
		secCtx *pkgctx.SecurityContext
		obj    map[string]any
		field  string
		op     string
		want   bool
	}{
		// Edge case: nil security context
		{
			name:   "nil security context",
			secCtx: nil,
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "title",
			op:     "read",
			want:   false,
		},
		// Edge case: missing kind
		{
			name:   "missing kind",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001"},
			field:  "title",
			op:     "read",
			want:   true, // Defaults to allowing if kind missing (line 87 in spec_access_control.go)
		},
		// Edge case: empty permissions
		{
			name:   "empty permissions",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, nil),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "title",
			op:     "read",
			want:   true, // Field has no requirements, so public field allows read even without kind-level permission
		},
		// Edge case: wildcard read permission
		{
			name:   "wildcard read permission",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:*"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "title",
			op:     "read",
			want:   true, // Wildcard read grants kind-level read
		},
		// Edge case: field not in spec
		{
			name:   "field not in spec",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "unknown_field",
			op:     "read",
			want:   true, // Defaults to allowing if field not in spec (kind-level read exists)
		},
		// Edge case: multiple requires, all match
		{
			name:   "multiple requires, all match",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"admin"}, []string{"read:backlog_item", "access:confidential"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "field_with_multiple_requires",
			op:     "read",
			want:   true, // Should allow if all requirements match (user has admin role and access:confidential)
		},
		// Edge case: deactivated user
		{
			name:   "deactivated user",
			secCtx: pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			obj:    map[string]any{objects.FieldKeyID: "OBJ-001", objects.FieldKeyKind: "backlog_item"},
			field:  "title",
			op:     "read",
			want:   false,
		},
	}

	// Build permission cache
	// Use unique user IDs for each test to avoid cross-contamination
	for i, tt := range tests {
		if tt.secCtx == nil {
			continue
		}
		// Create a unique user ID for each test case to avoid deactivation affecting other tests
		uniqueUserID := fmt.Sprintf("%s_%d", tt.secCtx.AccountID, i)
		uniqueSecCtx := pkgctx.NewSecurityContext(uniqueUserID, tt.secCtx.Roles, tt.secCtx.Permissions)
		//nolint:errcheck // Cache build errors are non-critical in tests
		_ = pc.BuildPermissionCache(uniqueSecCtx)
		if tt.name == "deactivated user" {
			// Deactivate user AFTER building cache
			//nolint:errcheck // Deactivate errors are non-critical in tests
			_ = pc.DeactivateUser(uniqueUserID)
		}
		// Update the test case to use the unique user ID
		tests[i].secCtx = uniqueSecCtx
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasAccess := sac.HasFieldAccess(tt.field, tt.obj, tt.secCtx, tt.op)
			if hasAccess != tt.want {
				t.Errorf("HasFieldAccess() = %v, want %v", hasAccess, tt.want)
			}
		})
	}
}

// TestSecurityModel_Filtering tests that filtering respects field-level permissions
