package mcp

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// PermissionCache manages user permissions based on object specifications
// Provides activation/deactivation of user access and permission caching
// Optimized for performance with pre-computed privilege tag checks
type PermissionCache struct {
	specLoader      SpecLoader                  // Interface to avoid import cycle
	userPermissions map[string]*UserPermissions // accountID -> permissions
	userStatus      map[string]bool             // accountID -> active (true) or inactive (false)
	// Privilege tag cache: objectID -> []privilege tags (fast lookup)
	privilegeTags map[string][]string // objectID -> privilege tags
	// Pre-computed access cache: (accountID, objectID) -> hasAccess (bool)
	// This is the fast path - checked before any spec loading
	accessCache        map[string]bool // key: accountID:objectID -> hasAccess
	cacheHits          atomic.Int64    // Lifetime cache hit count
	cacheMisses        atomic.Int64    // Lifetime cache miss count
	invalidationsTotal atomic.Int64    // Lifetime cache invalidations count
	mu                 sync.RWMutex
	lastValidated      time.Time
	logger             Logger
	eventEmitter       *EventEmitter            // Optional event emitter for permission events
	mcpContext         *pkgctx.MCPServerContext // MCP server context for checking serving state
}

// GetPermissionCacheStats returns lifetime counters for access cache hits, misses, and invalidations.
func (pc *PermissionCache) GetPermissionCacheStats() (hits, misses, invalidations int64) {
	if pc == nil {
		return 0, 0, 0
	}
	return pc.cacheHits.Load(), pc.cacheMisses.Load(), pc.invalidationsTotal.Load()
}

// UserPermissions represents cached permissions for a user
type UserPermissions struct {
	AccountID   string
	Roles       []string
	Permissions []string
	FieldAccess map[string]map[string]*FieldAccessRules // kind -> field -> access rules
	// Pre-computed privilege tag permissions for fast checking
	PrivilegeTagPerms map[string]bool // privilege tag -> has access
	Active            bool
	LastUpdated       time.Time
}

// FieldAccessRules represents access rules for a specific field
type FieldAccessRules struct {
	ReadRoles      []string
	ReadPerms      []string
	WriteRoles     []string
	WritePerms     []string
	IsSensitive    bool
	RequiresAccess []string
}

// NewPermissionCache creates a new permission cache manager
func NewPermissionCache(specLoader SpecLoader) *PermissionCache {
	return &PermissionCache{
		specLoader:      specLoader,
		userPermissions: make(map[string]*UserPermissions),
		userStatus:      make(map[string]bool),
		privilegeTags:   make(map[string][]string),
		accessCache:     make(map[string]bool),
		lastValidated:   time.Time{},
		logger:          nil, // Can be set via SetLogger to avoid import cycle
	}
}

// SetLogger sets the logger for debug output
// This allows the logger to be injected from outside to avoid import cycles
func (pc *PermissionCache) SetLogger(logger Logger) {
	pc.logger = logger
}

// SetEventEmitter sets the event emitter for permission events
func (pc *PermissionCache) SetEventEmitter(emitter *EventEmitter) {
	pc.eventEmitter = emitter
}

// SetMCPServerContext sets the MCP server context for checking serving state
// This allows dependency injection instead of global access
func (pc *PermissionCache) SetMCPServerContext(ctx *pkgctx.MCPServerContext) {
	pc.mcpContext = ctx
}

// GetEventEmitter returns the event emitter if set
func (pc *PermissionCache) GetEventEmitter() *EventEmitter {
	return pc.eventEmitter
}

