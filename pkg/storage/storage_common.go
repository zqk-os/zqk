// Package storage provides a unified storage abstraction layer for zqk,
// supporting both file-based and graph-based storage backends through a
// common interface.
//
// This file contains shared utilities used by both FileObjectStorage and
// GraphObjectStorage to ensure consistent behavior across backends.
package storage

import (
	"fmt"
	"regexp"
	"slices"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// ============================================================================
// Shared Permission Checking
// ============================================================================

// CheckPermission checks if the security context has permission for the operation.
// This is a shared utility used by both file and graph storage backends.
//
// Permission format: "operation:kind" (e.g., "read:backlog_item", "write:*")
// Special handling:
//   - System account always has access
//   - Admin role has all permissions
//   - Wildcard permissions (e.g., "read:*") grant access to all kinds
//
// Returns ErrPermissionDenied if permission is denied, nil otherwise.
func CheckPermission(secCtx *pkgctx.SecurityContext, operation, kind string) error {
	// System account always has access
	if secCtx.AccountID == pkgctx.SystemAccountID {
		return nil
	}

	// Check for wildcard permissions
	for _, perm := range secCtx.Permissions {
		if perm == fmt.Sprintf("%s:*", operation) {
			return nil
		}
		if perm == fmt.Sprintf("%s:%s", operation, kind) {
			return nil
		}
	}

	// Check roles (admin has all permissions)
	if slices.Contains(secCtx.Roles, "admin") {
		return nil
	}

	return ErrPermissionDenied
}

// CheckPermissionWithKindSpecialCases checks permission with special handling for specific kinds.
// This extends CheckPermission with kind-specific logic (e.g., keystore_entry).
//
// For keystore_entry:
//   - Read access is always allowed (filtering happens in Read/List methods)
//   - Write access requires admin or ownership (checked in Update/Delete methods)
func CheckPermissionWithKindSpecialCases(secCtx *pkgctx.SecurityContext, operation, kind string) error {
	// First check standard permissions
	if err := CheckPermission(secCtx, operation, kind); err == nil {
		return nil
	}

	// Special handling for keystore_entry
	if kind == objects.KindKeystoreEntry {
		if operation == "read" {
			// Read access is allowed - filtering by account_id happens in Read/List
			return nil
		}
		// Write operations require admin or ownership (checked in Update/Delete)
		// For now, allow if admin, otherwise check in Update/Delete
		if slices.Contains(secCtx.Roles, "admin") {
			return nil
		}
		// Non-admin write access will be checked in Update/Delete by verifying account_id
		return nil
	}

	return ErrPermissionDenied
}

// ============================================================================
// Shared Metadata Management
// ============================================================================

// EnsureObjectMetadata ensures required metadata fields are set on an object.
// This is a shared utility used by both file and graph storage backends.
//
// Sets:
//   - created_at, created_by (if isCreate is true and not already set)
//   - updated_at, updated_by (always set)
//   - namespace_id (derived from object ID if not already set)
//
// The namespace_id is derived from the object ID to enable namespace-aware operations
// while maintaining backward compatibility with legacy IDs.
//
// idValidator can be nil - if provided, it's used to infer kind from ID for namespace lookup.
func EnsureObjectMetadata(obj map[string]any, secCtx *pkgctx.SecurityContext, isCreate bool, idValidator IDValidatorProvider) {
	now := zqktime.NowRFC3339UTC()

	if isCreate {
		if _, ok := obj[objects.FieldKeyCreatedAt]; !ok {
			obj[objects.FieldKeyCreatedAt] = now
		}
		if _, ok := obj[objects.FieldKeyCreatedBy]; !ok {
			obj[objects.FieldKeyCreatedBy] = secCtx.AccountID
		}
		// Ensure create status is a valid lifecycle origin (or another valid status).
		// Missing or invalid status → lifecycle origin. Avoids stranded objects
		// (promote requires a known status) without CLI hard-reject friction.
		ensureCreateLifecycleStatus(obj)
	}

	// Always set updated_at and updated_by
	obj[objects.FieldKeyUpdatedAt] = now
	obj[objects.FieldKeyUpdatedBy] = secCtx.AccountID

	// Inject version_context defaulting to 'default' if missing (Ontology Layer Scoping)
	if _, hasVersion := obj[ConstVersionContext]; !hasVersion {
		obj[ConstVersionContext] = "default"
	}

	// Normalize or set namespace_id so it always matches the validation pattern.
	// Invalid values (e.g. "cli" from operation_type) cause "namespace_id does not match pattern" on create.
	kind, _ := obj[objects.FieldKeyKind].(string)
	if ns := objects.GetString(obj, objects.FieldKeyNamespaceID); ns != emptyValue {
		config := validation.GetGlobalNamespacesConfig()
		if config != nil {
			pattern := config.GetNamespaceValidationPattern()
			if pattern != emptyValue {
				if re, err := regexp.Compile(pattern); err == nil && !re.MatchString(ns) {
					// Present but invalid — replace with default for this kind
					if kind != emptyValue {
						registry := validation.GetNamespaceRegistry()
						if defaultNS := registry.GetNamespaceForKind(kind); defaultNS != emptyValue {
							obj[objects.FieldKeyNamespaceID] = defaultNS
						}
					}
				}
			}
		}
	}

	// Derive namespace from object ID if not already set
	// This enables namespace-aware operations while maintaining backward compatibility
	if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
		if _, hasNamespace := obj[objects.FieldKeyNamespaceID]; !hasNamespace {
			parsed := validation.ParseNamespace(id)
			if parsed != nil && parsed.NamespaceID != emptyValue {
				// Object has a namespaced ID, store the namespace
				obj[objects.FieldKeyNamespaceID] = parsed.NamespaceID
			} else if idValidator != nil {
				// Legacy format (no namespace) - look up default namespace for this kind
				if kind == emptyValue {
					// Try to infer kind from ID
					kind = idValidator.InferKindFromID(id)
				}
				if kind != emptyValue {
					registry := validation.GetNamespaceRegistry()
					namespaceID := registry.GetNamespaceForKind(kind)
					if namespaceID != emptyValue {
						obj[objects.FieldKeyNamespaceID] = namespaceID
					}
				}
			}
		}
	}
}

// IDValidatorProvider provides ID validation capabilities without importing specific storage types
// This allows the shared metadata function to work with both file and graph storage
type IDValidatorProvider interface {
	InferKindFromID(id string) string
}

// IDValidatorAdapter adapts *validation.IDValidator to IDValidatorProvider interface
type IDValidatorAdapter struct {
	IDValidator *validation.IDValidator
}

// InferKindFromID implements IDValidatorProvider interface
func (a *IDValidatorAdapter) InferKindFromID(id string) string {
	if a.IDValidator == nil {
		return ""
	}
	return a.IDValidator.InferKindFromID(id)
}
