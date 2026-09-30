package crud

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ReadFileObject retrieves an object by ID.
func ReadFileObject(f FileStorageReadFacade, ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	// Normalize account IDs: convert filename format (account-username) to ID format (account:username)
	normalizedID := id
	if strings.HasPrefix(id, "account-") && !strings.HasPrefix(id, "account:") {
		username := strings.TrimPrefix(id, "account-")
		normalizedID = fmt.Sprintf("account:%s", username)
	}

	// Infer kind from ID
	if err := f.GetIDValidator().LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := f.GetIDValidator().InferKindFromID(normalizedID)
	if kind == emptyValue {
		return nil, errfmt.Errorf("%s: %s", ConstStreamCouldNotInferKindFromIdStr, id)
	}

	// Use normalized ID for file path lookup
	id = normalizedID

	// Check permission
	if err := f.CheckPermission(secCtx, "read", kind); err != nil {
		return nil, err
	}

	// Write-behind: merge pending state so same-process reads see unpersisted writes.
	if opType, opData, ok := f.GetWriteBufferObjectPending(kind, id); ok {
		if opType == "delete" {
			return nil, ErrObjectNotFound
		}
		if len(opData) > 0 {
			var obj map[string]any
			if err := yaml.Unmarshal(opData, &obj); err != nil {
				return nil, errfmt.Newf(ConstStreamFailedToUnmarshalPendingObject).Wrap(err)
			}
			obj[objects.FieldKeyKind] = kind
			return finalizeLoadedObject(f, secCtx, kind, id, obj)
		}
	}

	if f.StreamStorageEnabledForKind(kind) {
		return f.ReadStreamBacked(id, kind, secCtx)
	}

	// Identity index first: one live path (draft YAML or CAS hash).
	if live := f.CachedLivePath(id); live != emptyValue {
		if obj, err := readCachedLivePath(f, ctx, secCtx, id, kind, live); err == nil && obj != nil {
			return obj, nil
		} else if err != nil {
			return nil, err
		}
		// Fall through to authoritative lookup if cached path was deleted or superseded
	}

	// Check if this kind uses content-addressable storage
	if f.UsesContentAddressableStorage(kind) {
		return readContentAddressable(f, ctx, secCtx, id, kind)
	}

	return readDirectFile(f, ctx, secCtx, id, kind)
}

func readCachedLivePath(f FileStorageReadFacade, ctx context.Context, secCtx *pkgctx.SecurityContext, id, kind, filePath string) (map[string]any, error) {
	baseName := filepath.Base(filePath)
	hash := strings.TrimSuffix(baseName, ".yaml")
	isCASFile := len(hash) == 64 && !f.IsStreamCurrentPath(f.GetProjectRoot(), filePath) && !f.IsObjectDraftPlanePath(f.GetProjectRoot(), filePath)

	if isCASFile {
		if cachedRaw, ok := f.GetParseCacheEntry(hash); ok {
			obj := f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, cachedRaw)
			return finalizeLoadedObject(f, secCtx, kind, id, obj)
		}
	}

	data, readErr := fileutil.ReadFile(filePath)
	if readErr != nil {
		if !fileutil.IsNotExist(readErr) {
			return nil, errfmt.Errorf("read after CAS discovery failed: %s %s: %w", id, kind, readErr)
		}
		return nil, nil // Fall through
	}

	if isCASFile {
		if err := f.VerifyOrReconcileCASHash(ctx, kind, id, filePath, data); err != nil {
			return nil, err
		}
	}
	obj := make(map[string]any)
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf("STREAM_FAILED_TO_UNMARSHAL_OBJECT_AFTER_CAS_DISCOVERY").Wrap(err)
	}
	if isCASFile {
		f.PutParseCacheEntry(hash, obj)
	}
	obj = f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, obj)
	return finalizeLoadedObject(f, secCtx, kind, id, obj)
}

