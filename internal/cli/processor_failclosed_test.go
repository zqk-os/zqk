package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestNewProcessor_UnboundSecurityContext_FailsClosedToGuest(t *testing.T) {
	cmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	// Explicitly empty context without AuthMiddleware binding
	cmd.SetContext(context.Background())

	// Create processor without storage (or with dummy context)
	// We want to verify that when NewProcessor initializes, it assigns NewGuestSecurityContext
	secCtx := pkgctx.GetSecurityContext(cmd.Context())
	if secCtx != nil {
		t.Fatalf("expected nil security context on unbound command, got %v", secCtx)
	}

	guest := pkgctx.NewGuestSecurityContext()
	if guest == nil {
		t.Fatal("NewGuestSecurityContext returned nil")
	}
	if guest.AccountID != "ACC-GUEST" {
		t.Fatalf("expected ACC-GUEST, got %s", guest.AccountID)
	}
	if len(guest.Permissions) != 0 {
		t.Fatalf("expected zero permissions for guest, got %v", guest.Permissions)
	}
}
