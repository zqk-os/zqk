package object

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/entitlements"
	"github.com/spf13/cobra"
)

func testElevatedFlagCmd() *cobra.Command {
	cmd := new(cobra.Command)
	cmd.Use = "list"
	cmd.Flags().Bool(FlagElevatedInternal, false, "")
	return cmd
}

func TestRequireElevatedInternal_FlagUnset(t *testing.T) {
	t.Parallel()
	cmd := testElevatedFlagCmd()
	if err := RequireElevatedInternal(cmd); err != nil {
		t.Fatalf("unset flag should pass: %v", err)
	}
}

func TestRequireElevatedInternal_CommunityRefuses(t *testing.T) {
	prev := entitlements.CurrentCheckerForTest()
	defer entitlements.RegisterChecker(prev)
	entitlements.RegisterChecker(&entitlements.CommunityChecker{})
	brand.SetExecutableName("zqk")

	cmd := testElevatedFlagCmd()
	_ = cmd.Flags().Set(FlagElevatedInternal, "true")
	err := RequireElevatedInternal(cmd)
	if err == nil {
		t.Fatal("expected community refuse")
	}
	if !strings.Contains(err.Error(), "elevated") && !strings.Contains(err.Error(), "Enterprise") && !strings.Contains(err.Error(), "subscription") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireElevatedInternal_EnterpriseAllows(t *testing.T) {
	prev := entitlements.CurrentCheckerForTest()
	defer entitlements.RegisterChecker(prev)
	entitlements.RegisterChecker(&entitlements.EnterpriseChecker{})
	brand.SetExecutableName("zqk")

	cmd := testElevatedFlagCmd()
	_ = cmd.Flags().Set(FlagElevatedInternal, "true")
	if err := RequireElevatedInternal(cmd); err != nil {
		t.Fatalf("enterprise should allow: %v", err)
	}
}

func TestRequireElevatedInternal_AdminBinaryAllows(t *testing.T) {
	prev := entitlements.CurrentCheckerForTest()
	defer entitlements.RegisterChecker(prev)
	entitlements.RegisterChecker(&entitlements.CommunityChecker{})
	brand.SetExecutableName("zqk-admin")
	defer brand.SetExecutableName("zqk")

	cmd := testElevatedFlagCmd()
	_ = cmd.Flags().Set(FlagElevatedInternal, "true")
	if err := RequireElevatedInternal(cmd); err != nil {
		t.Fatalf("zqk-admin should allow without license: %v", err)
	}
}

func TestShouldSkipInternalKind(t *testing.T) {
	t.Parallel()
	if !ShouldSkipInternalKind("audit_event", false) {
		t.Fatal("non-elevated should skip audit_event")
	}
	if ShouldSkipInternalKind("audit_event", true) {
		t.Fatal("elevated should not skip audit_event")
	}
	if ShouldSkipInternalKind("backlog_item", false) {
		t.Fatal("public kind should not skip")
	}
}