// BuildPermissionCache builds the permission cache for a user based on their security context
// This analyzes all object specs and extracts field-level access rules
func (pc *PermissionCache) BuildPermissionCache(secCtx *pkgctx.SecurityContext) error {
	if secCtx == nil {
		return errfmt.Errorf("security context is required")
	}

	// Debug logging removed - these fmt.Printf calls were writing to stderr
	// during MCP server initialization, which IDE interprets as JSON-RPC responses
	// If debug logging is needed, use the logger field which respects MCPServerContext.IsServing()

	// Copy-out: Get user status (under lock)
	var active bool
	err := concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheBuildCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Check if user is active (default to active if not set)
			var ok bool
			active, ok = pc.userStatus[secCtx.AccountID]
			if !ok {
				// User not in status map - default to active
				active = true
				pc.userStatus[secCtx.AccountID] = true
			}
			return nil
		},
	)
	if err != nil {
		return err
	}

	if !active {
		// User is inactive - don't build cache, but mark as inactive
		_ = concurrency.RunInLockWithLogger(
			&pc.mu, LockNamePermissionCacheBuildInactive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				pc.userPermissions[secCtx.AccountID] = &UserPermissions{
					AccountID:         secCtx.AccountID,
					Roles:             secCtx.Roles,
					Permissions:       secCtx.Permissions,
					FieldAccess:       make(map[string]map[string]*FieldAccessRules),
					PrivilegeTagPerms: make(map[string]bool),
					Active:            false,
					LastUpdated:       time.Now(),
				}
				return nil
			},
		)
		return nil
	}

	// Process: Build field access rules and privilege tag permissions (I/O outside lock)
	fieldAccess := make(map[string]map[string]*FieldAccessRules)

	// Load all specs and extract access rules (I/O operations)
	commonKinds := []string{
		objects.KindBacklogItem, objects.KindRequirement, objects.KindDecision, objects.KindMilestone,
		objects.KindGoal, objects.KindWorkstream, objects.KindPolicy, objects.KindRule,
	}

	// Try to load specs, but don't fail if they're not found (for testing)
	for _, kind := range commonKinds {
		spec, err := pc.specLoader.LoadSpecWithInheritance(kind + ".yaml")
		if err != nil {
			// Spec not found - this is OK, continue
			continue
		}

		// Get resolved fields from spec
		resolvedFields := spec.GetResolvedFields()

		kindFieldAccess := make(map[string]*FieldAccessRules)
		for fieldName, fieldDef := range resolvedFields {
			fieldMap, ok := fieldDef.(map[string]any)
			if !ok {
				continue
			}

			// Extract access rules from field definition
			rules := extractFieldAccessRules(fieldMap)
			if rules != nil {
				kindFieldAccess[fieldName] = rules
			}
		}

		if len(kindFieldAccess) > 0 {
			fieldAccess[kind] = kindFieldAccess
		}
	}

	// Pre-compute privilege tag permissions for fast checking
	privilegeTagPerms := make(map[string]bool)

	// Extract access permissions from security context
	accessPerms := extractAccessPermissions(secCtx.Permissions)

	// Build privilege tag permission map
	// If user has "access:*", they have access to all privilege tags
	hasWildcardAccess := false
	for _, perm := range accessPerms {
		if perm == "access:*" {
			hasWildcardAccess = true
			break
		}
	}

	if hasWildcardAccess {
		// User has wildcard access - grant all privilege tags
		// We'll mark this with a special key
		privilegeTagPerms["*"] = true
	} else {
		// Build map of privilege tags user has access to
		for _, perm := range accessPerms {
			// Extract privilege tag from permission (e.g., "access:team-alpha" -> "team-alpha")
			if strings.HasPrefix(perm, "access:") {
				tag := strings.TrimPrefix(perm, "access:")
				privilegeTagPerms[tag] = true
			}
		}
	}

	// Copy-in: Cache the permissions (under lock)
	err = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheBuildUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pc.userPermissions[secCtx.AccountID] = &UserPermissions{
				AccountID:         secCtx.AccountID,
				Roles:             secCtx.Roles,
				Permissions:       secCtx.Permissions,
				FieldAccess:       fieldAccess,
				PrivilegeTagPerms: privilegeTagPerms,
				Active:            true,
				LastUpdated:       time.Now(),
			}

			// Ensure user status is set to active
			pc.userStatus[secCtx.AccountID] = true
			pc.lastValidated = time.Now()
			return nil
		},
	)
	return err
}

