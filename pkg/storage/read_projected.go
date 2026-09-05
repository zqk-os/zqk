package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// ReadProjected loads the object via [ObjectStorageProvider.Read] (full materialization, including
// runtime_delta overlay for applicable kinds), then returns only the requested top-level fields.
// fields empty or nil returns the full map (same reference as Read — no extra copy).
// This reduces downstream marshaling and log payload size; it does **not** avoid reading bytes from disk/CAS.
func ReadProjected(ctx context.Context, secCtx *pkgctx.SecurityContext, store ObjectStorageProvider, id string, fields []string) (map[string]any, error) {
	obj, err := store.Read(ctx, secCtx, id)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return obj, nil
	}
	kind, _ := obj[objects.FieldKeyKind].(string)
	mask := objects.HybridMaskForList(kind, fields, "")
	return objects.ProjectMapHybrid(obj, mask), nil
}
