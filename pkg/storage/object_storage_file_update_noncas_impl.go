package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
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

// UpdateNonCASPathNormalForTest exposes updateNonCASPathNormal for unit tests.
func (f *FileObjectStorage) UpdateNonCASPathNormalForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id, oldFilePath string, existing, updates, previousStateForJournal map[string]any, oldState, newState string, effectiveUpdates map[string]any, expectedUpdatedAt string) error {
	return f.updateNonCASPathNormal(ctx, secCtx, kind, id, oldFilePath, existing, updates, previousStateForJournal, oldState, newState, effectiveUpdates, expectedUpdatedAt)
}

// UpdateNonCASPathIDChangeForTest exposes updateNonCASPathIDChange for unit tests.
func (f *FileObjectStorage) UpdateNonCASPathIDChangeForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id, newID, oldFilePath string, existing, updates map[string]any) error {
	return f.updateNonCASPathIDChange(ctx, secCtx, kind, id, newID, oldFilePath, existing, updates)
}

// UpdateNonCASPathForTest exposes updateNonCASPath for testing.
func (f *FileObjectStorage) UpdateNonCASPathForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id, newID string, idUpdated bool, existing, updates, previousStateForJournal map[string]any, oldState, newState string, effectiveUpdates map[string]any, expectedUpdatedAt string) error {
	return f.updateNonCASPath(ctx, secCtx, kind, id, newID, idUpdated, existing, updates, previousStateForJournal, oldState, newState, effectiveUpdates, expectedUpdatedAt)
}

