package cas

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage/filecas"
)

// StorageFacade is used for CAS Recovery and other file-specific CAS operations.
type StorageFacade interface {
	GetContentAddressableStorage(kind string) (*filecas.ContentAddressableStorage, error)
}

// MetricsStorageFacade is used by metrics collectors which operate on the abstract ObjectStorageProvider.
type MetricsStorageFacade interface {
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
}

// CASFacade is the CAS subpackage contract implemented by FileObjectStorage.
// Queue SetStorage still accepts this type; stored values must satisfy both
// StorageFacade and MetricsStorageFacade (the live store does).
type CASFacade interface {
	StorageFacade
	MetricsStorageFacade
}
