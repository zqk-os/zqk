// Extracted from object_storage_hybrid.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"errors"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	// HybridObjectStorage wraps two providers: primary and secondary.
	// Writes are performed on both (primary first), while reads are performed on primary only.
)

func (h *HybridObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	primaryTx, err := h.primary.BeginTransaction(ctx)
	if err != nil {
		return nil, err
	}

	secondaryTx, err := h.secondary.BeginTransaction(ctx)
	if err != nil {
		var _err_82820463 = primaryTx.Rollback(ctx)
		if _err_82820463 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82820463).Log()
		}
		return nil, err
	}

	return &HybridObjectTransaction{
		primaryTx:   primaryTx,
		secondaryTx: secondaryTx,
	}, nil
}

// HybridObjectTransaction wraps transactions from both providers
type HybridObjectTransaction struct {
	primaryTx   ObjectTransaction
	secondaryTx ObjectTransaction
}

func (t *HybridObjectTransaction) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if err := t.primaryTx.Create(ctx, secCtx, obj); err != nil {
		return err
	}
	return t.secondaryTx.Create(ctx, secCtx, obj)
}

func (t *HybridObjectTransaction) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, err := t.primaryTx.Read(ctx, secCtx, id)
	if err == nil {
		return obj, nil
	}

	if errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrGraphNotAvailable) ||
		strings.Contains(err.Error(), ErrObjectNotFound.Error()) || strings.Contains(err.Error(), ErrGraphNotAvailable.Error()) {
		return t.secondaryTx.Read(ctx, secCtx, id)
	}

	return nil, err
}

func (t *HybridObjectTransaction) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	_, err := t.primaryTx.Read(ctx, secCtx, id)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrGraphNotAvailable) ||
			strings.Contains(err.Error(), ErrObjectNotFound.Error()) || strings.Contains(err.Error(), ErrGraphNotAvailable.Error()) {
			// Check secondary transaction
			secObj, sErr := t.secondaryTx.Read(ctx, secCtx, id)
			if sErr == nil {
				// Perform lazy migration within transaction: create in primary tx first
				// Merge updates into the object before creating in primary
				for k, v := range updates {
					secObj[k] = v
				}
				if cErr := t.primaryTx.Create(ctx, secCtx, secObj); cErr != nil {
					return errfmt.Newf(ConstStreamLazyMigrationFailedDuringPrimaryCreateForStr, id).Wrap(cErr)
				}
				// Update secondary tx as usual
				if uErr := t.secondaryTx.Update(ctx, secCtx, id, updates); uErr != nil {
					if sErr := handleSecondaryUpdateError(id, uErr); sErr != nil {
						return sErr
					}
				}
				return nil
			}
		}
		return err
	}

	if err := t.primaryTx.Update(ctx, secCtx, id, updates); err != nil {
		return err
	}
	if err := t.secondaryTx.Update(ctx, secCtx, id, updates); err != nil {
		if sErr := handleSecondaryUpdateError(id, err); sErr != nil {
			return sErr
		}
	}
	return nil
}

func (t *HybridObjectTransaction) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	if err := t.primaryTx.Delete(ctx, secCtx, id, cascade); err != nil {
		return err
	}
	if err := t.secondaryTx.Delete(ctx, secCtx, id, cascade); err != nil {
		if !strings.Contains(err.Error(), "object not found") {
			return err
		}
	}
	return nil
}

func (t *HybridObjectTransaction) Commit(ctx context.Context) error {
	if err := t.primaryTx.Commit(ctx); err != nil {
		return err
	}

	// Mirror commit to secondary
	if err := t.secondaryTx.Commit(ctx); err != nil {
		// If secondary commit fails, we have an inconsistency.
		// In a real 2PC we would handle this, but here we log a major error.
		return errfmt.Newf(ConstStreamFailedToMirrorTransactionCommitToSecondaryStorage).Wrap(err)
	}

	return nil
}

func (t *HybridObjectTransaction) Rollback(ctx context.Context) error {
	err1 := t.primaryTx.Rollback(ctx)
	err2 := t.secondaryTx.Rollback(ctx)
	if err1 != nil {
		return err1
	}
	return err2
}

// BulkCreate creates multiple objects in both providers