// GetUserPermissions returns cached permissions for a user
func (pc *PermissionCache) GetUserPermissions(accountID string) (*UserPermissions, bool) {
	var perms *UserPermissions
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&pc.mu, LockNamePermissionCacheGetUserPermissions, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			perms, ok = pc.userPermissions[accountID]
			exists = ok
			return nil
		},
	)
	return perms, exists
}

// IsUserActive checks if a user is active
func (pc *PermissionCache) IsUserActive(accountID string) bool {
	var active bool
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&pc.mu, LockNamePermissionCacheIsUserActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			active, ok = pc.userStatus[accountID]
			exists = ok
			return nil
		},
	)
	// CRITICAL: Suppress all debug logs during MCP server operation (see BuildPermissionCache for details)
	// Debug logs pollute stderr and break JSON-RPC protocol communication in MCP mode
	if !exists {
		// User not in status map - default to active
		return true
	}
	return active
}

// ActivateUser activates a user, allowing them to access the system
func (pc *PermissionCache) ActivateUser(accountID string) error {
	var eventEmitter *EventEmitter
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheActivateUser, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pc.userStatus[accountID] = true

			// If permissions are cached, mark as active
			if perms, ok := pc.userPermissions[accountID]; ok {
				perms.Active = true
				perms.LastUpdated = time.Now()
			}

			eventEmitter = pc.eventEmitter
			return nil
		},
	)

	// Emit user activated event (outside lock to avoid deadlock)
	if eventEmitter != nil {
		eventEmitter.Emit(&Event{
			Type:    EventTypeUserActivated,
			Message: fmt.Sprintf("User activated: %s", accountID),
			Fields: map[string]any{
				"user": accountID,
			},
			Severity: "info",
		})
	}

	return nil
}

// DeactivateUser deactivates a user, preventing them from accessing the system
func (pc *PermissionCache) DeactivateUser(accountID string) error {
	var eventEmitter *EventEmitter
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheDeactivateUser, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pc.userStatus[accountID] = false

			// If permissions are cached, mark as inactive
			if perms, ok := pc.userPermissions[accountID]; ok {
				perms.Active = false
				perms.LastUpdated = time.Now()
			}

			eventEmitter = pc.eventEmitter
			return nil
		},
	)

	// Emit user deactivated event (outside lock to avoid deadlock)
	if eventEmitter != nil {
		eventEmitter.Emit(&Event{
			Type:    EventTypeUserDeactivated,
			Message: fmt.Sprintf("User deactivated: %s", accountID),
			Fields: map[string]any{
				"user": accountID,
			},
			Severity: "warn",
		})
	}

	return nil
}

// RefreshUserPermissions rebuilds the permission cache for a user
// Useful when specs change or user roles/permissions are updated
func (pc *PermissionCache) RefreshUserPermissions(secCtx *pkgctx.SecurityContext) error {
	// Remove old cache entry
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheRefreshRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(pc.userPermissions, secCtx.AccountID)
			return nil
		},
	)

	// Rebuild cache
	return pc.BuildPermissionCache(secCtx)
}

// ClearCache clears all permission caches
func (pc *PermissionCache) ClearCache() {
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheClear, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pc.userPermissions = make(map[string]*UserPermissions)
			pc.accessCache = make(map[string]bool)
			pc.privilegeTags = make(map[string][]string)
			pc.lastValidated = time.Time{}
			return nil
		},
	)
}

