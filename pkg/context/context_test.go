package context

import (
	"slices"
	"testing"
)

func TestNewSecurityContext(t *testing.T) {
	t.Parallel()
	accountID := "account:testuser"
	roles := []string{"admin", "developer"}
	permissions := []string{"read:*", "write:backlog_item"}

	secCtx := NewSecurityContext(accountID, roles, permissions)

	if secCtx == nil {
		t.Fatal("NewSecurityContext returned nil")
	}

	if secCtx.AccountID != accountID {
		t.Errorf("expected AccountID %s, got %s", accountID, secCtx.AccountID)
	}

	if len(secCtx.Roles) != len(roles) {
		t.Errorf("expected %d roles, got %d", len(roles), len(secCtx.Roles))
	}

	for i, role := range roles {
		if secCtx.Roles[i] != role {
			t.Errorf("expected role[%d] %s, got %s", i, role, secCtx.Roles[i])
		}
	}

	if len(secCtx.Permissions) != len(permissions) {
		t.Errorf("expected %d permissions, got %d", len(permissions), len(secCtx.Permissions))
	}

	for i, perm := range permissions {
		if secCtx.Permissions[i] != perm {
			t.Errorf("expected permission[%d] %s, got %s", i, perm, secCtx.Permissions[i])
		}
	}
}

func TestActorIDForAttribution(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: SystemAccountID},
		{in: "system", want: SystemAccountID},
		{in: "account:system", want: SystemAccountID},
		{in: "  system  ", want: SystemAccountID},
		{in: SystemAccountID, want: SystemAccountID},
		{in: FounderAccountID, want: FounderAccountID},
	}
	for _, tt := range tests {
		if got := ActorIDForAttribution(tt.in); got != tt.want {
			t.Errorf("ActorIDForAttribution(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if IsSystemAccount("") {
		t.Error("empty account must not count as system for permission checks")
	}
	if !IsSystemAccount("system") || !IsSystemAccount("account:system") || !IsSystemAccount(SystemAccountID) {
		t.Error("retired system aliases and SystemAccountID must count as the system actor")
	}
}

func TestNewSystemSecurityContext(t *testing.T) {
	t.Parallel()
	secCtx := NewSystemSecurityContext()

	if secCtx == nil {
		t.Fatal("NewSystemSecurityContext returned nil")
	}

	if secCtx.AccountID != SystemAccountID {
		t.Errorf("expected AccountID '%s', got %s", SystemAccountID, secCtx.AccountID)
	}

	if len(secCtx.Roles) != 1 || secCtx.Roles[0] != "admin" {
		t.Errorf("expected roles ['admin'], got %v", secCtx.Roles)
	}

	expectedPerms := []string{"read:*", "write:*", "delete:*"}
	if len(secCtx.Permissions) != len(expectedPerms) {
		t.Errorf("expected %d permissions, got %d", len(expectedPerms), len(secCtx.Permissions))
	}

	for _, expectedPerm := range expectedPerms {
		if !slices.Contains(secCtx.Permissions, expectedPerm) {
			t.Errorf("expected permission %s not found in %v", expectedPerm, secCtx.Permissions)
		}
	}
}

func TestNewStorageContext(t *testing.T) {
	t.Parallel()
	storageCtx := NewStorageContext()

	if storageCtx == nil {
		t.Fatal("NewStorageContext returned nil")
	}

	if storageCtx.MaxPageSize != 0 {
		t.Errorf("expected MaxPageSize 0, got %d", storageCtx.MaxPageSize)
	}

	if storageCtx.DefaultPageSize != 0 {
		t.Errorf("expected DefaultPageSize 0, got %d", storageCtx.DefaultPageSize)
	}

	if storageCtx.EnableGrouping != false {
		t.Errorf("expected EnableGrouping false, got %v", storageCtx.EnableGrouping)
	}

	if storageCtx.MaxGroupSize != 0 {
		t.Errorf("expected MaxGroupSize 0, got %d", storageCtx.MaxGroupSize)
	}
}

func TestNewPaginationStorageContext(t *testing.T) {
	t.Parallel()
	maxPageSize := 50
	defaultPageSize := 10

	storageCtx := NewPaginationStorageContext(maxPageSize, defaultPageSize)

	if storageCtx == nil {
		t.Fatal("NewPaginationStorageContext returned nil")
	}

	if storageCtx.MaxPageSize != maxPageSize {
		t.Errorf("expected MaxPageSize %d, got %d", maxPageSize, storageCtx.MaxPageSize)
	}

	if storageCtx.DefaultPageSize != defaultPageSize {
		t.Errorf("expected DefaultPageSize %d, got %d", defaultPageSize, storageCtx.DefaultPageSize)
	}

	if storageCtx.EnableGrouping != false {
		t.Errorf("expected EnableGrouping false, got %v", storageCtx.EnableGrouping)
	}

	if storageCtx.MaxGroupSize != 0 {
		t.Errorf("expected MaxGroupSize 0, got %d", storageCtx.MaxGroupSize)
	}
}

func TestNewGroupingStorageContext(t *testing.T) {
	t.Parallel()
	maxGroupSize := 20

	storageCtx := NewGroupingStorageContext(maxGroupSize)

	if storageCtx == nil {
		t.Fatal("NewGroupingStorageContext returned nil")
	}

	if storageCtx.MaxPageSize != 0 {
		t.Errorf("expected MaxPageSize 0, got %d", storageCtx.MaxPageSize)
	}

	if storageCtx.DefaultPageSize != 0 {
		t.Errorf("expected DefaultPageSize 0, got %d", storageCtx.DefaultPageSize)
	}

	if storageCtx.EnableGrouping != true {
		t.Errorf("expected EnableGrouping true, got %v", storageCtx.EnableGrouping)
	}

	if storageCtx.MaxGroupSize != maxGroupSize {
		t.Errorf("expected MaxGroupSize %d, got %d", maxGroupSize, storageCtx.MaxGroupSize)
	}
}

func TestNewGroupingStorageContext_ZeroMaxGroupSize(t *testing.T) {
	t.Parallel()
	storageCtx := NewGroupingStorageContext(0)

	if storageCtx == nil {
		t.Fatal("NewGroupingStorageContext returned nil")
	}

	if storageCtx.EnableGrouping != true {
		t.Errorf("expected EnableGrouping true, got %v", storageCtx.EnableGrouping)
	}

	if storageCtx.MaxGroupSize != 0 {
		t.Errorf("expected MaxGroupSize 0, got %d", storageCtx.MaxGroupSize)
	}
}
