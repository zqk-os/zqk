package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// mockSpecLoader implements SpecLoader for testing
type mockSpecLoader struct {
	specs map[string]*mockSpec
}

type mockSpec struct {
	resolvedFields map[string]any
}

func (m *mockSpec) GetResolvedFields() map[string]any {
	return m.resolvedFields
}

func (m *mockSpecLoader) LoadSpecWithInheritance(filename string) (Spec, error) {
	if spec, ok := m.specs[filename]; ok {
		return spec, nil
	}
	return nil, &specNotFoundError{filename: filename}
}

type specNotFoundError struct {
	filename string
}

func (e *specNotFoundError) Error() string {
	return "spec not found: " + e.filename
}

func newMockSpecLoader() *mockSpecLoader {
	return &mockSpecLoader{
		specs: make(map[string]*mockSpec),
	}
}

func TestExtractPrivilegeTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		obj      map[string]any
		expected []string
	}{
		{
			name: "no tags",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-001",
				objects.FieldKeyKind: "backlog_item",
			},
			expected: nil,
		},
		{
			name: "access tags",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-001",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"access:team-alpha", "access:confidential"},
			},
			expected: []string{"team-alpha", "confidential"},
		},
		{
			name: "privilege tags",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-001",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"privilege:admin-only", "privilege:restricted"},
			},
			expected: []string{"admin-only", "restricted"},
		},
		{
			name: "mixed tags",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-001",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"access:team-alpha", "privilege:confidential", "regular-tag"},
			},
			expected: []string{"team-alpha", "confidential"},
		},
		{
			name: "non-string tags ignored",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-001",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"access:team-alpha", 123, true},
			},
			expected: []string{"team-alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPrivilegeTags(tt.obj)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d tags, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, tag := range tt.expected {
				if result[i] != tag {
					t.Errorf("tag[%d]: expected %q, got %q", i, tag, result[i])
				}
			}
		})
	}
}

func TestExtractAccessPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		permissions []string
		expected    []string
	}{
		{
			name:        "no access permissions",
			permissions: []string{"read:*", "write:backlog_item"},
			expected:    nil,
		},
		{
			name:        "access permissions",
			permissions: []string{"read:*", "access:team-alpha", "access:confidential"},
			expected:    []string{"access:team-alpha", "access:confidential"},
		},
		{
			name:        "wildcard access",
			permissions: []string{"read:*", "access:*", "write:*"},
			expected:    []string{"access:*"},
		},
		{
			name:        "mixed permissions",
			permissions: []string{"read:*", "access:team-alpha", "write:backlog_item", "access:team-beta"},
			expected:    []string{"access:team-alpha", "access:team-beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAccessPermissions(tt.permissions)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d permissions, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, perm := range tt.expected {
				if result[i] != perm {
					t.Errorf("permission[%d]: expected %q, got %q", i, perm, result[i])
				}
			}
		})
	}
}

func TestHasObjectAccessFast(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	// Build cache for test user
	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	tests := []struct {
		name              string
		obj               map[string]any
		expectedAccess    bool
		expectedCheckSpec bool
	}{
		{
			name: "no tags - should allow",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-001",
				objects.FieldKeyKind: "backlog_item",
			},
			expectedAccess:    true,
			expectedCheckSpec: true,
		},
		{
			name: "matching access tag",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-002",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"access:team-alpha"},
			},
			expectedAccess:    true,
			expectedCheckSpec: true,
		},
		{
			name: "non-matching access tag",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-003",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"access:team-beta"},
			},
			expectedAccess:    false,
			expectedCheckSpec: false,
		},
		{
			name: "multiple tags - one matches",
			obj: map[string]any{
				objects.FieldKeyID:   "OBJ-004",
				objects.FieldKeyKind: "backlog_item",
				objects.FieldKeyTags: []any{"access:team-alpha", "access:team-beta"},
			},
			expectedAccess:    true,
			expectedCheckSpec: true,
		},
		{
			name: "no object ID - should check spec",
			obj: map[string]any{
				objects.FieldKeyKind: "backlog_item",
			},
			expectedAccess:    true,
			expectedCheckSpec: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasAccess, shouldCheckSpec := pc.HasObjectAccessFast(tt.obj, secCtx)
			if hasAccess != tt.expectedAccess {
				t.Errorf("hasAccess: expected %v, got %v", tt.expectedAccess, hasAccess)
			}
			if shouldCheckSpec != tt.expectedCheckSpec {
				t.Errorf("shouldCheckSpec: expected %v, got %v", tt.expectedCheckSpec, shouldCheckSpec)
			}
		})
	}
}

func TestHasObjectAccessFast_WildcardAccess(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	// Build cache for user with wildcard access
	secCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*", "access:*"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-001",
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha", "access:confidential"},
	}

	hasAccess, shouldCheckSpec := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess {
		t.Error("expected wildcard access to grant access")
	}
	if !shouldCheckSpec {
		t.Error("expected wildcard access to still check spec")
	}
}

