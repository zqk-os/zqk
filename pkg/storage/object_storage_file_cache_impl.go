package storage

import (
	"context"
	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

func SetCacheOperationHandler(handler func(*pkgctx.CacheContext) error) {
	cacheOperationHandler = handler
}

func GetCacheOperationHandler() func(*pkgctx.CacheContext) error {
	return cacheOperationHandler
}

func SetCacheChecker(checker func(string) (string, bool)) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	when.When(func() bool { return checker == nil }).Then(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileSetCacheCheckerNilInfo).Log()
	}).OrElse(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileSetCacheCheckerSetInfo).
			Bool(ConstStreamCheckerIsNil, checker == nil).
			Log()
	}).Run()
	// Atomic store - lock-free, thread-safe
	cacheChecker.Store(checker)
	// TODO: Emit coordinator event for cache checker state change
	when.When(func() bool { return checker == nil }).Then(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileCacheCheckerNowNilInfo).Log()
	}).OrElse(func() {
		StorageLog(logger).Info(LogEventStorageObjectFileCacheCheckerNowSetInfo).Log()
	}).Run()
}

func GetCacheChecker() func(string) (string, bool) {
	if val := cacheChecker.Load(); val != nil {
		return val.(func(string) (string, bool))
	}
	return nil
}

func cachedLivePath(id string) string {
	if id == emptyValue {
		return emptyValue
	}
	checker := GetCacheChecker()
	if checker == nil {
		return emptyValue
	}
	cachedPath, ok := checker(id)
	if !ok || cachedPath == emptyValue {
		return emptyValue
	}
	if _, err := fileutil.Stat(cachedPath); err != nil {
		return emptyValue
	}
	return cachedPath
}

// executeCacheOperation executes cache operations based on context
// This is called by storage operations to perform cache updates/invalidations
// The storage layer can update the cache context with actual file paths and IDs before execution
func executeCacheOperation(ctx context.Context, filePath string) error {
	return executeCacheOperationWithID(ctx, filePath, "")
}

func executeCacheOperationWithID(ctx context.Context, filePath, id string) error {
	cacheCtx := pkgctx.GetCacheContext(ctx)
	if cacheCtx == nil {
		return nil
	}

	if id != emptyValue && cacheCtx.NewID == emptyValue {
		cacheCtx.NewID = id
	}
	if filePath != emptyValue && cacheCtx.FilePath == emptyValue {
		cacheCtx.FilePath = filePath
	}

	noteID := cacheCtx.NewID
	if noteID == emptyValue {
		noteID = cacheCtx.OldID
	}
	if noteID != emptyValue {
		projectRoot := ""
		if f := unwrapFileStorageFromContext(ctx); f != nil {
			projectRoot = f.projectRoot
		}
		if projectRoot == emptyValue {
			projectRoot = projectRootHintFromCachePath(cacheCtx.FilePath)
		}
		op := string(ObjectIDCachePendingOpUpdate)
		if cacheCtx.Operation == pkgctx.CacheOperationInvalidate {
			op = string(ObjectIDCachePendingOpInvalidate)
		}
		NoteObjectIDCachePending(projectRoot, op, noteID, cacheCtx.Kind, cacheCtx.FilePath, "execute_cache_operation")
	}

	if cacheOperationHandler == nil {
		return errfmt.Errorf(ErrMsgIdentityCacheHandlerRequired)
	}
	return cacheOperationHandler(cacheCtx)
}

func coupleObjectIDCacheLivePath(id, kind, filePath string) error {
	if id == emptyValue || kind == emptyValue || filePath == emptyValue {
		return errfmt.Errorf("couple object-id-cache: missing id, kind, or path")
	}
	if cacheOperationHandler == nil {
		return errfmt.Errorf(ErrMsgIdentityCacheHandlerRequired)
	}
	cacheCtx := &pkgctx.CacheContext{
		Operation: pkgctx.CacheOperationUpdate,
		NewID:     id,
		Kind:      kind,
		FilePath:  filePath,
	}
	if err := cacheOperationHandler(cacheCtx); err != nil {
		return errfmt.Newf("couple object-id-cache").Wrap(err)
	}
	return nil
}

func projectRootHintFromCachePath(filePath string) string {
	if filePath == emptyValue {
		return emptyValue
	}
	// .../.zqk/process/<kind>/<hash>.yaml or .../.zqk/object_drafts/...
	dir := filepath.Dir(filePath)
	for i := 0; i < 6 && dir != emptyValue && dir != "/" && dir != "."; i++ {
		base := filepath.Base(dir)
		if base == "docs" || base == ".zqk" {
			return filepath.Dir(dir)
		}
		dir = filepath.Dir(dir)
	}
	return emptyValue
}

// InvalidateCachesForKind triggers the invalidation shockwave for a specific kind.
// It clears both the global ListCache and the in-memory CAS index cache.
func (f *FileObjectStorage) InvalidateCachesForKind(kind string) {
	InvalidateListCacheForKind(kind)
	f.InvalidateCASCacheForKind(kind)
}
