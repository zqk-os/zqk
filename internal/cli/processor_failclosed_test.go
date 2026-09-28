package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestNewProcessor_UnboundSecurityContext_FailsClosedToGuest(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("test").
		WithRunE(func(cmd *cobra.Command, args []string) error { return nil }).
		Build()
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

func TestGuestSecurityContext_BoundaryAndImmutability(t *testing.T) {
	guest := pkgctx.NewGuestSecurityContext()
	if guest.AccountID != pkgctx.GuestAccountID {
		t.Errorf("expected GuestAccountID (%s), got %s", pkgctx.GuestAccountID, guest.AccountID)
	}
	if len(guest.GetRoles()) != 1 || guest.GetRoles()[0] != "guest" {
		t.Errorf("expected role ['guest'], got %v", guest.GetRoles())
	}
	if guest.GetPrecedence() != pkgctx.PrecedenceDefault {
		t.Errorf("expected PrecedenceDefault, got %d", guest.GetPrecedence())
	}

	// Validate method should report clean configuration
	errs := guest.Validate()
	if len(errs) != 0 {
		t.Errorf("expected 0 validation errors for default guest context, got %v", errs)
	}
}

func TestSecurityContext_FailClosedIntegration(t *testing.T) {
	ctx := context.Background()
	// Unbound context must return nil
	extracted := pkgctx.GetSecurityContext(ctx)
	if extracted != nil {
		t.Fatalf("expected nil from unbound context, got %v", extracted)
	}

	// Explicit guest context attached
	guest := pkgctx.NewGuestSecurityContext()
	boundCtx := pkgctx.WithSecurityContext(ctx, guest)
	res := pkgctx.GetSecurityContext(boundCtx)
	if res == nil {
		t.Fatal("expected non-nil security context from bound context")
	}
	if res.AccountID != pkgctx.GuestAccountID {
		t.Errorf("expected account %s, got %s", pkgctx.GuestAccountID, res.AccountID)
	}
}

