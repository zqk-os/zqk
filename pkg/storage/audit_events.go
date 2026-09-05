package storage

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/audit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// WriteSystemObjectAndRegisterHash writes a system object to disk and registers its hash
// Git's approach: Calculate hash from content BEFORE writing, then trust file.Sync() succeeded
// This eliminates race conditions from reading back immediately after writing
// If fileStorage is provided and the kind uses CAS, routes through CAS instead of direct file write
//
// Do not use for stream-backed kinds (audit_event, scheduler_job, etc.) when stream storage is
// enabled—that would create YAML under docs/process. Use storage.Create or writeObjectToStorage
// so writes go to the stream. See stream_config.go and docs/architecture/STREAM_STORAGE.md §7.
func WriteSystemObjectAndRegisterHash(filePath string, data []byte, kind, kindDir, objectID string, fileStorage *FileObjectStorage) error {
	if len(data) == 0 {
		return errfmt.Errorf(ErrMsgEmptyContent, filePath)
	}
	if StreamStorageEnabledForKind(kind) {
		return errfmt.Errorf(ErrMsgStreamBacked, kind, paths.ProcessDir)
	}

	// If fileStorage is provided and this kind uses CAS, route through CAS
	if fileStorage != nil && fileStorage.usesContentAddressableStorage(kind) {
		cas, err := fileStorage.getContentAddressableStorage(kind)
		if err != nil {
			// CAS is required for kinds that use it - do not fall back to direct write
			// This ensures consistency (e.g., audit events must always use CAS)
			return errfmt.Errorf(ErrMsgNoCAS, kind, err)
		}

		// Extract bucket directory from filePath if it's different from kindDir
		// This supports bucketed storage (e.g., audit/2026-01/)
		var bucketDir string
		if filePath != emptyValue {
			fileDir := filepath.Dir(filePath)
			// If fileDir is different from kindDir, it's in a bucket
			if fileDir != kindDir && strings.HasPrefix(fileDir, kindDir) {
				bucketDir = fileDir
			}
		}

		// Use CAS for creation, passing bucket directory if applicable
		if bucketDir != emptyValue {
			if err := cas.Create(objectID, data, bucketDir); err != nil {
				return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
			}
		} else {
			if err := cas.Create(objectID, data); err != nil {
				return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
			}
		}
		// Update reverse reference index (best effort - system objects may reference other objects)
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err == nil {
			updateReverseReferenceIndexOnCreate(objectID, obj)
		}
		return nil
	}

	// Fall back to direct file write (for non-CAS kinds or if CAS unavailable)
	// Calculate hash from the data we're about to write (Git's approach)
	// This is the hash of what we're writing, and file.Sync() guarantees it's on disk
	hash := CalculateSHA256Hash(data)

	// Ensure directory exists
	dirPath := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Write file to disk using file handle so we can sync
	file, err := fileutil.OpenFile(filePath, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_TRUNC, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenFileWrite).Wrap(err)
	}
	defer file.Close()

	// Write data
	if _, err := file.Write(data); err != nil {
		return errfmt.Newf(ErrMsgWriteFile).Wrap(err)
	}

	// Sync file to ensure write is persisted to disk
	// Git trusts this sync - no read-back verification needed
	if err := file.Sync(); err != nil {
		// Log warning but don't fail - sync is best effort
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditSyncAfterWriteFailedWarn).Path(filePath).WithError(err).Log()
	}

	// Update hash registry (use cached registry when fileStorage is provided to cap goroutine growth)
	ctx := pkgctx.NewSystemContext()
	var hashRegistry *HashRegistry
	if fileStorage != nil {
		hashRegistry = fileStorage.newHashRegistry(ctx, kind, kindDir)
	} else {
		hashRegistry = NewHashRegistry(ctx, kind, kindDir)
	}
	if hashRegistry == nil {
		return errfmt.Errorf(ErrMsgGetHashReg, kind)
	}
	if err := hashRegistry.Load(); err != nil {
		// Hash registry doesn't exist yet - will be created on first save
	}
	filename := filepath.Base(filePath)
	hashRegistry.SetHash(filename, hash)

	// Use retry logic to ensure hash registry update succeeds
	if err := saveHashRegistryWithRetryForSystemObject(hashRegistry, objectID, filename, hash); err != nil {
		return errfmt.Newf(ErrMsgUpdateHashReg).Wrap(err)
	}

	// Update reverse reference index (best effort - system objects may reference other objects)
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err == nil {
		updateReverseReferenceIndexOnCreate(objectID, obj)
	}

	return nil
}

// saveHashRegistryWithRetryForSystemObject saves the hash registry with retry logic for system-created objects
// CRITICAL: System-created objects (audit events, change journal entries) must have their hashes registered
// to prevent hash mismatches. This function ensures the hash is persisted even under concurrent access.
//
//nolint:unparam // expectedHash kept for API consistency with saveHashRegistryWithRetry
func saveHashRegistryWithRetryForSystemObject(registry *HashRegistry, _, _, _ string) error {
	const maxAttempts = 3
	const initialDelay = 50 * time.Millisecond
	const maxDelay = 500 * time.Millisecond
	const backoffFactor = 2.0

	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := registry.Save()
		if err == nil {
			// Success - Save() already does file.Sync(), so we trust it succeeded
			// Git doesn't verify by reading back - it trusts the write succeeded
			// Reading back immediately can hit file system caching issues (especially on macOS)
			// The file.Sync() in Save() is the real guarantee, not a read-back verification
			return nil
		}
		lastErr = err

		// Last attempt, don't wait
		if attempt == maxAttempts-1 {
			break
		}

		// Wait before retry with exponential backoff
		time.Sleep(delay)
		delay = time.Duration(float64(delay) * backoffFactor)
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	// All retries exhausted
	return errfmt.Errorf(ErrMsgSaveHashReg, maxAttempts, lastErr)
}

// IsCLIOperation checks if the context indicates this is a CLI operation.
func IsCLIOperation(ctx context.Context, secCtx *pkgctx.SecurityContext) bool {
	return audit.IsCLIOperation(ctx, secCtx)
}

// WithCLIOperation marks a context as a CLI operation.
func WithCLIOperation(ctx context.Context) context.Context {
	return audit.WithCLIOperation(ctx)
}

// createDeleteAuditEvent creates an audit event for object deletion
// fileStorage is optional - if provided and CAS is enabled, routes through CAS
// When ctx has WithDeferAuditEvents, the event is enqueued and flushed later (e.g. by BulkDeleteOptimized).
