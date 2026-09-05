// Extracted from object_storage_hybrid.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	pkgobjects "github.com/lanceman/zqk/pkg/objects"
	// HybridObjectStorage wraps two providers: primary and secondary.
	// Writes are performed on both (primary first), while reads are performed on primary only.
)

func (h *HybridObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	var ids []string
	for _, obj := range objects {
		if v, ok := obj[pkgobjects.FieldKeyID].(string); ok && v != "" {
			ids = append(ids, v)
		}
	}

	var result *BulkResult
	var err error

	h.withBulkLocks(ids, func() error {
		result, err = h.primary.BulkCreate(ctx, secCtx, objects)
		if err != nil {
			return err
		}

		// Only mirror if there were successes in primary
		if result.SuccessCount > 0 {
			// Note: Secondary might fail for some objects that succeeded in primary.
			// For simplicity, we call secondary BulkCreate.
			_, err = h.secondary.BulkCreate(ctx, secCtx, objects)
			if err != nil {
				// Log warning or handle partial failure?
				// The prompt says secondary failure is a major issue.
				err = errfmt.Newf(ConstStreamFailedToMirrorBulkcreateToSecondaryStorage).Wrap(err)
				return err
			}
		}

		return nil
	})

	return result, err
}

// BulkUpdate updates multiple objects in both providers
func (h *HybridObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	var ids []string
	for _, u := range updates {
		if u.ID != "" {
			ids = append(ids, u.ID)
		}
	}

	var result *BulkResult
	var err error

	h.withBulkLocks(ids, func() error {
		result, err = h.primary.BulkUpdate(ctx, secCtx, updates)
		if err != nil {
			return err
		}

		if result.SuccessCount > 0 {
			_, err = h.secondary.BulkUpdate(ctx, secCtx, updates)
			if err != nil {
				err = errfmt.Newf(ConstStreamFailedToMirrorBulkupdateToSecondaryStorage).Wrap(err)
				return err
			}
		}

		return nil
	})

	return result, err
}

// BulkGet retrieves multiple objects from the primary provider
func (h *HybridObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	return h.primary.BulkGet(ctx, secCtx, ids)
}

// BulkDelete deletes multiple objects from both providers
func (h *HybridObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	var result *BulkResult
	var err error

	h.withBulkLocks(ids, func() error {
		result, err = h.primary.BulkDelete(ctx, secCtx, ids, cascade)
		if err != nil {
			return err
		}

		if result.SuccessCount > 0 {
			successIDs := make([]string, 0, result.SuccessCount)
			for _, m := range result.Results {
				if id, ok := m[pkgobjects.FieldKeyID].(string); ok && id != "" {
					successIDs = append(successIDs, id)
				}
			}
			if len(successIDs) == 0 {
				successIDs = ids
			}
			_, err = h.secondary.BulkDelete(ctx, secCtx, successIDs, cascade)
			if err != nil {
				err = errfmt.Newf(ConstStreamFailedToMirrorBulkdeleteToSecondaryStorage).Wrap(err)
				return err
			}
		}

		return nil
	})

	return result, err
}

// Exists checks if an object exists using the primary provider with fallback to secondary
