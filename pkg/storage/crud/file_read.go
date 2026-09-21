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

// materializeCasYAMLMapAfterLoad is the single path for merging runtime_delta_current into a map
// loaded from CAS bytes or a YAML file on disk (non-stream kinds). Call this after yaml.Unmarshal
// (or readObjectFile*) and before ParseObject / filter / return.
//
// logicalIDHint resolves the overlay filename when base[FieldKeyID] is missing (should be rare);
// when the payload has FieldKeyID (normal case), that id is used so list workers keyed by CAS
// address still load scheduler_job/SCH-maintenance-wal.yaml, etc.
//
// Always sets FieldKeyKind to kind so Read and List agree without duplicate merge rules elsewhere.

// Read retrieves an object by ID
func ReadFileObject(f FileStorageReadFacade, ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	// Normalize account IDs: convert filename format (account-username) to ID format (account:username)
	// This handles cases where account IDs are passed in filename format instead of ID format
	normalizedID := id
	if strings.HasPrefix(id, "account-") && !strings.HasPrefix(id, "account:") {
		// Convert account-ide-seat-01 ->
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
			keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
			if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
				obj = f.ApplyKeystoreAccessControl(obj, secCtx)
			}
			return obj, nil
		}
	}

	if f.StreamStorageEnabledForKind(kind) {
		return f.ReadStreamBacked(id, kind, secCtx)
	}

	// Identity index first: one live path (draft YAML or CAS hash). Do not dual-read
	// Identity index first: one live path (draft YAML or CAS hash). Do not dual-read
	// draft plane then CAS on every Get. TRACK: TDE-CEF-CAS-IDENTITY-TXN-001
	if live := f.CachedLivePath(id); live != emptyValue {
		filePath := live
		baseName := filepath.Base(filePath)
		hash := strings.TrimSuffix(baseName, ".yaml")
		isCASFile := len(hash) == 64 && !f.IsStreamCurrentPath(f.GetProjectRoot(), filePath) && !f.IsObjectDraftPlanePath(f.GetProjectRoot(), filePath)

		if isCASFile {
			if cachedRaw, ok := f.GetParseCacheEntry(hash); ok {
				obj := f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, cachedRaw)
				keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
				if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
					obj = f.ApplyKeystoreAccessControl(obj, secCtx)
				}
				if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue && kind != objects.KindKeystoreEntry {
					return nil, errfmt.Errorf("get invariant violated: CAS read returned meta-only stub (missing status) for %s", id)
				}
				return obj, nil
			}
		}

		data, readErr := fileutil.ReadFile(filePath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf("STREAM_READ_AFTER_CAS_DISCOVERY_FAILED: %s %s: %w", id, kind, readErr)
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
		keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
		if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
			obj = f.ApplyKeystoreAccessControl(obj, secCtx)
		}
		if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue && kind != objects.KindKeystoreEntry {
			return nil, errfmt.Errorf("get invariant violated: CAS read returned meta-only stub (missing status) for %s", id)
		}
		return obj, nil
	}

	// Check if this kind uses content-addressable storage
	if f.UsesContentAddressableStorage(kind) {
		cas, err := f.GetContentAddressableStorage(kind)
		if err != nil {
			return nil, errfmt.Newf("failed to get content addressable storage").Wrap(err)
		}
		hash, _ := cas.GetHashForID(id)
		if hash != "" {
			if cachedRaw, ok := f.GetParseCacheEntry(hash); ok {
				obj := f.MaterializeCasYAMLMapAfterLoad(f.GetProjectRoot(), kind, id, cachedRaw)
				keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
				if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
					obj = f.ApplyKeystoreAccessControl(obj, secCtx)
				}
				if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue && kind != objects.KindKeystoreEntry {
					return nil, errfmt.Errorf("get invariant violated: CAS read returned meta-only stub (missing status) for %s", id)
				}
				return obj, nil
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
			keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
			if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
				obj = f.ApplyKeystoreAccessControl(obj, secCtx)
			}
			return obj, nil
		}
		// CAS index miss: resolve path via getObjectFilePath (discovery fallback scans for hash-named files and updates index)
		filePath, pathErr := f.GetObjectFilePath(id, kind)
		if pathErr != nil {
			if errors.Is(pathErr, ErrObjectNotFound) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf("STREAM_CAS_READ_FAILED: %s %s %w", id, kind, pathErr)
		}
		// Stream-backed: path is "segmentPath::offset" or stream_current overlay file
		if segmentPath, offset, ok := f.StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
			obj, readErr := f.ReadRecordAt(segmentPath, offset)
			if readErr != nil {
				if errors.Is(readErr, fileutil.ErrNotExist) || errors.Is(readErr, ErrObjectNotFound) {
					return nil, ErrObjectNotFound
				}
				return nil, errfmt.Errorf("STREAM_READ_FAILED: %s %s %w", id, kind, readErr)
			}
			obj[objects.FieldKeyKind] = kind
			keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
			if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
				obj = f.ApplyKeystoreAccessControl(obj, secCtx)
			}
			return obj, nil
		}
		// Read from file (stream_current overlay or CAS-discovered path)
		data, readErr := fileutil.ReadFile(filePath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf("STREAM_READ_AFTER_CAS_DISCOVERY_FAILED: %s %s: %w", id, kind, readErr)
		}
		// Stream_current / draft-plane overlays are not content-addressed; skip hash verification
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
		keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
		if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
			obj = f.ApplyKeystoreAccessControl(obj, secCtx)
		}
		if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue && kind != objects.KindKeystoreEntry {
			return nil, errfmt.Errorf("get invariant violated: CAS read returned meta-only stub (missing status) for %s", id)
		}
		return obj, nil
	}

	// Get file path
	filePath, err := f.GetObjectFilePath(id, kind)
	if err != nil {
		return nil, err
	}

	// Read file
	obj, err := f.ReadObjectFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	// Ensure kind field matches the inferred kind (not ontology)
	// This fixes cases where objects were written with ontology value instead of kind
	obj[objects.FieldKeyKind] = kind

	// Apply keystore-specific access control
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		obj = f.ApplyKeystoreAccessControl(obj, secCtx)
	}

	if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue && kind != objects.KindKeystoreEntry {
		return nil, errfmt.Errorf("get invariant violated: file read returned meta-only stub (missing status) for %s", id)
	}

	return obj, nil
}

const emptyValue = ""
const ConstStreamFailedToLoadIdPatterns = "STREAM_FAILED_TO_LOAD_ID_PATTERNS"
const ConstStreamCouldNotInferKindFromIdStr = "STREAM_COULD_NOT_INFER_KIND_FROM_ID_STR"
const ConstStreamFailedToUnmarshalPendingObject = "STREAM_FAILED_TO_UNMARSHAL_PENDING_OBJECT"
