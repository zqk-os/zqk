package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func (f *FileObjectStorage) updateNonCASPath(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id, newID string, idUpdated bool, existing, updates, previousStateForJournal map[string]any, oldState, newState string, effectiveUpdates map[string]any, expectedUpdatedAt string) error {
	// Get old file path
	oldFilePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return err
	}

	// If ID was updated, move file to new location
	if idUpdated {
		return f.updateNonCASPathIDChange(ctx, secCtx, kind, id, newID, oldFilePath, existing, updates)
	}

	return f.updateNonCASPathNormal(ctx, secCtx, kind, id, oldFilePath, existing, updates, previousStateForJournal, oldState, newState, effectiveUpdates, expectedUpdatedAt)
}
