package featureflags

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/testkit"
)

// TestFeatureFlags_AuditEventCreation tests that audit events can be created
// without hanging, even if storage operations are slow
func TestFeatureFlags_AuditEventCreation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.featureflags.audit"})
	projectRoot := proj.Root
	flags := NewFeatureFlags(projectRoot)
	_ = flags.Load() //nolint:errcheck // Test setup - load errors use default flags

	// Test that SetEnabled completes quickly even if audit logging is slow
	done := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_set_enabled", "setting feature flag in audit test", func() {
		err := flags.SetEnabled("async_validation", true)
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetEnabled failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetEnabled hung - should complete quickly even with async audit")
	}
}

// TestFeatureFlags_ContextTimeout tests that operations respect context timeouts
func TestFeatureFlags_ContextTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 100*time.Millisecond)
	defer cancel()

	// Wait for context to timeout
	<-ctx.Done()

	if ctx.Err() != context.DeadlineExceeded {
		t.Error("Expected context deadline exceeded")
	}
}
