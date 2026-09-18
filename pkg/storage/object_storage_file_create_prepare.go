// Extracted from object_storage_file_create_impl.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

func (f *FileObjectStorage) proveCreateVisibility(ctx context.Context, secCtx *pkgctx.SecurityContext, id, kind string, useDraftPlane bool) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		lastErr = f.attemptProveCreateVisibility(ctx, secCtx, id, kind, useDraftPlane)
		if lastErr == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return lastErr
}

func (f *FileObjectStorage) attemptProveCreateVisibility(ctx context.Context, secCtx *pkgctx.SecurityContext, id, kind string, useDraftPlane bool) error {
	if useDraftPlane {
		draftPath := f.objectDraftPlanePath(kind, id)
		if _, statErr := fileutil.Stat(draftPath); statErr != nil {
			return errfmt.Newf("create persisted but draft-plane file not on disk").Wrap(statErr)
		}
		if _, readErr := fileutil.ReadFile(draftPath); readErr != nil {
			return errfmt.Newf("create persisted but draft-plane file not readable").Wrap(readErr)
		}
		if _, readErr := f.Read(ctx, secCtx, id); readErr != nil {
			return errfmt.Newf("create persisted but object not readable").Wrap(readErr)
		}
		return nil
	}
	if f.usesContentAddressableStorage(kind) && !StreamStorageEnabledForKind(kind) && !crud.DeferListingIndexFlushForBulkCreate(ctx, kind) {
		if cas, casErr := f.getContentAddressableStorage(kind); casErr == nil && cas != nil {
			path, pathErr := cas.GetFilePathForID(id)
			if pathErr != nil {
				// Index/pending can lag a just-written bucketed blob. Disk is the proof.
				kindDir := f.GetKindDir(kind)
				discovered, hash, discErr := filecas.DiscoverCASFilePathByScanning(id, kindDir)
				if discErr != nil {
					return errfmt.Newf("create persisted but CAS file path missing").Wrap(pathErr)
				}
				bucket := ""
				if rel, relErr := filepath.Rel(kindDir, filepath.Dir(discovered)); relErr == nil && rel != "." && rel != "" {
					bucket = rel
				}
				cas.HealIndexMappingAsync(id, hash, bucket)
				path = discovered
			}
			if _, statErr := fileutil.Stat(path); statErr != nil {
				return errfmt.Newf("create persisted but CAS file not on disk").Wrap(statErr)
			}
		}
		readObj, readErr := f.Read(ctx, secCtx, id)
		if readErr != nil {
			return errfmt.Newf("create persisted but object not readable").Wrap(readErr)
		}
		if len(readObj) <= 2 || objects.GetString(readObj, objects.FieldKeyStatus) == emptyValue {
			return errfmt.Errorf("create invariant violated: get round-trip hollow for %s", id)
		}
	}
	return nil
}

func (f *FileObjectStorage) validateAndPrepareObjectForCreation(_ context.Context, obj map[string]any, secCtx *pkgctx.SecurityContext) (string, error) {
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		// Attempt to infer kind from ID if present
		id, _ := obj[objects.FieldKeyID].(string)
		if id != "" && f.idValidator != nil {
			kind = f.idValidator.InferKindFromID(id)
			if kind != "" {
				obj[objects.FieldKeyKind] = kind
				ok = true
			}
		}
	}
	if !ok || kind == emptyValue {
		return "", errfmt.Errorf(ErrMsgObjectNeedsKind)
	}

	if err := f.checkPermission(secCtx, "write", kind); err != nil {
		return "", err
	}

	// For keystore_entry, ensure account_id is set from security context if not provided
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		if err := f.prepareKeystoreEntry(obj, secCtx); err != nil {
			return "", err
		}
	}

	return kind, nil
}

