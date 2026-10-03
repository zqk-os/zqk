package object

import (
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestAuthorizeDraftSweepApply_rbacPermissionOK(t *testing.T) {
	sec := pkgctx.NewSecurityContext("ACC-tpm-test", []string{"owner"}, []string{pkgctx.PermissionDeleteObjectDraftPlane})
	if err := authorizeDraftSweepApply(sec); err != nil {
		t.Fatalf("rbac grant: %v", err)
	}
}

func TestAuthorizeDraftSweepApply_adminOK(t *testing.T) {
	sec := pkgctx.NewSecurityContext("ACC-admin-test", []string{"admin"}, []string{"read:*", "write:*", "delete:*"})
	if err := authorizeDraftSweepApply(sec); err != nil {
		t.Fatalf("admin: %v", err)
	}
}

func TestAuthorizeDraftSweepApply_doerDenied(t *testing.T) {
	sec := pkgctx.NewSecurityContext("ACC-doer", []string{"coder_agent"}, []string{"read:*", "write:code", "write:agent_task"})
	err := authorizeDraftSweepApply(sec)
	if err == nil || !strings.Contains(err.Error(), pkgctx.PermissionDeleteObjectDraftPlane) {
		t.Fatalf("want RBAC deny, got %v", err)
	}
}

func TestAuthorizeDraftSweepApply_systemFallbackDenied(t *testing.T) {
	err := authorizeDraftSweepApply(pkgctx.NewSystemSecurityContext())
	if err == nil || !strings.Contains(err.Error(), "unbound system") {
		t.Fatalf("want unbound system reject, got %v", err)
	}
}

func TestAuthorizeDraftSweepApply_nilDenied(t *testing.T) {
	err := authorizeDraftSweepApply(nil)
	if err == nil || !strings.Contains(err.Error(), "authenticated ACC") {
		t.Fatalf("want nil deny, got %v", err)
	}
}

func TestAuthorizeDraftSweepApply_forceBypassesLocalWorkspace(t *testing.T) {
	if err := authorizeDraftSweepApply(nil, true); err != nil {
		t.Fatalf("expected nil error with force=true for nil secCtx, got %v", err)
	}
	if err := authorizeDraftSweepApply(pkgctx.NewSystemSecurityContext(), true); err != nil {
		t.Fatalf("expected nil error with force=true for system secCtx, got %v", err)
	}
	secDoer := pkgctx.NewSecurityContext("ACC-doer", []string{"coder_agent"}, []string{"read:*"})
	if err := authorizeDraftSweepApply(secDoer, true); err != nil {
		t.Fatalf("expected nil error with force=true for unprivileged doer, got %v", err)
	}
}
