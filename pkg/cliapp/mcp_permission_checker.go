package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

const mcpPermissionRoleAdmin = "admin"

// MCPPermissionCheckerAdapter adapts MCP permission checking to CLI format handler permission checking
// This bridges the MCP permission system with the CLI format handler system
type MCPPermissionCheckerAdapter struct {
	// These interfaces avoid import cycles - they're defined in pkg/mcp
	permissionCacheInterface   any // *mcp.PermissionCache (avoid import cycle)
	specAccessControlInterface any // *mcp.SpecAccessControl (avoid import cycle)
	secCtx                     *pkgctx.SecurityContext
	mu                         sync.RWMutex
}

// NewMCPPermissionCheckerAdapter creates a new adapter for MCP permission checking
// This allows the CLI format handler system to use MCP's permission cache and spec access control
func NewMCPPermissionCheckerAdapter(
	permissionCache any,
	specAccessControl any,
	secCtx *pkgctx.SecurityContext,
) *MCPPermissionCheckerAdapter {
	return &MCPPermissionCheckerAdapter{
		permissionCacheInterface:   permissionCache,
		specAccessControlInterface: specAccessControl,
		secCtx:                     secCtx,
	}
}

// CheckPermission checks if the security context has permission for the operation
// Returns (allowed, reason) - reason is empty if allowed
func (a *MCPPermissionCheckerAdapter) CheckPermission(ctx context.Context, operation, resource string) (allowed bool, reason string) {
	var secCtx *pkgctx.SecurityContext
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&a.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameMcpPermissionCheckerCheckPermission,
		func() error {
			secCtx = a.secCtx
			return nil
		},
	)

	if secCtx == nil {
		return false, "no security context"
	}

	// Check if user is active (if permission cache is available)
	if a.permissionCacheInterface != nil {
		// Use type assertion with any to avoid import cycle
		// The permission cache interface has IsUserActive method
		if pc, ok := a.permissionCacheInterface.(interface {
			IsUserActive(accountID string) bool
		}); ok {
			if !pc.IsUserActive(secCtx.AccountID) {
				return false, "user is inactive"
			}
		}
	}

	// Admin has all permissions
	if slices.Contains(secCtx.Roles, mcpPermissionRoleAdmin) {
		return true, ""
	}

	// Check specific permission
	requiredPerm := fmt.Sprintf("%s:%s", operation, resource)
	wildcardPerm := fmt.Sprintf("%s:*", operation)

	for _, perm := range secCtx.Permissions {
		if perm == requiredPerm || perm == wildcardPerm {
			return true, ""
		}
	}

	return false, fmt.Sprintf("missing permission: %s", requiredPerm)
}

func (a *MCPPermissionCheckerAdapter) checkBaseAccess(secCtx *pkgctx.SecurityContext) (bool, string, bool) {
	if secCtx == nil {
		return false, "no security context", true
	}
	if a.permissionCacheInterface != nil {
		if pc, ok := a.permissionCacheInterface.(interface {
			IsUserActive(accountID string) bool
		}); ok {
			if !pc.IsUserActive(secCtx.AccountID) {
				return false, "user is inactive", true
			}
		}
	}
	for _, role := range secCtx.Roles {
		if role == "admin" {
			return true, "", true
		}
	}
	return false, "", false
}

// CheckFormatPermission checks if format is allowed for the security context
// Some formats (like json-rpc streaming) may require special permissions
func (a *MCPPermissionCheckerAdapter) CheckFormatPermission(ctx context.Context, format OutputFormat) (bool, string) {
	var secCtx *pkgctx.SecurityContext
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&a.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameMcpPermissionCheckerCheckFormat,
		func() error {
			secCtx = a.secCtx
			return nil
		},
	)

	if allowed, reason, handled := a.checkBaseAccess(secCtx); handled {
		return allowed, reason
	}

	// Streaming formats (json-rpc, stream) may require special permissions
	if format == FormatJSONRPC || format == FormatStream {
		// Check for streaming permission
		hasStreamingPerm := false
		for _, perm := range secCtx.Permissions {
			if perm == "stream:*" || perm == "format:json-rpc" || perm == "format:stream" {
				hasStreamingPerm = true
				break
			}
		}

		// If no explicit streaming permission, check if user has read:* (default allow)
		if !hasStreamingPerm {
			for _, perm := range secCtx.Permissions {
				if perm == "read:*" {
					hasStreamingPerm = true
					break
				}
			}
		}

		if !hasStreamingPerm {
			return false, "streaming format requires stream:* or read:* permission"
		}
	}

	// All other formats are allowed by default (table, json, yaml)
	return true, ""
}

