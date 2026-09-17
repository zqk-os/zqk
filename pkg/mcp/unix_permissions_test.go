package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestParseUnixPermission(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected UnixPermission
		wantErr  bool
	}{
		{
			name:     "full access",
			input:    "rwx",
			expected: ReadWriteExec,
			wantErr:  false,
		},
		{
			name:     "read only",
			input:    "r--",
			expected: ReadOnly,
			wantErr:  false,
		},
		{
			name:     "read and execute",
			input:    "r-x",
			expected: ReadExec,
			wantErr:  false,
		},
		{
			name:     "write only",
			input:    "-w-",
			expected: WriteOnly,
			wantErr:  false,
		},
		{
			name:     "no access",
			input:    "---",
			expected: NoAccess,
			wantErr:  false,
		},
		{
			name:     "invalid length",
			input:    "rw",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseUnixPermission(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUnixPermission() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ParseUnixPermission() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnixPermission_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		perm     UnixPermission
		expected string
	}{
		{
			name:     "full access",
			perm:     ReadWriteExec,
			expected: "rwx",
		},
		{
			name:     "read only",
			perm:     ReadOnly,
			expected: "r--",
		},
		{
			name:     "read and execute",
			perm:     ReadExec,
			expected: "r-x",
		},
		{
			name:     "no access",
			perm:     NoAccess,
			expected: "---",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.perm.String()
			if result != tt.expected {
				t.Errorf("UnixPermission.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnixPermission_CheckPermission(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		perm      UnixPermission
		operation string
		expected  bool
	}{
		{
			name:      "read on rwx",
			perm:      ReadWriteExec,
			operation: "read",
			expected:  true,
		},
		{
			name:      "write on rwx",
			perm:      ReadWriteExec,
			operation: "write",
			expected:  true,
		},
		{
			name:      "execute on rwx",
			perm:      ReadWriteExec,
			operation: "execute",
			expected:  true,
		},
		{
			name:      "write on r-x",
			perm:      ReadExec,
			operation: "write",
			expected:  false,
		},
		{
			name:      "read on r-x",
			perm:      ReadExec,
			operation: "read",
			expected:  true,
		},
		{
			name:      "read on r--",
			perm:      ReadOnly,
			operation: "read",
			expected:  true,
		},
		{
			name:      "write on r--",
			perm:      ReadOnly,
			operation: "write",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.perm.CheckPermission(tt.operation)
			if result != tt.expected {
				t.Errorf("UnixPermission.CheckPermission(%q) = %v, want %v", tt.operation, result, tt.expected)
			}
		})
	}
}

func TestFieldPermission_HasFieldAccess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		perm          UnixPermission
		requires      []string
		kindLevelRead bool
		operation     string
		secCtx        *pkgctx.SecurityContext
		expected      bool
	}{
		{
			name:          "kind-level read, no requirements, rwx",
			perm:          ReadWriteExec,
			requires:      []string{},
			kindLevelRead: true,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			expected:      true,
		},
		{
			name:          "no kind-level read, no requirements, rwx",
			perm:          ReadWriteExec,
			requires:      []string{},
			kindLevelRead: false,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{}),
			expected:      true, // Public field allows read
		},
		{
			name:          "kind-level read, requires confidential, has access",
			perm:          ReadWriteExec,
			requires:      []string{"access:confidential"},
			kindLevelRead: true,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item", "access:confidential"}),
			expected:      true,
		},
		{
			name:          "kind-level read, requires confidential, no access",
			perm:          ReadWriteExec,
			requires:      []string{"access:confidential"},
			kindLevelRead: true,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			expected:      false,
		},
		{
			name:          "kind-level read, requires admin role, has role",
			perm:          ReadWriteExec,
			requires:      []string{"role:admin"},
			kindLevelRead: true,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:backlog_item"}),
			expected:      true,
		},
		{
			name:          "kind-level read, requires admin role, no role",
			perm:          ReadWriteExec,
			requires:      []string{"role:admin"},
			kindLevelRead: true,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			expected:      false,
		},
		{
			name:          "r-x permission, write operation",
			perm:          ReadExec,
			requires:      []string{},
			kindLevelRead: true,
			operation:     "write",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			expected:      false,
		},
		{
			name:          "r-x permission, read operation",
			perm:          ReadExec,
			requires:      []string{},
			kindLevelRead: true,
			operation:     "read",
			secCtx:        pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:backlog_item"}),
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := &FieldPermission{
				Permission: tt.perm,
				Requires:   tt.requires,
			}

			result := fp.HasFieldAccess(tt.kindLevelRead, tt.operation, tt.secCtx)
			if result != tt.expected {
				t.Errorf("FieldPermission.HasFieldAccess() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseFieldPermission(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		fieldDef         map[string]any
		expectedPerm     UnixPermission
		expectedRequires []string
	}{
		{
			name: "explicit permissions rwx",
			fieldDef: map[string]any{
				objects.FieldKeyPermissions: "rwx",
			},
			expectedPerm:     ReadWriteExec,
			expectedRequires: []string{},
		},
		{
			name: "explicit permissions r-x",
			fieldDef: map[string]any{
				objects.FieldKeyPermissions: "r-x",
			},
			expectedPerm:     ReadExec,
			expectedRequires: []string{},
		},
		{
			name: "sensitive field from checklist",
			fieldDef: map[string]any{
				"checklist": map[string]any{
					"security": "sensitive - contains confidential information",
				},
			},
			expectedPerm:     ReadWriteExec,
			expectedRequires: []string{"access:confidential"},
		},
		{
			name: "non-sensitive is not confidential",
			fieldDef: map[string]any{
				"checklist": map[string]any{
					"security": "non-sensitive",
				},
			},
			expectedPerm:     ReadWriteExec,
			expectedRequires: []string{},
		},
		{
			name: "priority_plan active_order stays writable without confidential",
			fieldDef: map[string]any{
				objects.FieldKeyPermissions: "rwx",
				"checklist": map[string]any{
					"security": "non-sensitive.",
				},
			},
			expectedPerm:     ReadWriteExec,
			expectedRequires: []string{},
		},
		{
			name: "requires from access metadata",
			fieldDef: map[string]any{
				"access": map[string]any{
					"requires": []any{"access:confidential", "role:admin"},
				},
			},
			expectedPerm:     ReadWriteExec,
			expectedRequires: []string{"access:confidential", "role:admin"},
		},
		{
			name: "combined permissions and requires",
			fieldDef: map[string]any{
				objects.FieldKeyPermissions: "r-x",
				"access": map[string]any{
					"requires": []any{"access:confidential"},
				},
			},
			expectedPerm:     ReadExec,
			expectedRequires: []string{"access:confidential"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := ParseFieldPermission(tt.fieldDef)

			if fp.Permission != tt.expectedPerm {
				t.Errorf("ParseFieldPermission().Permission = %v, want %v", fp.Permission, tt.expectedPerm)
			}

			if len(fp.Requires) != len(tt.expectedRequires) {
				t.Errorf("ParseFieldPermission().Requires length = %d, want %d", len(fp.Requires), len(tt.expectedRequires))
				return
			}

			for i, req := range tt.expectedRequires {
				if i >= len(fp.Requires) || fp.Requires[i] != req {
					t.Errorf("ParseFieldPermission().Requires[%d] = %v, want %v", i, fp.Requires, req)
					break
				}
			}
		})
	}
}
