package mcp

import (
	"fmt"
	"slices"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// SpecLoader interface to avoid import cycle with pkg/objects
type SpecLoader interface {
	LoadSpecWithInheritance(filename string) (Spec, error)
}

// Spec interface to avoid import cycle
type Spec interface {
	GetResolvedFields() map[string]any
}

// PermissionCacheInterface interface for user permission management
// This allows spec access control to work with permission cache without import cycles
type PermissionCacheInterface interface {
	GetUserPermissions(accountID string) (*UserPermissions, bool)
	IsUserActive(accountID string) bool
	BuildPermissionCache(secCtx *pkgctx.SecurityContext) error
	GetSpecLoader() any // Returns *objects.SpecLoader
}

// SpecAccessControl handles access control based on object specifications
// Logger interface to avoid import cycle with pkg/logging
type Logger interface {
	Debug(msg string, fields ...LogField)
}

// LogField represents a structured log field
type LogField struct {
	Key   string
	Value any
}

// Provides field-level access control and cascading access through references
type SpecAccessControl struct {
	specLoader      SpecLoader
	permissionCache PermissionCacheInterface
	logger          Logger
	eventEmitter    *EventEmitter            // Optional event emitter for permission events
	mcpContext      *pkgctx.MCPServerContext // MCP server context for checking serving state
}

// NewSpecAccessControl creates a new SpecAccessControl instance
// If permissionCache is provided, uses cached permissions for better performance
func NewSpecAccessControl(specLoader SpecLoader) *SpecAccessControl {
	return &SpecAccessControl{
		specLoader:      specLoader,
		permissionCache: nil,
		logger:          nil, // Can be set via SetLogger to avoid import cycle
	}
}

// SetLogger sets the logger for debug output
// This allows the logger to be injected from outside to avoid import cycles
func (sac *SpecAccessControl) SetLogger(logger Logger) {
	sac.logger = logger
}

// SetEventEmitter sets the event emitter for permission events
func (sac *SpecAccessControl) SetEventEmitter(emitter *EventEmitter) {
	sac.eventEmitter = emitter
}

// SetMCPServerContext sets the MCP server context for checking serving state
// This allows dependency injection instead of global access
func (sac *SpecAccessControl) SetMCPServerContext(ctx *pkgctx.MCPServerContext) {
	sac.mcpContext = ctx
}

// SetPermissionCache sets the permission cache for optimized access control
func (sac *SpecAccessControl) SetPermissionCache(cache PermissionCacheInterface) {
	sac.permissionCache = cache
}

// FieldAccess represents access permissions for a field
type FieldAccess struct {
	ReadRoles      []string // Roles that can read this field
	ReadPerms      []string // Permissions that can read this field
	WriteRoles     []string // Roles that can write this field
	WritePerms     []string // Permissions that can write this field
	IsSensitive    bool     // Field contains sensitive information
	RequiresAccess []string // Additional access requirements (e.g., ["team-alpha", "confidential"])
}

// Helper functions for extracting object metadata
func getObjID(obj map[string]any) string {
	if id, ok := obj[objects.FieldKeyID].(string); ok {
		return id
	}
	return ""
}

// getObjKind removed - unused function

// HasFieldAccess checks if security context has access to a field using Unix-like permissions
// Default behavior: If user has read access on kind, they can read all fields unless field requires higher-level access
func (sac *SpecAccessControl) HasFieldAccess(fieldName string, obj map[string]any, secCtx *pkgctx.SecurityContext, operation string) bool {
	if secCtx == nil {
		return false
	}

	// CRITICAL: Suppress all debug logs during MCP server operation
	// Debug logs pollute stderr and break JSON-RPC protocol communication in MCP mode
	// The logger framework checks MCPServerContext.IsServing(), but we suppress here
	// to avoid unnecessary logger calls and ensure no stderr pollution

	// Check if user is active (if permission cache is available)
	// Only check if cache has been built for this user (to avoid false negatives)
	if sac.permissionCache != nil {
		// Always check active status - IsUserActive returns true by default if user not in cache
		// This allows the permission system to work even if cache hasn't been built yet
		isActive := sac.permissionCache.IsUserActive(secCtx.AccountID)
		// CRITICAL: Suppress all debug logs during MCP server operation
		// Debug logs pollute stderr and break JSON-RPC protocol communication in MCP mode
		if !isActive {
			return false
		}
	}

	// Admin has access to all fields
	if slices.Contains(secCtx.Roles, "admin") {
		return true
	}

	// Get object kind
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok {
		// Can't determine access without kind - default to allowing
		// CRITICAL: Suppress all debug logs during MCP server operation
		return true
	}

	// Check if user has kind-level read access
	hasKindLevelRead := slices.Contains(secCtx.Permissions, fmt.Sprintf("read:%s", kind)) ||
		slices.Contains(secCtx.Permissions, "read:*")
	// CRITICAL: Suppress all debug logs during MCP server operation

	// Load spec for this kind
	if sac.specLoader == nil {
		// No spec loader - if user has kind-level read, allow access (Unix-like: read on kind = read all fields)
		// But also allow public fields (no requirements) even without kind-level read
		if !hasKindLevelRead && operation == "read" {
			// Public field - allow read even without kind-level permission
			// CRITICAL: Suppress all debug logs during MCP server operation
			return true
		}
		// CRITICAL: Suppress all debug logs during MCP server operation
		return hasKindLevelRead
	}

	spec, err := sac.specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		// If spec can't be loaded, default to kind-level read access
		// But also allow public fields (no requirements) even without kind-level read
		// CRITICAL: Suppress all debug logs during MCP server operation
		if !hasKindLevelRead && operation == "read" {
			// Public field - allow read even without kind-level permission
			return true
		}
		return hasKindLevelRead
	}

	// Get field definition
	resolvedFields := spec.GetResolvedFields()
	fieldDef, ok := resolvedFields[fieldName]
	if !ok {
		// Field not in spec - default to allowing access if kind-level read exists
		// CRITICAL: Suppress all debug logs during MCP server operation
		return hasKindLevelRead
	}

	fieldMap, ok := fieldDef.(map[string]any)
	if !ok {
		// Malformed field definition - default to allowing access
		return hasKindLevelRead
	}

	// Parse Unix-like field permissions
	fieldPerm := ParseFieldPermission(fieldMap)

	// Convert security context to map for permission checking
	secCtxMap := map[string]any{
		objects.FieldKeyRoles:       secCtx.Roles,
		objects.FieldKeyPermissions: secCtx.Permissions,
	}

	// Check field access using Unix-like permission model
	hasAccess := fieldPerm.HasFieldAccess(hasKindLevelRead, operation, secCtxMap)

	// Emit permission event
	if sac.eventEmitter != nil {
		eventType := EventTypePermissionGranted
		if !hasAccess {
			eventType = EventTypePermissionDenied
		}
		sac.eventEmitter.Emit(&Event{
			Type:    eventType,
			Message: fmt.Sprintf("Field access %s: %s.%s", map[bool]string{true: "granted", false: "denied"}[hasAccess], kind, fieldName),
			Fields: map[string]any{
				"user":                    secCtx.AccountID,
				objects.FieldKeyField:     fieldName,
				objects.FieldKeyKind:      kind,
				objects.FieldKeyOperation: operation,
				"obj_id":                  getObjID(obj),
			},
			Severity: map[bool]string{true: "info", false: "warn"}[hasAccess],
		})
	}

	return hasAccess
}

