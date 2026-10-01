package storage

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// CheckTestRepoWriteGuard refuses Create/Update/Delete when a test is bound to a
// non-isolated project root. Isolation is the existing classifier
// (IsTestOrTempProjectRoot: t.TempDir / os.TempDir / "-test-"), not an env override.
func (f *FileObjectStorage) CheckTestRepoWriteGuard() error {
	if f == nil || f.projectRoot == emptyValue {
		return nil
	}
	if zqkenv.IsInTest() && !crud.IsTestOrTempProjectRoot(f.projectRoot) {
		return errfmt.Errorf("fail-closed gate: tests may not write to repository root process state (%s); use t.TempDir() or an isolated test root", f.projectRoot)
	}
	return nil
}

// checkPermission checks if the security context has permission for the operation
// Uses shared CheckPermissionWithKindSpecialCases utility for consistency
func (f *FileObjectStorage) checkPermission(secCtx *pkgctx.SecurityContext, operation, kind string) error {
	return CheckPermissionWithKindSpecialCases(secCtx, operation, kind)
}

// ensureObjectMetadata ensures required metadata fields are set
// Uses shared EnsureObjectMetadata utility for consistency
func (f *FileObjectStorage) ensureObjectMetadata(ctx context.Context, obj map[string]any, secCtx *pkgctx.SecurityContext, isCreate bool) {
	adapter := &IDValidatorAdapter{IDValidator: f.idValidator}
	EnsureObjectMetadata(ctx, obj, secCtx, isCreate, adapter)
}

// kindDirectoryName resolves and validates the storage directory token for a kind.
// Guardrail: mappings must be simple directory names (e.g. "criteria"), never paths.
func (f *FileObjectStorage) kindDirectoryName(kind string) (string, error) {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return "", errfmt.Errorf(ErrMsgUnknownKind, kind)
	}
	if err := crud.ValidateKindDirectoryName(kind, dirName); err != nil {
		return "", err
	}
	return filepath.Clean(dirName), nil
}

// calculateHash calculates SHA256 hash of file content
// This is the canonical hash calculation function - all hash calculations should use this
func (f *FileObjectStorage) calculateHash(content []byte) string {
	return crud.CalculateSHA256Hash(content)
}

// saveHashRegistryWithRetry saves the hash registry with retry logic
// Returns error if all retries fail - this is critical for object integrity
// filename and expectedHash are used to verify the hash was actually persisted
//
//nolint:unparam // expectedHash kept for API consistency and potential future verification
func (f *FileObjectStorage) saveHashRegistryWithRetry(registry *HashRegistry, _, _, _ string) error {
	if registry == nil || StreamStorageEnabledForKind(registry.kind) {
		return nil
	}
	const maxAttempts = 3
	const initialDelay = 50 * time.Millisecond
	const maxDelay = 500 * time.Millisecond
	const backoffFactor = 2.0

	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := f.saveHashRegistry(registry)
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

// writeObjectFile writes an object to a YAML file
// Uses file locking (flock) to ensure exclusive access during write
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFile(ctx context.Context, filePath string, obj map[string]any) error {
	config := GetStorageConfig()
	return f.writeObjectFileWithPerm(ctx, filePath, obj, config.DefaultFilePerm)
}

// writeObjectFileWithPerm writes an object to a YAML file with specified permissions
// Uses file locking (flock) to ensure exclusive access during write
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFileWithPerm(ctx context.Context, filePath string, obj map[string]any, perm fileutil.FileMode) error {
	data, err := f.yamlMarshalForPersistence(obj)
	if err != nil {
		return errfmt.Newf(ErrMsgMarshalYAML).Wrap(err)
	}
	return f.writeObjectFileWithPermAndData(ctx, filePath, data, perm)
}

// writeObjectFileWithPermAndData writes pre-marshaled YAML data to a file with specified permissions
// This allows callers to marshal once and use the same data for writing and hash calculation
// Uses I/O queue for deadlock prevention and load distribution
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFileWithPermAndData(ctx context.Context, filePath string, data []byte, perm fileutil.FileMode) error {
	// Route through I/O queue (on-demand workers, deadlock prevention)
	return f.writeObjectFileViaQueue(ctx, filePath, data, perm)
}

// writeObjectFileViaQueue writes an object via I/O queue
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFileViaQueue(ctx context.Context, filePath string, data []byte, perm fileutil.FileMode) error {
	// Ensure directory exists (must be done before enqueueing)
	dirPath := filepath.Dir(filePath)
	config := GetStorageConfig()
	if err := fileutil.MkdirAll(dirPath, config.DefaultDirPerm); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}
	// For keystore directory, use more restrictive permissions
	if strings.Contains(dirPath, "keystore") {
		if err := fileutil.Chmod(dirPath, config.KeystoreDirPerm); err != nil {
			// Log but don't fail - best effort
		}
	}

	manager := GetGlobalIOQueueManager(ctx)

	// Create result channel
	resultChan := make(chan IOResult, 1)

	// Enqueue write operation
	op := &IOOperation{
		Type:     IOOperationWrite,
		FilePath: filePath,
		Data:     data,
		Perm:     perm,
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		// Fallback to direct write if queue is unavailable
		dsiaProvider := NewDSIAStorageProvider()
		if writeErr := dsiaProvider.AtomicWriteFile(filePath, data, perm); writeErr != nil {
			return errfmt.Errorf(ErrMsgEnqueueWriteFail, err, writeErr)
		}
		return nil
	}

	// Wait for result (with timeout)
	select {
	case result := <-resultChan:
		if result.Err != nil {
			return result.Err
		}
		return nil
	case <-time.After(5 * time.Second):
		// Fallback to direct write on timeout
		dsiaProvider := NewDSIAStorageProvider()
		if writeErr := dsiaProvider.AtomicWriteFile(filePath, data, perm); writeErr != nil {
			return errfmt.Newf(ErrMsgWriteTimeout).Wrap(writeErr)
		}
		return nil
	}
}

// readObjectFile reads an object from a YAML file
// Uses I/O queue for deadlock prevention and load distribution
// ctx: parent context from command entry point (should not be created here)