// HasObjectAccessFast performs fast privilege tag-based access check
// This is the first check that should happen before any spec loading
// Returns (hasAccess, shouldCheckSpec)
// If hasAccess is false, deny immediately (fast path)
// If hasAccess is true and shouldCheckSpec is true, continue to spec-based checks
func (pc *PermissionCache) HasObjectAccessFast(obj map[string]any, secCtx *pkgctx.SecurityContext) (hasAccess, shouldCheckSpec bool) {
	if secCtx == nil {
		return false, false
	}

	// Fast path: Check if user is active (atomic read)
	var active bool
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&pc.mu, LockNamePermissionCacheCheckAccessActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			active, ok = pc.userStatus[secCtx.AccountID]
			exists = ok
			return nil
		},
	)

	if exists && !active {
		// User is deactivated - deny immediately
		return false, false
	}

	// Get object ID for cache key
	objID, ok := obj[objects.FieldKeyID].(string)
	if !ok {
		// No ID - can't cache, proceed to spec check
		return true, true
	}

	// Check pre-computed access cache (fast path)
	cacheKey := fmt.Sprintf("%s:%s", secCtx.AccountID, objID)
	var cached bool
	var foundInCache bool
	_ = concurrency.RunInRLockWithLogger(
		&pc.mu, LockNamePermissionCacheCheckAccessCache, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			cached, ok = pc.accessCache[cacheKey]
			foundInCache = ok
			return nil
		},
	)

	if foundInCache {
		pc.cacheHits.Add(1)
		// Found in cache - return cached result
		// If cached is true, still need to check spec for field-level filtering
		return cached, cached
	}

	pc.cacheMisses.Add(1)

	// Not in cache - extract privilege tags and check
	privilegeTags := extractPrivilegeTags(obj)

	// If no privilege tags, grant access (proceed to spec check)
	if len(privilegeTags) == 0 {
		// Cache the result (no tags = accessible)
		_ = concurrency.RunInLockWithLogger(
			&pc.mu, LockNamePermissionCacheCheckAccessCacheNoTags, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				pc.accessCache[cacheKey] = true
				return nil
			},
		)
		return true, true
	}

	// Get user permissions for privilege tag checking
	var userPerms *UserPermissions
	var hasUserPerms bool
	_ = concurrency.RunInRLockWithLogger(
		&pc.mu, LockNamePermissionCacheCheckAccessGetPerms, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			userPerms, ok = pc.userPermissions[secCtx.AccountID]
			hasUserPerms = ok
			return nil
		},
	)

	if !hasUserPerms {
		// User permissions not cached - need to build cache
		// For now, deny access (will be granted after cache is built)
		return false, false
	}

	// Check if user has wildcard access
	if userPerms.PrivilegeTagPerms["*"] {
		// User has wildcard access - grant access, cache it
		_ = concurrency.RunInLockWithLogger(
			&pc.mu, LockNamePermissionCacheCheckAccessCacheWildcard, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				pc.accessCache[cacheKey] = true
				return nil
			},
		)
		return true, true
	}

	// Check if user has access to any of the object's privilege tags
	hasAccess = false
	for _, tag := range privilegeTags {
		if userPerms.PrivilegeTagPerms[tag] {
			hasAccess = true
			break
		}
	}

	// Cache the result
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheCheckAccessCacheResult, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			pc.accessCache[cacheKey] = hasAccess
			pc.privilegeTags[objID] = privilegeTags // Cache tags for invalidation
			return nil
		},
	)

	return hasAccess, hasAccess
}

// InvalidateObjectAccess invalidates cached access for an object
// Call this when an object's privilege tags change
func (pc *PermissionCache) InvalidateObjectAccess(objID string) {
	pc.invalidationsTotal.Add(1)
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheInvalidateObject, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Remove from access cache (all users)
			// We'll rebuild on next access
			keysToDelete := make([]string, 0)
			for key := range pc.accessCache {
				if strings.HasSuffix(key, ":"+objID) {
					keysToDelete = append(keysToDelete, key)
				}
			}
			for _, key := range keysToDelete {
				delete(pc.accessCache, key)
			}

			// Remove privilege tags cache
			delete(pc.privilegeTags, objID)
			return nil
		},
	)
}