// ensureObjectID ensures object has a valid ID, generating one if needed
func (f *FileObjectStorage) ensureObjectID(ctx context.Context, obj map[string]any, kind string) (string, error) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		StorageLog(logger).Debug(LogEventStorageObjectCreateEnsureNoIDGenerating).
			Kind(kind).
			Log()

		// For CAS-enabled kinds, use timestamp-based IDs because CAS uses hash-based filenames
		// and sequential ID generation can't scan the directory to find the next sequence number
		if f.usesContentAddressableStorage(kind) {
			// Get ID prefix from validator (strict - no fallback); LoadPatterns waits if another goroutine is loading
			if err := f.idValidator.LoadPatterns(); err != nil {
				return "", errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
			}
			prefixes := f.idValidator.GetValidPrefixes(kind)
			if len(prefixes) == 0 {
				return "", errfmt.Errorf(ErrMsgNoValidIDPrefix, kind)
			}
			// Use first prefix (most common case)
			prefix := NormalizeCASIDPrefix(prefixes[0])

			// Generate unique timestamp-based ID with random component to prevent collisions
			// Even with nanosecond precision, concurrent goroutines can generate IDs at the same nanosecond
			// Adding a small random component (4 bytes = 8 hex chars) ensures uniqueness
			// Format: PREFIX-timestamp-random (e.g., BAS-1768909936457275000-a1b2c3d4)
			baseTime := time.Now().UnixNano()
			randomBytes := make([]byte, 4) // 4 bytes = 8 hex characters
			if _, err := rand.Read(randomBytes); err != nil {
				// Fallback: use timestamp with nanosecond delay if random fails
				StorageLog(logger).Warn(LogEventStorageObjectCreateEnsureRandomComponentFailed).
					Kind(kind).
					WithError(err).
					Log()
				id = fmt.Sprintf("%s%d", prefix, baseTime)
				obj[objects.FieldKeyID] = id
			} else {
				randomHex := hex.EncodeToString(randomBytes)
				generatedID := fmt.Sprintf("%s%d-%s", prefix, baseTime, randomHex)
				StorageLog(logger).Debug(LogEventStorageObjectCreateEnsureGeneratedCASID).
					Kind(kind).
					String("generated_id", generatedID).
					Log()
				id = generatedID
				obj[objects.FieldKeyID] = id
			}
		} else {
			// Use thread-safe batch generator for non-CAS kinds (consistent with audit IDs and other sequential IDs)
			generatedID, err := f.generateID(ctx, kind)
			if err != nil {
				StorageLog(logger).Error(LogEventStorageObjectCreateEnsureGenerateIDFailed, err).
					Kind(kind).
					Log()
				return "", errfmt.Newf(ErrMsgGenerateID).Wrap(err)
			}
			StorageLog(logger).Debug(LogEventStorageObjectCreateEnsureIDGeneratedOK).
				Kind(kind).
				String("generated_id", generatedID).
				Log()
			id = generatedID
			obj[objects.FieldKeyID] = id
		}
	}

	// Legacy churn ids: SCH-<ts>-scheduler-job-<parent> chains when parent was already a long SCH-* id.
	// Normalize to a short hash-based id so we never persist unbounded recursive names.
	if kind == objects.KindSchedulerJob {
		if normalized := normalizeSchedulerJobIDIfRecursive(kind, id); normalized != id {
			StorageLog(logger).Warn(LogEventStorageObjectCreateEnsureNormalizedRecursiveSchedulerJob).
				Kind(kind).
				String("previous_id", id).
				String("normalized_id", normalized).
				Log()
			id = normalized
			obj[objects.FieldKeyID] = id
		}
	}

	// scheduler_job ids are used as lock filenames; filesystems have NAME_MAX ~255. Reject long ids at create so we never persist them.
	const maxSchedulerJobIDLen = 200
	if kind == objects.KindSchedulerJob && len(id) > maxSchedulerJobIDLen {
		return "", errfmt.Errorf(ErrMsgSchedulerJobIDLength, maxSchedulerJobIDLen, len(id))
	}

	// Validate ID format (strict - must pass validation); LoadPatterns waits if another goroutine is loading
	if err := f.idValidator.LoadPatterns(); err != nil {
		return "", errfmt.Newf(ErrMsgLoadIDPatternsValidation).Wrap(err)
	}
	valid, err := f.idValidator.ValidateID(id, kind)
	if err != nil {
		return "", errfmt.Newf(ErrMsgValidateID).Wrap(err)
	}
	if !valid {
		return "", errfmt.Errorf(ErrMsgInvalidIDFormat, kind, id)
	}

	return id, nil
}

