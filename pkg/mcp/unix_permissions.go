package mcp

import (
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// UnixPermission represents a Unix-like permission (rwx)
// r = read, w = write, x = execute (use in operations)
type UnixPermission uint8

const (
	// Permission bits
	ReadBit  UnixPermission = 4 // 100
	WriteBit UnixPermission = 2 // 010
	ExecBit  UnixPermission = 1 // 001

	// Common permission combinations
	NoAccess      UnixPermission = 0 // 000 ---
	ReadOnly      UnixPermission = 4 // 100 r--
	WriteOnly     UnixPermission = 2 // 010 -w-
	ExecOnly      UnixPermission = 1 // 001 --x
	ReadWrite     UnixPermission = 6 // 110 rw-
	ReadExec      UnixPermission = 5 // 101 r-x
	WriteExec     UnixPermission = 3 // 011 -wx
	ReadWriteExec UnixPermission = 7 // 111 rwx
)

// ParseUnixPermission parses a Unix-like permission string (e.g., "rwx", "r-x", "r--")
func ParseUnixPermission(s string) (UnixPermission, error) {
	if len(s) != 3 {
		return 0, errfmt.Errorf("permission string must be 3 characters, got %d", len(s))
	}

	var perm UnixPermission
	if s[0] == 'r' {
		perm |= ReadBit
	}
	if s[1] == 'w' {
		perm |= WriteBit
	}
	if s[2] == 'x' {
		perm |= ExecBit
	}

	return perm, nil
}

// String returns the Unix-like permission string (e.g., "rwx", "r-x")
func (p UnixPermission) String() string {
	var s strings.Builder
	if p&ReadBit != 0 {
		s.WriteRune('r')
	} else {
		s.WriteRune('-')
	}
	if p&WriteBit != 0 {
		s.WriteRune('w')
	} else {
		s.WriteRune('-')
	}
	if p&ExecBit != 0 {
		s.WriteRune('x')
	} else {
		s.WriteRune('-')
	}
	return s.String()
}

// Octal returns the octal representation (0-7)
func (p UnixPermission) Octal() uint8 {
	return uint8(p)
}

// HasRead returns true if read permission is set
func (p UnixPermission) HasRead() bool {
	return p&ReadBit != 0
}

// HasWrite returns true if write permission is set
func (p UnixPermission) HasWrite() bool {
	return p&WriteBit != 0
}

// HasExec returns true if execute permission is set
func (p UnixPermission) HasExec() bool {
	return p&ExecBit != 0
}

// CheckPermission checks if the permission includes the required operation
// operation: "read", "write", or "execute"
func (p UnixPermission) CheckPermission(operation string) bool {
	switch operation {
	case "read":
		return p.HasRead()
	case "write":
		return p.HasWrite()
	case "execute", "exec":
		return p.HasExec()
	default:
		return false
	}
}

// FieldPermission represents field-level permissions with Unix-like model
type FieldPermission struct {
	// Unix-like permission (rwx format)
	Permission UnixPermission // Default: rwx if user has kind-level read
	// Additional requirements beyond kind-level permissions
	Requires []string // e.g., ["access:confidential", "role:admin"]
}

// ParseFieldPermission parses field permission from spec definition
func ParseFieldPermission(fieldDef map[string]any) *FieldPermission {
	fp := &FieldPermission{
		Permission: ReadWriteExec, // Default: full access if kind-level read exists
		Requires:   []string{},
	}

	// Check for explicit permission definition
	if permStr, ok := fieldDef[objects.FieldKeyPermissions].(string); ok {
		if perm, err := ParseUnixPermission(permStr); err == nil {
			fp.Permission = perm
		}
	}

	// Check for additional requirements
	if accessMeta, ok := fieldDef["access"].(map[string]any); ok {
		// Extract requires from access metadata
		if requires, ok := accessMeta["requires"].([]any); ok {
			fp.Requires = make([]string, len(requires))
			for i, req := range requires {
				if reqStr, ok := req.(string); ok {
					fp.Requires[i] = reqStr
				}
			}
		}
	}

	// Check checklist.security for sensitive fields.
	// "non-sensitive" must not match the substring "sensitive".
	if checklist, ok := fieldDef["checklist"].(map[string]any); ok {
		if security, ok := checklist["security"].(string); ok {
			if checklistSecurityRequiresConfidential(security) && len(fp.Requires) == 0 {
				fp.Requires = []string{"access:confidential"}
			}
		}
	}

	return fp
}

func checklistSecurityRequiresConfidential(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "non-") || strings.HasPrefix(s, "not ") || strings.HasPrefix(s, "not-") {
		return false
	}
	return strings.Contains(s, "sensitive") || strings.Contains(s, "confidential")
}

// HasFieldAccess checks if user has access to a field based on Unix-like permissions
// kindLevelRead: true if user has read permission on the kind
// secCtx: security context with roles and permissions (can be *pkgctx.SecurityContext or map[string]any)
func (fp *FieldPermission) HasFieldAccess(kindLevelRead bool, operation string, secCtx any) bool {
	// If no kind-level read, deny access (unless field has no requirements)
	if !kindLevelRead {
		// Check if field has no requirements and allows read
		if len(fp.Requires) == 0 && fp.Permission.HasRead() && operation == "read" {
			// Public field - allow read even without kind-level permission
			return true
		}
		return false
	}

	// Check additional requirements
	if len(fp.Requires) > 0 {
		var roles []string
		var permissions []string

		// Handle both *pkgctx.SecurityContext and map[string]any
		switch ctx := secCtx.(type) {
		case *pkgctx.SecurityContext:
			roles = ctx.Roles
			permissions = ctx.Permissions
		case map[string]any:
			if r, ok := ctx[objects.FieldKeyRoles].([]string); ok {
				roles = r
			}
			if p, ok := ctx[objects.FieldKeyPermissions].([]string); ok {
				permissions = p
			}
		default:
			// If we can't extract roles/permissions, deny access
			return false
		}

		// Check each requirement
		for _, req := range fp.Requires {
			// Check role requirement (e.g., "role:admin")
			if strings.HasPrefix(req, "role:") {
				requiredRole := strings.TrimPrefix(req, "role:")
				hasRole := false
				for _, role := range roles {
					if role == requiredRole {
						hasRole = true
						break
					}
				}
				if !hasRole {
					return false
				}
				continue
			}

			// Check access requirement (e.g., "access:confidential")
			if strings.HasPrefix(req, "access:") {
				requiredAccess := req
				hasAccess := false
				for _, perm := range permissions {
					if perm == requiredAccess || perm == "access:*" {
						hasAccess = true
						break
					}
				}
				if !hasAccess {
					return false
				}
				continue
			}

			// Check permission requirement (e.g., "permission:read:confidential")
			if strings.HasPrefix(req, "permission:") {
				requiredPerm := strings.TrimPrefix(req, "permission:")
				hasPerm := false
				for _, perm := range permissions {
					if perm == requiredPerm || perm == "*" {
						hasPerm = true
						break
					}
				}
				if !hasPerm {
					return false
				}
				continue
			}
		}
	}

	// Check Unix-like permission for the operation
	return fp.Permission.CheckPermission(operation)
}
