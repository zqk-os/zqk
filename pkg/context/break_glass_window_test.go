package context_test

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestLifecycleBreakGlassWindow(t *testing.T) {
	t.Parallel()

	t.Run("active_window_is_armed", func(t *testing.T) {
		ctx := pkgctx.WithLifecycleBreakGlassWindow(context.Background(), "emergency repair", 5*time.Minute)
		if !pkgctx.IsLifecycleBreakGlass(ctx) {
			t.Fatal("expected break glass to be armed within active window")
		}
		if !pkgctx.HasLifecycleBreakGlass(ctx) {
			t.Fatal("expected HasLifecycleBreakGlass to be true within active window")
		}
		reason := pkgctx.GetLifecycleBreakGlassReason(ctx)
		if reason != "emergency repair" {
			t.Fatalf("expected reason 'emergency repair', got %q", reason)
		}
		expiry, ok := pkgctx.GetLifecycleBreakGlassExpiry(ctx)
		if !ok || expiry.IsZero() {
			t.Fatal("expected expiry to be set")
		}
		if time.Until(expiry) <= 0 {
			t.Fatal("expected expiry to be in the future")
		}
	})

	t.Run("expired_window_disarms_break_glass", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Minute)
		ctx := pkgctx.WithLifecycleBreakGlassExpiresAt(context.Background(), "expired repair", past)
		if pkgctx.IsLifecycleBreakGlass(ctx) {
			t.Fatal("expected expired break glass to be disarmed")
		}
		if pkgctx.HasLifecycleBreakGlass(ctx) {
			t.Fatal("expected HasLifecycleBreakGlass to return false when expired")
		}
	})

	t.Run("unbounded_break_glass_without_expiry_remains_armed", func(t *testing.T) {
		ctx := pkgctx.WithLifecycleBreakGlass(context.Background(), "unbounded repair")
		if !pkgctx.IsLifecycleBreakGlass(ctx) {
			t.Fatal("expected unbounded break glass to be armed")
		}
		_, ok := pkgctx.GetLifecycleBreakGlassExpiry(ctx)
		if ok {
			t.Fatal("expected no expiry set for unbounded break glass")
		}
	})
}
