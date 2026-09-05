package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func (f *FileObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if err := f.CheckTestRepoWriteGuard(); err != nil {
		return err
	}
	handled, err := f.updateViaKernelIfNeeded(ctx, secCtx, id, updates)
	if err != nil || handled {
		return err
	}
	p, err := f.prepareFileObjectUpdate(ctx, secCtx, id, updates)
	if err != nil {
		return err
	}
	if err := f.mergeFileObjectUpdate(p); err != nil {
		return err
	}
	return f.persistFileObjectUpdate(p)
}

// computeUpdatesMap returns a map of field names to new values for fields that were added or changed.
// Used when building change journal entries from previous and new full state (e.g. stream-backed apply).