func TestHasObjectAccessFast_CacheHit(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-001",
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha"},
	}

	// First call - should compute and cache
	hasAccess1, shouldCheckSpec1 := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess1 {
		t.Error("expected access to be granted")
	}

	// Second call - should hit cache
	hasAccess2, shouldCheckSpec2 := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess2 {
		t.Error("expected cached access to be granted")
	}
	if shouldCheckSpec1 != shouldCheckSpec2 {
		t.Error("cached result should match first result")
	}
}

func TestUserActivationDeactivation(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-001",
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha"},
	}

	// Initially active (default)
	hasAccess, _ := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess {
		t.Error("expected access for active user")
	}

	// Deactivate user
	err = pc.DeactivateUser(secCtx.AccountID)
	if err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	// Should deny access
	hasAccess, _ = pc.HasObjectAccessFast(obj, secCtx)
	if hasAccess {
		t.Error("expected access to be denied for deactivated user")
	}

	// Check IsUserActive
	if pc.IsUserActive(secCtx.AccountID) {
		t.Error("expected user to be inactive")
	}

	// Reactivate user
	err = pc.ActivateUser(secCtx.AccountID)
	if err != nil {
		t.Fatalf("failed to activate user: %v", err)
	}

	// Should grant access again
	hasAccess, _ = pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess {
		t.Error("expected access to be granted for reactivated user")
	}

	// Check IsUserActive
	if !pc.IsUserActive(secCtx.AccountID) {
		t.Error("expected user to be active")
	}
}

func TestInvalidateObjectAccess(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	objID := "OBJ-001"
	obj := map[string]any{
		objects.FieldKeyID:   objID,
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha"},
	}

	// First call - should compute and cache
	hasAccess1, _ := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess1 {
		t.Error("expected access to be granted")
	}

	// Invalidate cache
	pc.InvalidateObjectAccess(objID)

	// Second call - should recompute (cache miss)
	hasAccess2, _ := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess2 {
		t.Error("expected access to still be granted after invalidation")
	}

	// Verify cache was rebuilt
	// Third call should hit cache again
	hasAccess3, _ := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess3 {
		t.Error("expected cached access to be granted")
	}
}

func TestInvalidateUserCache(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-001",
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha"},
	}

	// First call - should compute and cache
	hasAccess1, _ := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess1 {
		t.Error("expected access to be granted")
	}

	// Invalidate user cache
	pc.InvalidateUserCache(secCtx.AccountID)

	// User permissions should be cleared
	_, ok := pc.GetUserPermissions(secCtx.AccountID)
	if ok {
		t.Error("expected user permissions to be cleared")
	}

	// Access should fail (no permissions cached)
	hasAccess2, _ := pc.HasObjectAccessFast(obj, secCtx)
	if hasAccess2 {
		t.Error("expected access to be denied after cache invalidation (no permissions)")
	}

	// Rebuild cache
	err = pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to rebuild permission cache: %v", err)
	}

	// Access should work again
	hasAccess3, _ := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess3 {
		t.Error("expected access to be granted after cache rebuild")
	}
}

func TestBuildPermissionCache_PrivilegeTagPerms(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	tests := []struct {
		name             string
		secCtx           *pkgctx.SecurityContext
		expectedWildcard bool
		expectedTags     []string
	}{
		{
			name:             "wildcard access",
			secCtx:           pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*", "access:*"}),
			expectedWildcard: true,
			expectedTags:     nil,
		},
		{
			name:             "specific access tags",
			secCtx:           pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:*", "access:team-alpha", "access:confidential"}),
			expectedWildcard: false,
			expectedTags:     []string{"team-alpha", "confidential"},
		},
		{
			name:             "no access permissions",
			secCtx:           pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:*"}),
			expectedWildcard: false,
			expectedTags:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pc.BuildPermissionCache(tt.secCtx)
			if err != nil {
				t.Fatalf("failed to build permission cache: %v", err)
			}

			perms, ok := pc.GetUserPermissions(tt.secCtx.AccountID)
			if !ok {
				t.Fatal("expected user permissions to be cached")
			}

			hasWildcard := perms.PrivilegeTagPerms["*"]
			if hasWildcard != tt.expectedWildcard {
				t.Errorf("wildcard access: expected %v, got %v", tt.expectedWildcard, hasWildcard)
			}

			if !tt.expectedWildcard {
				for _, tag := range tt.expectedTags {
					if !perms.PrivilegeTagPerms[tag] {
						t.Errorf("expected tag %q to be in privilege tag perms", tag)
					}
				}
			}
		})
	}
}