// prepareObjectPath prepares the file path and ensures directory exists.
// For CAS kinds, we build the legacy path directly so Create does not call getObjectFilePath
// for non-existing objects (getObjectFilePath returns an error when the object is not found and kind dir exists).
func (f *FileObjectStorage) prepareObjectPath(id, kind string) (string, error) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return "", errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}
	config := GetStorageConfig()
	if err := fileutil.MkdirAll(kindDir, config.DefaultDirPerm); err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathMkdirFailed, err).
			Kind(kind).
			ObjectID(id).
			String("dir_path", kindDir).
			Log()
		return "", errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}
	var filePath string
	if f.usesContentAddressableStorage(kind) {
		accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
		when.When(func() bool {
			return accountDir == "accounts" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(id, "account:")
		}).Then(func() {
			username := strings.TrimPrefix(id, "account:")
			filePath = filepath.Join(kindDir, fmt.Sprintf("account-%s%s", username, config.YAMLExtension))
		}).OrElse(func() {
			filePath = filepath.Join(kindDir, fmt.Sprintf("%s%s", id, config.YAMLExtension))
		}).Run()
	} else {
		var err error
		filePath, err = f.getObjectFilePath(id, kind)
		if err != nil {
			StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathGetObjectPathFailed, err).
				Kind(kind).
				ObjectID(id).
				Log()
			return "", err
		}
	}
	dirPath := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dirPath, config.DefaultDirPerm); err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathMkdirFailed, err).
			Kind(kind).
			ObjectID(id).
			String("dir_path", dirPath).
			Log()
		return "", errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}
	// For keystore, use more restrictive permissions
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		if err := fileutil.Chmod(filepath.Dir(filePath), config.KeystoreDirPerm); err != nil {
			// Log but don't fail - best effort
		}
	}
	return filePath, nil
}

// checkObjectExists checks if object already exists (stream registry, CAS, or file-based).
// For stream-backed kinds, only the stream registry is authoritative so Create can succeed when the ID
// exists only in legacy CAS/file and not in the stream (e.g. ensure-retention-jobs creating fixed-ID jobs).
func (f *FileObjectStorage) checkObjectExists(id, kind, filePath string) error {
	// Check for blocking issues before write operations (except for automated kinds)
	// Note: This requires context, but we'll pass it through the main Create method
	// For now, we'll skip this check here and do it in validateObjectBeforeCreation

	// For stream-backed kinds, existence is defined only by the stream registry (List reads from there).
	// Do not use CAS or legacy file path so Create can add the ID to the stream when it's missing there.
	if StreamStorageEnabledForKind(kind) {
		if f.getStreamLocation(id, kind) != emptyValue {
			return ErrObjectExists
		}
		return nil
	}

	if f.objectDraftPlaneExists(kind, id) {
		return ErrObjectExists
	}

	// For CAS-enabled kinds, check the CAS index instead of file path
	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err == nil {
			_, hashErr := cas.GetHashForID(id)
			if hashErr == nil {
				_, pathErr := cas.GetFilePathForID(id)
				if pathErr == nil {
					return ErrObjectExists
				}
				// Stale index: ID maps to a hash whose file is gone — allow Create to repair.
				return nil
			}
		}
		// If CAS check fails, fall through to file-based check as backup
	}

	// For non-CAS kinds, check file existence
	if _, err := fileutil.Stat(filePath); err == nil {
		return ErrObjectExists
	}

	return nil
}

// errIfDraftCreateWouldDualPlane refuses a draft-plane Create when the CAS index
// already maps id. checkObjectExists allows Create when the hash file is missing
// (index repair); that repair must not land a draft shadow beside a live CAS blob.
// TRACK: BLI-CAS-HAND-DUP-CHECK-001
func (f *FileObjectStorage) errIfDraftCreateWouldDualPlane(id, kind string) error {
	if f == nil || id == emptyValue || kind == emptyValue {
		return nil
	}
	if StreamStorageEnabledForKind(kind) || !f.usesContentAddressableStorage(kind) {
		return nil
	}
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil || cas == nil {
		return nil
	}
	if _, hashErr := cas.GetHashForID(id); hashErr == nil {
		return errfmt.Errorf("create: refusing draft-plane write for %s — CAS index already maps this id (dual-plane)", id)
	}
	return nil
}

// marshalObjectForCreation marshals object to YAML for writing
func (f *FileObjectStorage) marshalObjectForCreation(obj map[string]any, filePath string) ([]byte, error) {
	// Marshal object to YAML before writing
	// Git's approach: Calculate hash from content BEFORE writing, then trust the write
	// This eliminates race conditions from reading back immediately after writing
	data, err := f.yamlMarshalForPersistence(obj)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgMarshalObjCreation).Wrap(err)
	}

	if len(data) == 0 {
		return nil, errfmt.Errorf(ErrMsgEmptyContent, filePath)
	}

	return data, nil
}

// writeObjectToStorage writes object to storage (stream, draft plane, CAS, or file-based).
// When stream storage is enabled for the kind, writes to append-only segment (no CAS overhead).
// Preliminary CAS kinds write to the id-keyed draft plane (no hash/index) until leave-preliminary.
// useDraftPlane is decided by the caller from the live object map (do not re-derive from YAML).
// CAS / draft-plane process objects go through PrivilegedWriter (fail-closed) unless tests set
// ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1 — see privileged_writer_membrane.go.