// CheckDataAccess checks if the security context can access the data
// This is called before formatting to short-circuit unauthorized access
func (a *MCPPermissionCheckerAdapter) CheckDataAccess(ctx context.Context, data any) (bool, string) {
	var secCtx *pkgctx.SecurityContext
	var specAccessControl any
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&a.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameMcpPermissionCheckerCheckDataAccess,
		func() error {
			secCtx = a.secCtx
			specAccessControl = a.specAccessControlInterface
			return nil
		},
	)

	if allowed, reason, handled := a.checkBaseAccess(secCtx); handled {
		return allowed, reason
	}

	// If no spec access control, allow access (fallback)
	if specAccessControl == nil {
		return true, ""
	}

	// Convert data to map for access checking
	dataMap, ok := data.(map[string]any)
	if !ok {
		// Try to marshal/unmarshal if it's a struct or slice
		jsonData, err := json.Marshal(data)
		if err != nil {
			return true, "" // Can't check, default to allow
		}

		var unmarshaled any
		if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
			return true, "" // Can't check, default to allow
		}
		switch

		// Handle slices/arrays
		v := unmarshaled.(type) {
		case []any:
			// Check each item in the slice
			for i, item := range v {
				if itemMap, ok := item.(map[string]any); ok {
					allowed, reason := a.checkObjectAccess(itemMap, secCtx, specAccessControl)
					if !allowed {
						return false, fmt.Sprintf("item %d: %s", i, reason)
					}
				}
			}
			return true, ""
		case

			// Handle single object
			map[string]any:
			return a.checkObjectAccess(v, secCtx, specAccessControl)
		}

		// Unknown type, default to allow
		return true, ""
	}

	// Single object check
	return a.checkObjectAccess(dataMap, secCtx, specAccessControl)
}

// checkObjectAccess checks if the security context can access a single object
// Uses any to avoid import cycles with pkg/mcp
func (a *MCPPermissionCheckerAdapter) checkObjectAccess(
	obj map[string]any,
	secCtx *pkgctx.SecurityContext,
	specAccessControl any,
) (allowed bool, reason string) {
	// Fast path: Check permission cache if available
	if a.permissionCacheInterface != nil {
		// Use type assertion to call HasObjectAccessFast
		if pc, ok := a.permissionCacheInterface.(interface {
			HasObjectAccessFast(obj map[string]any, secCtx *pkgctx.SecurityContext) (bool, bool)
		}); ok {
			hasAccess, shouldCheckSpec := pc.HasObjectAccessFast(obj, secCtx)
			if !hasAccess {
				objID, _ := obj[objects.FieldKeyID].(string)
				objKind, _ := obj[objects.FieldKeyKind].(string)
				return false, fmt.Sprintf("access denied to %s:%s", objKind, objID)
			}
			// If fast path says we have access but should check spec, continue to spec check
			if !shouldCheckSpec {
				return true, ""
			}
		}
	}

	// Spec-based access check
	if specAccessControl != nil {
		// Use type assertion to call HasObjectAccess
		if sac, ok := specAccessControl.(interface {
			HasObjectAccess(obj map[string]any, secCtx *pkgctx.SecurityContext, operation string) bool
		}); ok {
			hasAccess := sac.HasObjectAccess(obj, secCtx, "read")
			if !hasAccess {
				objID, _ := obj[objects.FieldKeyID].(string)
				objKind, _ := obj[objects.FieldKeyKind].(string)
				return false, fmt.Sprintf("access denied to %s:%s", objKind, objID)
			}
		}
	}

	return true, ""
}

// SetSecurityContext updates the security context (thread-safe)
func (a *MCPPermissionCheckerAdapter) SetSecurityContext(secCtx *pkgctx.SecurityContext) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&a.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameMcpPermissionCheckerSetSecCtx,
		func() error {
			a.secCtx = secCtx
			return nil
		},
	)
}

// GetSecurityContext returns the current security context (thread-safe)
func (a *MCPPermissionCheckerAdapter) GetSecurityContext() *pkgctx.SecurityContext {
	var secCtx *pkgctx.SecurityContext
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&a.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameMcpPermissionCheckerGetSecCtx,
		func() error {
			secCtx = a.secCtx
			return nil
		},
	)
	return secCtx
}
