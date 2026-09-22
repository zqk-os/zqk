package handslapper

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestExtraCoverage_Electrocute(t *testing.T) {
	t.Parallel()

	// 1. Nil context defaults to UNKNOWN_ENTITY
	errNilCtx := Electrocute(nil, "TEST_VIOLATION", "testing nil context")
	if errNilCtx == nil || !strings.Contains(errNilCtx.Error(), "UNKNOWN_ENTITY") {
		t.Errorf("expected UNKNOWN_ENTITY in error message, got: %v", errNilCtx)
	}

	// 2. Context with SecurityContext but no roles
	ctxNoRoles := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "agent-alpha",
	})
	errNoRoles := Electrocute(ctxNoRoles, "TEST_VIOLATION", "testing no roles")
	if errNoRoles == nil || !strings.Contains(errNoRoles.Error(), "agent-alpha") || strings.Contains(errNoRoles.Error(), "Role:") {
		t.Errorf("expected agent-alpha without role, got: %v", errNoRoles)
	}

	// 3. Context with SecurityContext with roles
	ctxWithRoles := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "agent-beta",
		Roles:     []string{"orchestrator", "worker"},
	})
	errWithRoles := Electrocute(ctxWithRoles, "TEST_VIOLATION", "testing roles")
	if errWithRoles == nil || !strings.Contains(errWithRoles.Error(), "agent-beta (Role: orchestrator)") {
		t.Errorf("expected role formatting in error message, got: %v", errWithRoles)
	}
}

func TestExtraCoverage_MonitorCommand_NonStrictMode(t *testing.T) {
	t.Parallel()

	svc := NewService(false) // non-strict mode: logs drift but returns nil error

	// Empty command
	if err := svc.MonitorCommand(context.Background(), "agent-1", "   "); err != nil {
		t.Errorf("expected nil for whitespace command, got: %v", err)
	}

	// Namespace violation with .zqk in non-strict mode
	nsCmd := "touch " + paths.ProjectDataDir + "/test.txt"
	if err := svc.MonitorCommand(context.Background(), "agent-1", nsCmd); err != nil {
		t.Errorf("expected nil in non-strict mode for namespace violation, got: %v", err)
	}

	// Tool bypass with grep in non-strict mode
	toolCmd := "grep -r 'pattern' ."
	if err := svc.MonitorCommand(context.Background(), "agent-1", toolCmd); err != nil {
		t.Errorf("expected nil in non-strict mode for tool bypass, got: %v", err)
	}
}

func TestExtraCoverage_MonitorCommand_StrictMode_NamespaceViolation(t *testing.T) {
	t.Parallel()

	svc := NewService(true) // strict mode: returns Electrocute error

	nsCmd := "touch " + paths.ProjectDataDir + "/state.json"
	err := svc.MonitorCommand(context.Background(), "agent-strict", nsCmd)
	if err == nil || !strings.Contains(err.Error(), "NAMESPACE_VIOLATION") {
		t.Errorf("expected NAMESPACE_VIOLATION error, got: %v", err)
	}
}
