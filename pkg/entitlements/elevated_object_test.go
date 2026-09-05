package entitlements

import (
	"context"
	"strings"
	"testing"
)

func TestCommunityChecker_ElevatedObjectBundle(t *testing.T) {
	t.Parallel()
	checker := &CommunityChecker{}
	err := checker.CheckBundle(context.Background(), BundleElevatedObject)
	if err == nil {
		t.Fatal("expected community to refuse elevated_object bundle")
	}
	if !strings.Contains(err.Error(), "Enterprise") && !strings.Contains(err.Error(), "subscription") {
		t.Fatalf("upgrade copy missing: %v", err)
	}
}

func TestEnterpriseChecker_ElevatedObjectBundle(t *testing.T) {
	t.Parallel()
	checker := &EnterpriseChecker{}
	if err := checker.CheckBundle(context.Background(), BundleElevatedObject); err != nil {
		t.Fatalf("enterprise should allow elevated_object: %v", err)
	}
}