func TestBuildPermissionCache_InactiveUser(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})

	// Deactivate user before building cache
	err := pc.DeactivateUser(secCtx.AccountID)
	if err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	// Build cache for inactive user
	err = pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	// Check that user permissions are marked as inactive
	perms, ok := pc.GetUserPermissions(secCtx.AccountID)
	if !ok {
		t.Fatal("expected user permissions to be cached")
	}

	if perms.Active {
		t.Error("expected user permissions to be marked as inactive")
	}

	// Access should be denied
	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-001",
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha"},
	}

	hasAccess, _ := pc.HasObjectAccessFast(obj, secCtx)
	if hasAccess {
		t.Error("expected access to be denied for inactive user")
	}
}

func TestRefreshUserPermissions(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	// Get initial permissions
	perms1, _ := pc.GetUserPermissions(secCtx.AccountID)
	initialTime := perms1.LastUpdated

	// Update security context
	secCtx.Permissions = append(secCtx.Permissions, "access:team-beta")

	// Refresh permissions
	err = pc.RefreshUserPermissions(secCtx)
	if err != nil {
		t.Fatalf("failed to refresh permissions: %v", err)
	}

	// Get refreshed permissions
	perms2, ok := pc.GetUserPermissions(secCtx.AccountID)
	if !ok {
		t.Fatal("expected user permissions to be cached after refresh")
	}

	if perms2.LastUpdated.Before(initialTime) || perms2.LastUpdated.Equal(initialTime) {
		t.Error("expected LastUpdated to be after initial time")
	}

	// Verify new permission is in cache
	if !perms2.PrivilegeTagPerms["team-beta"] {
		t.Error("expected team-beta to be in privilege tag perms after refresh")
	}
}

func TestClearCache(t *testing.T) {
	t.Parallel()
	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user", []string{"developer"}, []string{"read:*", "access:team-alpha"})
	err := pc.BuildPermissionCache(secCtx)
	if err != nil {
		t.Fatalf("failed to build permission cache: %v", err)
	}

	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-001",
		objects.FieldKeyKind: "backlog_item",
		objects.FieldKeyTags: []any{"access:team-alpha"},
	}

	// Build some cache entries
	pc.HasObjectAccessFast(obj, secCtx)

	// Clear cache
	pc.ClearCache()

	// Verify cache is cleared
	_, ok := pc.GetUserPermissions(secCtx.AccountID)
	if ok {
		t.Error("expected user permissions to be cleared")
	}

	// Access should fail (no cache)
	hasAccess, _ := pc.HasObjectAccessFast(obj, secCtx)
	if hasAccess {
		t.Error("expected access to be denied after cache clear (no permissions)")
	}
}

func TestPermissionCache_LifetimeCounters(t *testing.T) {
	t.Parallel()
	var pcNil *PermissionCache
	hNil, mNil, iNil := pcNil.GetPermissionCacheStats()
	if hNil != 0 || mNil != 0 || iNil != 0 {
		t.Fatalf("expected nil stats (0, 0, 0), got (%d, %d, %d)", hNil, mNil, iNil)
	}

	mockLoader := newMockSpecLoader()
	pc := NewPermissionCache(mockLoader)

	secCtx := pkgctx.NewSecurityContext("account:test-user-counters", []string{"developer"}, []string{"read:*"})
	_ = pc.BuildPermissionCache(secCtx)

	obj := map[string]any{
		objects.FieldKeyID:   "OBJ-COUNTER-001",
		objects.FieldKeyKind: "backlog_item",
	}

	hBefore, mBefore, iBefore := pc.GetPermissionCacheStats()

	// First call -> cache miss
	_, _ = pc.HasObjectAccessFast(obj, secCtx)

	_, mAfter1, _ := pc.GetPermissionCacheStats()
	if mAfter1 != mBefore+1 {
		t.Errorf("expected cache misses to increase by 1, got before=%d after=%d", mBefore, mAfter1)
	}

	// Second call -> cache hit
	_, _ = pc.HasObjectAccessFast(obj, secCtx)

	hAfter2, _, _ := pc.GetPermissionCacheStats()
	if hAfter2 != hBefore+1 {
		t.Errorf("expected cache hits to increase by 1, got before=%d after=%d", hBefore, hAfter2)
	}

	// Invalidate object -> invalidations increases by 1
	pc.InvalidateObjectAccess("OBJ-COUNTER-001")
	_, _, iAfter3 := pc.GetPermissionCacheStats()
	if iAfter3 != iBefore+1 {
		t.Errorf("expected invalidations to increase by 1, got before=%d after=%d", iBefore, iAfter3)
	}

	// Invalidate user -> invalidations increases by 2 total
	pc.InvalidateUserCache("account:test-user-counters")
	_, _, iAfter4 := pc.GetPermissionCacheStats()
	if iAfter4 != iBefore+2 {
		t.Errorf("expected invalidations to increase by 2 total, got before=%d after=%d", iBefore, iAfter4)
	}
}
