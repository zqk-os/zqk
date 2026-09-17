package object

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/storage"
)

// authorizeDraftSweepApply gates mutating draft sweep on existing RBAC
// (SecurityContext permissions), not seat-name strings and not env overrides.
//
// Contract:
//  1. Reject nil / unbound SystemSecurityContext fallback (peers must not inherit system)
//  2. Require delete:object_draft_plane (or delete:* / admin) via CheckPermission
//
// No ZQK_ALLOW_* env bypass — env break-glasses are privilege-bleeding holes
// (tech-debt CVS facet; TRACK: BLI-ENV-BREAKGLASS-REMOVE-001).
//
// Denial messages name the missing capability rather than a policy object ID: kernel objects can
// be archived or deleted, which would leave the CLI pointing at a reference the user cannot read.
func authorizeDraftSweepApply(secCtx *pkgctx.SecurityContext) error {
	if secCtx == nil {
		return errfmt.Errorf(
			"draft sweep apply requires an authenticated ACC SecurityContext with %s "+
				"(dry-run remains open; no env override)",
			pkgctx.PermissionDeleteObjectDraftPlane,
		)
	}
	// Processor falls back to SystemSecurityContext when AuthMiddleware did not bind
	// an ACC — that would make every unbound peer look elevated. Fail closed.
	if secCtx.AccountID == pkgctx.SystemAccountID {
		return errfmt.Errorf(
			"draft sweep apply refuses an unbound system context — "+
				"log in as an ACC account whose role grants %s (no env override)",
			pkgctx.PermissionDeleteObjectDraftPlane,
		)
	}
	if err := storage.CheckPermission(secCtx, storage.OpDelete, pkgctx.KindObjectDraftPlane); err != nil {
		return errfmt.Errorf(
			"draft sweep apply denied — need RBAC permission %s "+
				"(granted on process-admin roles; doers promote/fix only): %w",
			pkgctx.PermissionDeleteObjectDraftPlane, err,
		)
	}
	return nil
}
