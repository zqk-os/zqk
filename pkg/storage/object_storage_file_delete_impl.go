package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/kernelcas"
	"github.com/lanceman/zqk/pkg/objects"
)

// Delete deletes an object
//
//nolint:gocyclo
func (f *FileObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	if err := f.CheckTestRepoWriteGuard(); err != nil {
		return err
	}
	// Pipeline COMMIT re-enters here with kernelcas.WithCommit — run underlying path only.
	if kernelcas.IsCommit(ctx) {
		return f.deleteImpl(ctx, secCtx, id, cascade)
	}

	// Require CLI authorization for deletions
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ErrMsgDeleteRequiresCLI)
	}

	existing, err := f.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}
	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ErrMsgObjectNeedsKind)
	}

	// Intent must arrive from the caller. Minting it here from the actor's privilege is what made
	// the guard below unable to refuse the 2026-08-24 retention sweep, which was elevated by
	// construction. TRACK: REDACTED
	return kernelcas.RunErase(ctx, nil, &kernelcas.Mutation{
		Kind:       kind,
		ID:         id,
		Intent:     kernelcas.IntentEraseLogical,
		Cascade:    cascade,
		UnlinkRefs: UnlinkReferencesBeforeDelete(ctx),
		CommitFn: func(c context.Context) error {
			return f.deleteImpl(c, secCtx, id, cascade)
		},
	})
}

// deleteImpl is the authoritative erase body (pipeline COMMIT only).
//
//nolint:gocyclo
