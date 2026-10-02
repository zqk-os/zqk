// Package storage provides a unified storage abstraction layer for zqk,
// supporting both file-based and graph-based storage backends through a
// common interface.
//
// This file contains shared utilities used by both FileObjectStorage and
// GraphObjectStorage to ensure consistent behavior across backends.
package storage

import (
	"context"
	"fmt"
	"slices"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// ClampInt clamps val to the [minVal, maxVal] range.
func ClampInt(val, minVal, maxVal int) int {
	return min(max(val, minVal), maxVal)
}

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
	if secCtx == nil {
		return ErrPermissionDenied
	}
	// System account always has access (including retired "system" / "account:system" aliases).
	if pkgctx.IsSystemAccount(secCtx.AccountID) {
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
		// delete:core → delete of kernel-critical kinds only (reason-code waived separately).
		// TRACK: follow-up in kernel backlog
		if operation == OpDelete && perm == pkgctx.PermissionDeleteCore && isCoreKernelKind(kind) {
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
// On create, ctx may carry WithPromoteOnCreate so a valid shovel-ready status skips draft-first coerce.
func EnsureObjectMetadata(ctx context.Context, obj map[string]any, secCtx *pkgctx.SecurityContext, isCreate bool, idValidator IDValidatorProvider) {
	now := zqktime.NowRFC3339UTC()

	actor := pkgctx.ActorIDForAttribution(secCtx.AccountID)

	if isCreate {
		if !pkgctx.IsLifecycleBreakGlass(ctx) && (secCtx == nil || !pkgctx.IsSystemAccount(secCtx.AccountID)) {
			obj[objects.FieldKeyCreatedAt] = now
			obj[objects.FieldKeyCreatedBy] = actor
			delete(obj, "cas_address")
			delete(obj, "hash")
		} else {
			if _, ok := obj[objects.FieldKeyCreatedAt]; !ok {
				obj[objects.FieldKeyCreatedAt] = now
			}
			if existing, ok := obj[objects.FieldKeyCreatedBy].(string); !ok || existing == emptyValue {
				obj[objects.FieldKeyCreatedBy] = actor
			} else {
				obj[objects.FieldKeyCreatedBy] = pkgctx.ActorIDForAttribution(existing)
			}
		}
		// Ensure create status is a valid lifecycle origin (or another valid status).
		// Missing or invalid status → lifecycle origin. Avoids stranded objects
		// (promote requires a known status) without CLI hard-reject friction.
		// WithPromoteOnCreate, non-CLI, or bypass kinds keep a valid non-preliminary status (skip draft plane).
		ensureCreateLifecycleStatus(ctx, obj, pkgctx.GetPromoteOnCreate(ctx))
	}

	// Always set updated_at and updated_by (never persist the retired "system" alias) unless break-glass.
	if !pkgctx.IsLifecycleBreakGlass(ctx) && (secCtx == nil || !pkgctx.IsSystemAccount(secCtx.AccountID)) {
		obj[objects.FieldKeyUpdatedAt] = now
		obj[objects.FieldKeyUpdatedBy] = actor
		delete(obj, "cas_address")
		delete(obj, "hash")
	} else {
		if _, ok := obj[objects.FieldKeyUpdatedAt]; !ok {
			obj[objects.FieldKeyUpdatedAt] = now
		}
		if _, ok := obj[objects.FieldKeyUpdatedBy]; !ok {
			obj[objects.FieldKeyUpdatedBy] = actor
		}
	}

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
				if re, err := validation.GetCachedRegexp(pattern); err == nil && !re.MatchString(ns) {
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

// ValidateIDFormatStrict loads patterns and strictly validates id against kind format rules.
func ValidateIDFormatStrict(v *validation.IDValidator, id, kind string) error {
	if v == nil {
		return nil
	}
	if err := v.LoadPatterns(); err != nil {
		return errfmt.Newf(ErrMsgLoadIDPatternsValidation).Wrap(err)
	}
	valid, err := v.ValidateID(id, kind)
	if err != nil {
		return errfmt.Newf(ErrMsgValidateID).Wrap(err)
	}
	if !valid {
		return errfmt.Errorf(ErrMsgInvalidIDFormat, kind, id)
	}
	return nil
}
