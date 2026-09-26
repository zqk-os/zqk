package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBreakGlassElevation_TimeBoundedWindow(t *testing.T) {
	fos, _, secCtx := setupTestStorage(t)
	objID := "BLI-BREAKGLASS-TEST-001"
	createTestBacklogItem(t, fos, secCtx, objID)

	t.Run("expired_window_blocks_provenance_and_invalid_transition", func(t *testing.T) {
		past := time.Now().Add(-10 * time.Minute)
		expiredCtx := pkgctx.WithLifecycleBreakGlassExpiresAt(context.Background(), "expired emergency window", past)
		expiredCtx = pkgctx.WithAllowCoreObjectDelete(expiredCtx)

		// 1. Attempt provenance mutation with expired break-glass window
		err := fos.Update(expiredCtx, secCtx, objID, map[string]any{
			"created_at": "2020-01-01T00:00:00Z",
		})
		if err == nil {
			t.Fatal("expected expired break glass to block created_at mutation, got nil")
		}
		if !strings.Contains(err.Error(), "immutable") && !strings.Contains(err.Error(), "system provenance") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// 2. Attempt invalid lifecycle status hop (exploring -> complete) with expired break-glass window
		err = fos.Update(expiredCtx, secCtx, objID, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		})
		if err == nil {
			t.Fatal("expected expired break glass to block invalid lifecycle status transition, got nil")
		}
		if !strings.Contains(err.Error(), "invalid status transition") && !strings.Contains(err.Error(), "validation failed") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("active_window_permits_emergency_repair", func(t *testing.T) {
		activeCtx := pkgctx.WithLifecycleBreakGlassWindow(context.Background(), "active emergency repair window", 15*time.Minute)
		activeCtx = pkgctx.WithAllowCoreObjectDelete(activeCtx)

		// Permitted in active emergency window:
		err := fos.Update(activeCtx, secCtx, objID, map[string]any{
			"created_at":           "2021-05-01T12:00:00Z",
			objects.FieldKeyStatus: objects.ObjectStatusPlanned,
		})
		if err != nil {
			t.Fatalf("expected active break glass window to permit emergency update, got: %v", err)
		}

		updated, err := fos.Read(context.Background(), secCtx, objID)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if updated["created_at"] != "2021-05-01T12:00:00Z" {
			t.Fatalf("expected updated created_at, got %v", updated["created_at"])
		}
		if updated[objects.FieldKeyStatus] != objects.ObjectStatusPlanned {
			t.Fatalf("expected updated status planned, got %v", updated[objects.FieldKeyStatus])
		}
	})
}
