package system

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

// getCachedCASForKind returns a CAS instance for the kind from the global storage provider cache
// when projectRoot is non-empty, so we avoid loading the full index per call (OOM risk in hot paths).
// Returns (cas, true) when found; (nil, false) when not available (caller should use NewContentAddressableStorage).
func getCachedCASForKind(ctx context.Context, projectRoot, kind string) (*storage.ContentAddressableStorage, bool) {
	if projectRoot == emptyValue || kind == emptyValue {
		return nil, false
	}
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	provider, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)
	if err != nil {
		return nil, false
	}
	fileStorage, ok := provider.(*storage.FileObjectStorage)
	if !ok {
		return nil, false
	}
	cas, err := fileStorage.GetContentAddressableStorage(kind)
	if err != nil || cas == nil {
		return nil, false
	}
	return cas, true
}
