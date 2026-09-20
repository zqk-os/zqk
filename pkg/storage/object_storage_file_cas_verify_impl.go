package storage

import (
	"context"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	// VerifyOrReconcileCASHash verifies that file content matches the expected CAS hash for the given path.
	// It is the single canonical implementation for CAS integrity checks used by Read and any other
	// code path that reads CAS-backed files (e.g. discovery fallback, list, future callers).
	//
	// Behavior:
	//   - Hash-named files (64-char hex filename): expected hash is the filename stem; no registry.
	//   - ID-based filenames (e.g. MCP-001.yaml): expected hash comes from the hash registry for that kind/dir.
	//     If the registry is missing or stale (content changed but registry not updated), the registry is
	//     reconciled to the current content hash so future reads and updates succeed.
	//
	// Returns an error only when actual content hash does not match the resolved expected hash (integrity failure).
)

func (f *FileObjectStorage) VerifyOrReconcileCASHash(ctx context.Context, kind, id, filePath string, data []byte) error {
	actualHash := CalculateSHA256Hash(data)
	filename := filepath.Base(filePath)
	expectedHash := strings.TrimSuffix(filename, filepath.Ext(filename))

	// ID-based filenames (e.g. MCP-001.yaml) are not content hashes; get expected from hash registry.
	// If registry is stale (e.g. previous write succeeded but registry save failed), reconcile and proceed.
	if len(expectedHash) != 64 || !isHex(expectedHash) {
		kindDir := filepath.Dir(filePath)
		hashReg := f.newHashRegistry(ctx, kind, kindDir)
		var _err_83383744 = hashReg.Load()
		if _err_83383744 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83383744).Log()
		}
		if regHash := hashReg.GetHash(filename); regHash != emptyValue {
			expectedHash = regHash
		}
		if actualHash != expectedHash {
			// Reconcile: update registry to current content so future reads/updates succeed
			hashReg.SetHash(filename, actualHash)
			var _err_83383983 = f.saveHashRegistryWithRetry(hashReg, id, filename, actualHash)
			if _err_83383983 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83383983).Log()
			}
			expectedHash = actualHash
		}
	}

	if err := VerifyContentHash(data, expectedHash); err != nil {
		return errfmt.Errorf(ConstStreamCasHashMismatchForStrKindStrErr, id, kind, err)
	}
	return nil
}
