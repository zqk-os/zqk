package filecas

import (
	"errors"

	"context"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
)

type ObjectStorageProvider interface {
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) error
	List(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string) ([]map[string]any, error)
	GetProjectRoot() string
}

type IndexWriteQueue interface {
	EnqueueUpdateWithOperationCallbackAndCreatedAt(kind, objectID, hash, bucketKey, createdAt string, cas *ContentAddressableStorage, opCallback concurrency.OperationCallback) (<-chan error, error)
	EnqueueUpdateWithOperationCallback(kind, objectID, hash, bucketKey string, cas *ContentAddressableStorage, opCallback concurrency.OperationCallback) (<-chan error, error)
	EnqueueUpdateWithCallback(kind, objectID, hash string, cas *ContentAddressableStorage) (<-chan error, error)
	EnqueueInternal(kind, objectID, hash, bucketKey, createdAt string, storage *ContentAddressableStorage, opCallback concurrency.OperationCallback, isSync, isDelete bool) (<-chan error, error)
	EnqueueRemove(kind, objectID string, cas *ContentAddressableStorage) (<-chan error, error)
	EnqueueRemoveNoWait(kind, objectID string, cas *ContentAddressableStorage) error
	FlushKind(kind string, timeout time.Duration) error
}

var GlobalIndexWriteQueue IndexWriteQueue

type StorageMetrics interface {
	RecordCreate(duration time.Duration, success bool)
	RecordRead(duration time.Duration, success bool)
	RecordUpdate(duration time.Duration, success bool)
	RecordDelete(duration time.Duration, success bool)
	RecordIndexFileLock(success bool, duration time.Duration)
	RecordIndexSave(duration time.Duration, err error, count int)
	RecordSetMapping(duration time.Duration, err error, isNew bool)
	RecordIndexReload()
	RecordRemoveMapping(duration time.Duration, err error)
}

var GetMetrics func() StorageMetrics
var IsHighVolumeKindForCache func(kind string) bool
var ErrObjectNotFound = errors.New("object not found")

type LockStrategy interface {
	AcquireLock(path string, timeout time.Duration) (FileLockHandle, error)
}

type FileLockHandle interface {
	Release() error
}

var NewAutoCleanupStrategy func() LockStrategy

type noopStorageMetrics struct{}

func (noopStorageMetrics) RecordCreate(time.Duration, bool)            {}
func (noopStorageMetrics) RecordRead(time.Duration, bool)              {}
func (noopStorageMetrics) RecordUpdate(time.Duration, bool)            {}
func (noopStorageMetrics) RecordDelete(time.Duration, bool)            {}
func (noopStorageMetrics) RecordIndexFileLock(bool, time.Duration)     {}
func (noopStorageMetrics) RecordIndexSave(time.Duration, error, int)   {}
func (noopStorageMetrics) RecordSetMapping(time.Duration, error, bool) {}
func (noopStorageMetrics) RecordIndexReload()                          {}
func (noopStorageMetrics) RecordRemoveMapping(time.Duration, error)    {}

type noopLockStrategy struct{}
type noopLockHandle struct{}

func (noopLockHandle) Release() error { return nil }
func (noopLockStrategy) AcquireLock(path string, timeout time.Duration) (FileLockHandle, error) {
	return noopLockHandle{}, nil
}

func getSafeMetrics() StorageMetrics {
	if GetMetrics != nil {
		if m := GetMetrics(); m != nil {
			return m
		}
	}
	return noopStorageMetrics{}
}

func getSafeLockStrategy() LockStrategy {
	if NewAutoCleanupStrategy != nil {
		if s := NewAutoCleanupStrategy(); s != nil {
			return s
		}
	}
	return noopLockStrategy{}
}
