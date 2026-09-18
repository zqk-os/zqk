package entitlements

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
)

func TestCommunityChecker(t *testing.T) {
	checker := &CommunityChecker{}
	ctx := context.Background()

	// Limit <= 3 should pass
	if err := checker.CheckEvolveLimit(ctx, 3); err != nil {
		t.Errorf("expected no error for limit 3, got: %v", err)
	}

	// Limit > 3 should fail
	err := checker.CheckEvolveLimit(ctx, 4)
	if err == nil {
		t.Error("expected error for limit 4, got nil")
	} else if !strings.Contains(err.Error(), "community tier is limited to 3") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEnterpriseChecker(t *testing.T) {
	checker := &EnterpriseChecker{}
	ctx := context.Background()

	// Any limit should pass
	if err := checker.CheckEvolveLimit(ctx, 100); err != nil {
		t.Errorf("expected no error for limit 100, got: %v", err)
	}
}

type mockChecker struct {
	called bool
}

func (m *mockChecker) CheckEvolveLimit(ctx context.Context, limit int) error {
	m.called = true
	return nil
}

func (m *mockChecker) CheckMeshEntitlement(ctx context.Context) error {
	return nil
}

func (m *mockChecker) CheckBundle(ctx context.Context, bundle string) error {
	return nil
}

func TestRegisterChecker(t *testing.T) {
	// Reset global state
	defer func() {
		globalChecker = &CommunityChecker{}
	}()

	mock := &mockChecker{}
	RegisterChecker(mock)

	ctx := context.Background()
	_ = CheckEvolveEntitlement(ctx, 10)

	if !mock.called {
		t.Error("expected mock checker to be called")
	}
}

func TestCheckEvolveEntitlement_AutoUpgrade(t *testing.T) {
	// Reset global state
	defer func() {
		globalChecker = &CommunityChecker{}
		_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	}()

	ctx := context.Background()

	// Without key, should fail limit > 3
	_ = os.Unsetenv(brand.DefaultEnvPrefix + "_LICENSE_KEY")
	globalChecker = &CommunityChecker{}
	if err := CheckEvolveEntitlement(ctx, 4); err == nil {
		t.Error("expected error without license key for limit 4")
	}

	// With key, should auto-upgrade and pass
	_ = os.Setenv(brand.DefaultEnvPrefix+"_LICENSE_KEY", "mock-enterprise-key")
	LoadLicense()
	if err := CheckEvolveEntitlement(ctx, 4); err != nil {
		t.Errorf("expected no error with license key, got: %v", err)
	}
}