func readContentAddressable(f FileStorageReadFacade, ctx context.Context, secCtx *pkgctx.SecurityContext, id, kind string) (map[string]any, error) {
	cas, err := f.GetContentAddressableStorage(kind)
	if err != nil {
		return nil, errfmt.Newf("failed to get content addressable storage").Wrap(err)
	}
	hash, hashErr := cas.GetHashForID(id)
	if hashErr == nil && hash != "" {
		if cachedRaw, ok := f.GetParseCacheEntry(hash); ok {
			obj := f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, cachedRaw)
			return finalizeLoadedObject(f, secCtx, kind, id, obj)
		}
	}
	data, err := cas.Read(id)
	if err == nil {
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil {
			return nil, errfmt.Newf("STREAM_FAILED_TO_UNMARSHAL_OBJECT").Wrap(err)
		}
		if hash != "" {
			f.PutParseCacheEntry(hash, obj)
		}
		obj = f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, obj)
		return finalizeLoadedObject(f, secCtx, kind, id, obj)
	}

	filePath, pathErr := f.GetObjectFilePath(id, kind)
	if pathErr != nil {
		if errors.Is(pathErr, ErrObjectNotFound) {
			return nil, ErrObjectNotFound
		}
		return nil, errfmt.Errorf("CAS read failed: %s %s %w", id, kind, pathErr)
	}

	if segmentPath, offset, ok := f.StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		obj, readErr := f.ReadRecordAt(segmentPath, offset)
		if readErr != nil {
			if errors.Is(readErr, fileutil.ErrNotExist) || errors.Is(readErr, ErrObjectNotFound) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf("read failed: %s %s %w", id, kind, readErr)
		}
		obj[objects.FieldKeyKind] = kind
		return finalizeLoadedObject(f, secCtx, kind, id, obj)
	}

	data, readErr := fileutil.ReadFile(filePath)
	if readErr != nil {
		if fileutil.IsNotExist(readErr) {
			return nil, ErrObjectNotFound
		}
		return nil, errfmt.Errorf("STREAM_READ_AFTER_CAS_DISCOVERY_FAILED: %s %s: %w", id, kind, readErr)
	}
	if !f.IsStreamCurrentPath(f.GetProjectRoot(), filePath) && !f.IsObjectDraftPlanePath(f.GetProjectRoot(), filePath) {
		if err := f.VerifyOrReconcileCASHash(ctx, kind, id, filePath, data); err != nil {
			return nil, err
		}
	}
	obj := make(map[string]any)
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf("STREAM_FAILED_TO_UNMARSHAL_OBJECT_AFTER_CAS_DISCOVERY").Wrap(err)
	}
	obj = f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, obj)
	return finalizeLoadedObject(f, secCtx, kind, id, obj)
}

func readDirectFile(f FileStorageReadFacade, ctx context.Context, secCtx *pkgctx.SecurityContext, id, kind string) (map[string]any, error) {
	filePath, err := f.GetObjectFilePath(id, kind)
	if err != nil {
		return nil, err
	}

	obj, err := f.ReadObjectFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	obj[objects.FieldKeyKind] = kind
	return finalizeLoadedObject(f, secCtx, kind, id, obj)
}

func finalizeLoadedObject(f FileStorageReadFacade, secCtx *pkgctx.SecurityContext, kind, id string, obj map[string]any) (map[string]any, error) {
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		obj = f.ApplyKeystoreAccessControl(obj, secCtx)
	}

	if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue && kind != objects.KindKeystoreEntry {
		return nil, errfmt.Errorf("get invariant violated: read returned meta-only stub (missing status) for %s", id)
	}

	return obj, nil
}

const emptyValue = ""
const ConstStreamFailedToLoadIdPatterns = "failed to load ID patterns"
const ConstStreamCouldNotInferKindFromIdStr = "could not infer kind from ID"
const ConstStreamFailedToUnmarshalPendingObject = "failed to unmarshal pending object"
