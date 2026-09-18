package object

import (
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// ===========================================================================
// Edge case: nil security context must be rejected
// ===========================================================================

func TestAuthorizeDraftSweepApply_NilSecCtxRejected(t *testing.T) {
	err := authorizeDraftSweepApply(nil)
	if err == nil {
		t.Fatal("expected nil secCtx to be rejected")
	}
	if !strings.Contains(err.Error(), pkgctx.PermissionDeleteObjectDraftPlane) {
		t.Errorf("error should name the required capability, got: %v", err)
	}
	if !strings.Contains(err.Error(), "requires an authenticated ACC SecurityContext") {
		t.Errorf("error should mention auth requirement, got: %v", err)
	}
}

// ===========================================================================
// Edge case: system fallback account ID must be rejected (fails closed)
// ===========================================================================

func TestAuthorizeDraftSweepApply_SystemFallbackRejected(t *testing.T) {
	sec := pkgctx.NewSecurityContext(
		pkgctx.SystemAccountID, // system account ID — should be rejected!
		[]string{"admin"},
		[]string{"read:*", "write:*", "delete:*"},
	)
	err := authorizeDraftSweepApply(sec)
	if err == nil {
		t.Fatal("expected system fallback account to be rejected (fail-closed)")
	}
	if !strings.Contains(err.Error(), pkgctx.PermissionDeleteObjectDraftPlane) {
		t.Errorf("error should name the required capability, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unbound system context") {
		t.Errorf("error should mention refusing unbound system, got: %v", err)
	}
}

// ===========================================================================
// Edge case: wildcard write+read but no delete should be denied
// ===========================================================================

func TestAuthorizeDraftSweepApply_WildcardWRNoDeleteDenied(t *testing.T) {
	sec := pkgctx.NewSecurityContext(
		"ACC-WR-NODELETE",
		[]string{"contributor"},
		[]string{"read:*", "write:*"}, // no delete!
	)
	err := authorizeDraftSweepApply(sec)
	if err == nil {
		t.Fatal("expected write+read without delete to be denied")
	}
	if !strings.Contains(err.Error(), pkgctx.PermissionDeleteObjectDraftPlane) {
		t.Errorf("error should name the required capability, got: %v", err)
	}
}

// ===========================================================================
// Edge case: partial delete permission (delete:backlog_item but not draft_plane) denied
// ===========================================================================

func TestAuthorizeDraftSweepApply_PartialDeleteDenied(t *testing.T) {
	sec := pkgctx.NewSecurityContext(
		"ACC-PARTIAL-DEL",
		[]string{"contributor"},
		[]string{
			"delete:backlog_item",
			"read:object_draft_plane",
			"write:object_draft_plane",
		},
	)
	err := authorizeDraftSweepApply(sec)
	if err == nil {
		t.Fatal("expected partial delete permission to be denied")
	}
	if !strings.Contains(err.Error(), "delete:object_draft_plane") {
		t.Errorf("error should mention delete:object_draft_plane requirement, got: %v", err)
	}
}
