package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func (f *FileObjectStorage) UsesContentAddressableStorage(kind string) bool {
	return f.usesContentAddressableStorage(kind)
}

func (f *FileObjectStorage) WriteObjectToStorage(ctx context.Context, id, kind, filePath string, data []byte, secCtx *pkgctx.SecurityContext, isDraft bool) error {
	return f.writeObjectToStorage(ctx, id, kind, filePath, data, secCtx, isDraft)
}