// InvalidateUserCache invalidates all cached access for a user
// Call this when user's roles/permissions change
func (pc *PermissionCache) InvalidateUserCache(accountID string) {
	pc.invalidationsTotal.Add(1)
	_ = concurrency.RunInLockWithLogger(
		&pc.mu, LockNamePermissionCacheInvalidateUser, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Remove all access cache entries for this user
			keysToDelete := make([]string, 0)
			prefix := accountID + ":"
			for key := range pc.accessCache {
				if strings.HasPrefix(key, prefix) {
					keysToDelete = append(keysToDelete, key)
				}
			}
			for _, key := range keysToDelete {
				delete(pc.accessCache, key)
			}

			// Remove user permissions (will be rebuilt on next access)
			delete(pc.userPermissions, accountID)
			return nil
		},
	)
}

// extractPrivilegeTags extracts privilege tags from an object
// extractPrivilegeTags is a convenience wrapper around ExtractPrivilegeTags.
// Kept for backward compatibility with existing code.
func extractPrivilegeTags(obj map[string]any) []string {
	return ExtractPrivilegeTags(obj)
}

// extractAccessPermissions is a convenience wrapper around ExtractAccessPermissions.
// Kept for backward compatibility with existing code.
func extractAccessPermissions(permissions []string) []string {
	return ExtractAccessPermissions(permissions)
}

// GetSpecLoader returns the underlying spec loader
// Implements PermissionCache interface
func (pc *PermissionCache) GetSpecLoader() any {
	return pc.specLoader
}

// GetSpecLoaderTyped returns the spec loader as SpecLoader interface
func (pc *PermissionCache) GetSpecLoaderTyped() SpecLoader {
	return pc.specLoader
}

// extractFieldAccessRules extracts access rules from a field definition
func extractFieldAccessRules(fieldMap map[string]any) *FieldAccessRules {
	rules := &FieldAccessRules{}

	// Extract from access metadata
	if accessMeta, ok := fieldMap["access"].(map[string]any); ok {
		// Read access
		if readMeta, ok := accessMeta["read"].(map[string]any); ok {
			if roles, ok := readMeta[objects.FieldKeyRoles].([]any); ok {
				rules.ReadRoles = make([]string, len(roles))
				for i, r := range roles {
					if role, ok := r.(string); ok {
						rules.ReadRoles[i] = role
					}
				}
			}
			if perms, ok := readMeta[objects.FieldKeyPermissions].([]any); ok {
				rules.ReadPerms = make([]string, len(perms))
				for i, p := range perms {
					if perm, ok := p.(string); ok {
						rules.ReadPerms[i] = perm
					}
				}
			}
		}

		// Write access
		if writeMeta, ok := accessMeta["write"].(map[string]any); ok {
			if roles, ok := writeMeta[objects.FieldKeyRoles].([]any); ok {
				rules.WriteRoles = make([]string, len(roles))
				for i, r := range roles {
					if role, ok := r.(string); ok {
						rules.WriteRoles[i] = role
					}
				}
			}
			if perms, ok := writeMeta[objects.FieldKeyPermissions].([]any); ok {
				rules.WritePerms = make([]string, len(perms))
				for i, p := range perms {
					if perm, ok := p.(string); ok {
						rules.WritePerms[i] = perm
					}
				}
			}
		}

		// Additional access requirements
		if requires, ok := accessMeta["requires"].([]any); ok {
			rules.RequiresAccess = make([]string, len(requires))
			for i, r := range requires {
				if req, ok := r.(string); ok {
					rules.RequiresAccess[i] = req
				}
			}
		}
	}

	// Extract sensitivity from checklist
	if checklist, ok := fieldMap["checklist"].(map[string]any); ok {
		if security, ok := checklist["security"].(string); ok {
			securityLower := strings.ToLower(security)
			if strings.Contains(securityLower, "sensitive") ||
				strings.Contains(securityLower, "confidential") {
				rules.IsSensitive = true
			}
		}
	}

	// Only return rules if there are any restrictions
	if len(rules.ReadRoles) == 0 && len(rules.ReadPerms) == 0 &&
		len(rules.WriteRoles) == 0 && len(rules.WritePerms) == 0 &&
		!rules.IsSensitive && len(rules.RequiresAccess) == 0 {
		return nil // No restrictions
	}

	return rules
}