// FilterObjectFields filters object fields based on access control
// Returns a new object with only accessible fields
func (sac *SpecAccessControl) FilterObjectFields(obj map[string]any, secCtx *pkgctx.SecurityContext, operation string) map[string]any {
	if secCtx == nil {
		// No security context means unauthenticated request
		return map[string]any{
			"error":      "authentication required",
			"error_code": Unauthenticated,
			"error_type": "unauthenticated",
		}
	}

	filtered := make(map[string]any)

	// Always include id and kind (needed for references)
	if id, ok := obj[objects.FieldKeyID].(string); ok {
		filtered[objects.FieldKeyID] = id
	}
	if kind, ok := obj[objects.FieldKeyKind].(string); ok {
		filtered[objects.FieldKeyKind] = kind
	}

	// Filter each field
	for fieldName, fieldValue := range obj {
		// Skip already included fields
		if fieldName == objects.FieldKeyID || fieldName == objects.FieldKeyKind {
			continue
		}

		// Check access
		if sac.HasFieldAccess(fieldName, obj, secCtx, operation) {
			filtered[fieldName] = fieldValue
		} else {
			// Field is restricted - replace with placeholder or omit
			// For sensitive fields, we might want to indicate they exist but are restricted
			// For now, we'll just omit them
		}
	}

	return filtered
}

// HasCascadingAccess checks if user has cascading access to a referenced object
// Cascading access: if user has access to object A, and A references B, user can see B
// (but with field-level restrictions still applied)
func (sac *SpecAccessControl) HasCascadingAccess(referencedObj, referencingObj map[string]any, secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}

	// Admin has access to all
	if slices.Contains(secCtx.Roles, "admin") {
		return true
	}

	// If user has direct access to the referenced object, allow it
	if sac.HasObjectAccess(referencedObj, secCtx) {
		return true
	}

	// Cascading access: if user has access to the referencing object,
	// they get limited access to referenced objects
	if sac.HasObjectAccess(referencingObj, secCtx) {
		// User has access to referencing object - grant cascading access
		// Field-level restrictions still apply
		return true
	}

	return false
}

// HasObjectAccess checks if user has access to an object at the kind level
// This is the base check before field-level filtering
func (sac *SpecAccessControl) HasObjectAccess(obj map[string]any, secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}

	// Admin has access to all
	if slices.Contains(secCtx.Roles, "admin") {
		return true
	}

	// Get object kind
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok {
		return false
	}

	// Check kind-level permission
	hasAccess := slices.Contains(secCtx.Permissions, fmt.Sprintf("read:%s", kind)) ||
		slices.Contains(secCtx.Permissions, "read:*")

	// Emit permission event
	if sac.eventEmitter != nil {
		eventType := EventTypePermissionGranted
		if !hasAccess {
			eventType = EventTypePermissionDenied
		}
		sac.eventEmitter.Emit(&Event{
			Type:    eventType,
			Message: fmt.Sprintf("Object access %s: %s", map[bool]string{true: "granted", false: "denied"}[hasAccess], kind),
			Fields: map[string]any{
				"user":               secCtx.AccountID,
				objects.FieldKeyKind: kind,
				"obj_id":             getObjID(obj),
			},
			Severity: map[bool]string{true: "info", false: "warn"}[hasAccess],
		})
	}

	return hasAccess
}

// extractFieldAccess removed - unused function, use extractFieldAccessRules instead

// checkReadAccess, checkWriteAccess, and hasAccessPermissionSpec removed - unused functions
