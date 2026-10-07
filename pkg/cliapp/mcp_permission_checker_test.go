package cli

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

type mockUserChecker struct {
	active bool
}

func (m *mockUserChecker) IsUserActive(accountID string) bool {
	return m.active
}

func TestMCPPermissionCheckerAdapter_Permissions(t *testing.T) {
	ctx := context.Background()

	// 1. Nil security context
	adapterNil := NewMCPPermissionCheckerAdapter(nil, nil, nil)
	allowed, reason := adapterNil.CheckPermission(ctx, "read", "goal")
	if allowed || reason != "no security context" {
		t.Errorf("expected false, 'no security context', got %v, %s", allowed, reason)
	}

	// 2. Inactive user
	inactiveUserChecker := &mockUserChecker{active: false}
	secCtx := &pkgctx.SecurityContext{
		AccountID: "acc-123",
	}
	adapterInactive := NewMCPPermissionCheckerAdapter(inactiveUserChecker, nil, secCtx)
	allowed, reason = adapterInactive.CheckPermission(ctx, "read", "goal")
	if allowed || reason != "user is inactive" {
		t.Errorf("expected false, 'user is inactive', got %v, %s", allowed, reason)
	}

	// 3. Admin role
	adminSecCtx := &pkgctx.SecurityContext{
		AccountID: "admin-1",
		Roles:     []string{mcpPermissionRoleAdmin},
	}
	adapterAdmin := NewMCPPermissionCheckerAdapter(nil, nil, adminSecCtx)
	allowed, _ = adapterAdmin.CheckPermission(ctx, "write", "anything")
	if !allowed {
		t.Errorf("expected admin to be allowed all permissions")
	}

	// 4. Exact permission match
	permSecCtx := &pkgctx.SecurityContext{
		AccountID:   "user-1",
		Permissions: []string{"read:goal", "execute:*"},
	}
	adapterPerm := NewMCPPermissionCheckerAdapter(nil, nil, permSecCtx)
	allowed, _ = adapterPerm.CheckPermission(ctx, "read", "goal")
	if !allowed {
		t.Errorf("expected read:goal to be allowed")
	}

	// Wildcard permission match
	allowed, _ = adapterPerm.CheckPermission(ctx, "execute", "scenario")
	if !allowed {
		t.Errorf("expected execute:* wildcard to match execute:scenario")
	}

	// Missing permission
	allowed, reason = adapterPerm.CheckPermission(ctx, "write", "goal")
	if allowed || reason != "missing permission: write:goal" {
		t.Errorf("expected missing permission error, got %v, %s", allowed, reason)
	}
}

func TestMCPPermissionCheckerAdapter_FormatPermissions(t *testing.T) {
	ctx := context.Background()

	// Default formats (table, json, yaml) allowed without streaming permissions
	secCtx := &pkgctx.SecurityContext{AccountID: "user-1"}
	adapter := NewMCPPermissionCheckerAdapter(nil, nil, secCtx)

	allowed, _ := adapter.CheckFormatPermission(ctx, FormatTable)
	if !allowed {
		t.Errorf("expected FormatTable to be allowed by default")
	}
	allowed, _ = adapter.CheckFormatPermission(ctx, FormatJSON)
	if !allowed {
		t.Errorf("expected FormatJSON to be allowed by default")
	}

	// Streaming format without stream permission denied
	allowed, reason := adapter.CheckFormatPermission(ctx, FormatJSONRPC)
	if allowed || reason == "" {
		t.Errorf("expected FormatJSONRPC to be denied without stream:* or read:*")
	}

	// Streaming format with read:* allowed
	secCtxRead := &pkgctx.SecurityContext{
		AccountID:   "user-2",
		Permissions: []string{"read:*"},
	}
	adapterRead := NewMCPPermissionCheckerAdapter(nil, nil, secCtxRead)
	allowed, _ = adapterRead.CheckFormatPermission(ctx, FormatJSONRPC)
	if !allowed {
		t.Errorf("expected FormatJSONRPC to be allowed with read:*")
	}

	// Streaming format with stream:* allowed
	secCtxStream := &pkgctx.SecurityContext{
		AccountID:   "user-3",
		Permissions: []string{"stream:*"},
	}
	adapterStream := NewMCPPermissionCheckerAdapter(nil, nil, secCtxStream)
	allowed, _ = adapterStream.CheckFormatPermission(ctx, FormatStream)
	if !allowed {
		t.Errorf("expected FormatStream to be allowed with stream:*")
	}
}

