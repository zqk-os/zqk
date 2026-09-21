package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/objects"
)

func (f *FileObjectStorage) updateViaKernelIfNeeded(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) (bool, error) {
	// Kernel Mutation Pipeline entry (COMMIT re-enters with kernelcas.WithCommit).
	// TRACK: follow-up in kernel backlog
	if !kernelcas.IsCommit(ctx) {
		kind := ""
		if f.idValidator != nil {
			_ = f.idValidator.LoadPatterns()
			kind = f.idValidator.InferKindFromID(id)
		}
		intent := kernelcas.IntentUpdateFields
		if _, hasStatus := updates[objects.FieldKeyStatus]; hasStatus {
			intent = kernelcas.IntentTransition
		}
		run := kernelcas.RunUpdate
		if intent == kernelcas.IntentTransition {
			run = kernelcas.RunTransition
		}
		return true, run(ctx, nil, &kernelcas.Mutation{
			Kind:   kind,
			ID:     id,
			Intent: intent,
			Reason: pkgctx.GetLifecycleBreakGlassReason(ctx),
			CommitFn: func(c context.Context) error {
				return f.Update(c, secCtx, id, updates)
			},
		})
	}

	return false, nil
}
