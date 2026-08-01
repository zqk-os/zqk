package rollback

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
)

// ObjectRef identifies an object to snapshot (kind + id).
type ObjectRef struct {
	Kind string
	ID   string
}

// SnapshotObjectStates reads current state for each ref from storage and returns ObjectStates.
// State is the full object map so that Apply restores exactly. Use for lifecycle status-relevant
// graph or any maintenance task that needs to snapshot and optionally rollback.
func SnapshotObjectStates(ctx context.Context, provider storage.ObjectStorageProvider, refs []ObjectRef) ([]ObjectState, error) {
	if provider == nil || len(refs) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	out := make([]ObjectState, 0, len(refs))
	for _, r := range refs {
		obj, err := provider.Read(ctx, secCtx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ObjectState{
			Kind:  r.Kind,
			ID:    r.ID,
			State: obj,
		})
	}
	return out, nil
}
