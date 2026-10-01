package cli

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mockOverrideStorage struct {
	createdObj map[string]any
}

func (m *mockOverrideStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.createdObj = obj
	return nil
}

func TestEnforceOverrideFriction(t *testing.T) {
	cmd := NewCommandBuilder("test").Build()
	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{AccountID: "ACC-SYSTEM"}
	store := &mockOverrideStorage{}

	reason := "This is a long descriptive reason containing at least five words and thirty characters to bypass the lifecycle rules."

	t.Run("short_justification_fails", func(t *testing.T) {
		err := EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", "short reason")
		if err == nil {
			t.Error("expected error for short justification, got nil")
		}
	})

	t.Run("non_tty_long_reason_denied", func(t *testing.T) {
		// Unit tests run without a TTY; long reason alone must not succeed (agent/pipe hole closed).
		t.Setenv(zqkenv.EnvCI().Name(), "")
		store.createdObj = nil
		err := EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", reason)
		if err == nil {
			t.Fatal("expected non-TTY deny, got nil")
		}
		if !strings.Contains(err.Error(), "interactive TTY") {
			t.Fatalf("error should mention TTY: %v", err)
		}
		if strings.Contains(err.Error(), "POL-") {
			t.Fatalf("product error must not cite studio policy ids: %v", err)
		}
		if !strings.Contains(err.Error(), "object promote") {
			t.Fatalf("error should point at promote/demote: %v", err)
		}
		if store.createdObj != nil {
			t.Fatal("must not record technical debt when non-TTY deny fires")
		}
	})

	t.Run("ci_blocked", func(t *testing.T) {
		t.Setenv(zqkenv.EnvCI().Name(), "true")
		err := EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", reason)
		if err == nil {
			t.Error("expected CI block error, got nil")
		}
	})

	t.Run("tray_indirect_execution_blocked", func(t *testing.T) {
		t.Setenv(zqkenv.ExecSource().Name(), "tray")
		err := EnforceOverrideFriction(cmd, ctx, secCtx, store, "TEST-1", "backlog_item", reason)
		if err == nil {
			t.Fatal("expected tray indirect execution block error, got nil")
		}
		if !strings.Contains(err.Error(), "completely blocked when invoked via indirect runners (Tray)") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("pulse_meaningful_activity", func(t *testing.T) {
		stop := pulseMeaningfulActivityWhileWaiting()
		if stop == nil {
			t.Fatal("expected non-nil stop func")
		}
		stop()
	})
}
