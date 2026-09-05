package migration

import (
	"context"
	"github.com/lanceman/zqk/pkg/validation"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/filecas"
)

type IDValidator interface {
	LoadPatterns() error
	InferKindFromID(id string) string
}

type StorageFacade interface {
	GetProjectRoot() string
	GetLogger() logging.Logger
	UsesContentAddressableStorage(kind string) bool
	GetContentAddressableStorage(kind string) (*filecas.ContentAddressableStorage, error)
	GetProcessDir() string
	DiscoverObjectKinds() []string
	ScanIDBasedFilesRecursive(kindDir, kind string) map[string]string
	ScanIDBasedFilesWithPaths(kindDir, kind string) map[string]string
	GetIDValidator() *validation.IDValidator
	WriteObjectToStorage(ctx context.Context, id, kind, filePath string, data []byte, secCtx *pkgctx.SecurityContext, isDraft bool) error
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, objectID string) (map[string]any, error)
	ValidateObject(ctx context.Context, obj map[string]any, kind, secCtx string) error // Wait, secCtx is string? No, in cas_migration_utility.go:43 it's passing secCtx string?
	GetObjectFilePath(id, kind string) (string, error)
}

type CASMigrationUtility struct{ facade StorageFacade }

func NewCASMigrationUtility(f StorageFacade) *CASMigrationUtility {
	return &CASMigrationUtility{facade: f}
}

type DSIAMigrationUtility struct{ facade StorageFacade }

func NewDSIAMigrationUtility(f StorageFacade) *DSIAMigrationUtility {
	return &DSIAMigrationUtility{facade: f}
}
