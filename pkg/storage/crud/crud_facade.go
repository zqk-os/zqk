package crud

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/validation"
)

type FileStorageReadFacade interface {
	ApplyKeystoreAccessControl(obj map[string]any, secCtx *pkgctx.SecurityContext) map[string]any
	CheckPermission(secCtx *pkgctx.SecurityContext, op string, kind string) error
	GetContentAddressableStorage(kind string) (*filecas.ContentAddressableStorage, error)
	GetObjectFilePath(id, kind string) (string, error)
	GetIDValidator() *validation.IDValidator
	GetProjectRoot() string
	ReadObjectFile(ctx context.Context, filePath string) (map[string]any, error)
	ReadStreamBacked(id, kind string, secCtx *pkgctx.SecurityContext) (map[string]any, error)
	UsesContentAddressableStorage(kind string) bool
	VerifyOrReconcileCASHash(ctx context.Context, kind, id, filePath string, data []byte) error
	HasWriteBufferContent() bool
	GetWriteBufferObjectPending(kind, id string) (string, []byte, bool)
	StreamStorageEnabledForKind(kind string) bool
	RuntimeDeltaEnabledForKind(projectRoot, kind string) bool
	ReadRuntimeDeltaCurrentState(projectRoot, kind, id string) map[string]any
	CachedLivePath(filePath string) string
	IsStreamCurrentPath(projectRoot, filePath string) bool
	IsObjectDraftPlanePath(projectRoot, filePath string) bool
	StreamPathAndOffset(pathWithOffset string) (string, int64, bool)
	ReadRecordAt(segmentPath string, offset int64) (map[string]any, error)
	ApplyRuntimeDeltaOverlay(projectRoot, kind, id string, base map[string]any) map[string]any
	MaterializeCasYAMLMapAfterLoad(projectRoot, kind, logicalIDHint string, base map[string]any) map[string]any
}

type CRUDFacade interface {
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error
	ApplyRuntimeDeltaOverlay(projectRoot, kind, id string, base map[string]any) map[string]any
	MaterializeCasYAMLMapAfterLoad(projectRoot, kind, logicalIDHint string, base map[string]any) map[string]any
}