func TestMCPPermissionCheckerAdapter_DataAccess(t *testing.T) {
	ctx := context.Background()

	// Without spec access control, access is allowed by default
	secCtx := &pkgctx.SecurityContext{AccountID: "user-1"}
	adapter := NewMCPPermissionCheckerAdapter(nil, nil, secCtx)

	allowed, _ := adapter.CheckDataAccess(ctx, map[string]any{"id": "GOAL-1"})
	if !allowed {
		t.Errorf("expected data access allowed when spec access control is nil")
	}
}

func TestMCPPermissionCheckerAdapter_SetGetSecurityContextAndFastPath(t *testing.T) {
	adapter := NewMCPPermissionCheckerAdapter(nil, nil, nil)
	if s := adapter.GetSecurityContext(); s != nil {
		t.Errorf("expected nil initial security context")
	}

	newSec := &pkgctx.SecurityContext{
		AccountID: "acc-updated-456",
		Roles:     []string{mcpPermissionRoleAdmin},
	}
	adapter.SetSecurityContext(newSec)
	got := adapter.GetSecurityContext()
	if got == nil || got.AccountID != "acc-updated-456" {
		t.Errorf("expected updated security context with acc-updated-456, got: %+v", got)
	}

	// Data access with updated admin context
	allowed, _ := adapter.CheckDataAccess(context.Background(), map[string]any{"id": "GOAL-1", "kind": "goal"})
	if !allowed {
		t.Errorf("expected admin to have data access")
	}
}

type mockSpecAccessControl struct {
	hasAccess bool
}

func (m *mockSpecAccessControl) HasObjectAccess(obj map[string]any, secCtx *pkgctx.SecurityContext, op string) bool {
	return m.hasAccess
}

type mockPermissionCache struct {
	hasAccess       bool
	shouldCheckSpec bool
}

func (m *mockPermissionCache) HasObjectAccessFast(obj map[string]any, secCtx *pkgctx.SecurityContext) (bool, bool) {
	return m.hasAccess, m.shouldCheckSpec
}

func TestMCPPermissionCheckerAdapter_SpecAndCacheAccess(t *testing.T) {
	ctx := context.Background()
	userSecCtx := &pkgctx.SecurityContext{AccountID: "user-regular"}

	// 1. Spec access denied
	mockSpec := &mockSpecAccessControl{hasAccess: false}
	adapter := NewMCPPermissionCheckerAdapter(nil, mockSpec, userSecCtx)
	allowed, _ := adapter.CheckDataAccess(ctx, map[string]any{"id": "GOAL-1", "kind": "goal"})
	if allowed {
		t.Errorf("expected data access to be denied by spec access control")
	}

	// 2. Spec access allowed
	mockSpec.hasAccess = true
	allowed, _ = adapter.CheckDataAccess(ctx, map[string]any{"id": "GOAL-1", "kind": "goal"})
	if !allowed {
		t.Errorf("expected data access to be allowed by spec access control")
	}

	// 3. Slice of objects with spec access denied on an item
	mockSpec.hasAccess = false
	sliceData := []any{map[string]any{"id": "GOAL-2", "kind": "goal"}}
	allowed, _ = adapter.CheckDataAccess(ctx, sliceData)
	if allowed {
		t.Errorf("expected slice data access to be denied")
	}

	// 4. Cache fast path denied
	mockCache := &mockPermissionCache{hasAccess: false, shouldCheckSpec: false}
	adapter.permissionCacheInterface = mockCache
	allowed, _ = adapter.CheckDataAccess(ctx, map[string]any{"id": "GOAL-3", "kind": "goal"})
	if allowed {
		t.Errorf("expected cache fast path to deny access")
	}

	// 5. Cache fast path allowed without spec check
	mockCache.hasAccess = true
	mockCache.shouldCheckSpec = false
	allowed, _ = adapter.CheckDataAccess(ctx, map[string]any{"id": "GOAL-3", "kind": "goal"})
	if !allowed {
		t.Errorf("expected cache fast path to allow access without spec check")
	}
}
