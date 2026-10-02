// Package storage: shared stream-backed Read, Exists, and Update helpers.
// Use when StreamStorageEnabledForKind(kind) so call sites stay DRY and logic lives in one place.

package storage

import (
	"context"
	"errors"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ensureStreamBackedResult normalizes a decoded stream-backed object: nil → empty map, sets kind, applies keystore access control.
func (f *FileObjectStorage) ensureStreamBackedResult(obj map[string]any, kind string, secCtx *pkgctx.SecurityContext) map[string]any {
	if obj == nil {
		obj = make(map[string]any)
	}
	obj[objects.FieldKeyKind] = kind
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		obj = f.applyKeystoreAccessControl(obj, secCtx)
	}
	return obj
}

// readStreamBacked reads an object from stream_current or stream segment; no CAS.
// Call when StreamStorageEnabledForKind(kind). Applies keystore access control when kind is keystore_entry.
func (f *FileObjectStorage) readStreamBacked(id, kind string, secCtx *pkgctx.SecurityContext) (map[string]any, error) {
	filePath, pathErr := f.getObjectFilePath(id, kind)
	if pathErr != nil {
		if errors.Is(pathErr, ErrObjectNotFound) {
			return nil, ErrObjectNotFound
		}
		return nil, errfmt.Errorf(ConstStreamStreamReadFailedForStrKindStrErr, id, kind, pathErr)
	}
	if segmentPath, offset, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		obj, readErr := ReadRecordAt(segmentPath, offset)
		if readErr != nil {
			if errors.Is(readErr, fileutil.ErrNotExist) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf(ConstStreamStreamReadFailedForStrKindStrErr, id, kind, readErr)
		}
		return f.ensureStreamBackedResult(obj, kind, secCtx), nil
	}
	data, readErr := fileutil.ReadFile(filePath)
	if readErr != nil {
		if fileutil.IsNotExist(readErr) {
			return nil, ErrObjectNotFound
		}
		return nil, errfmt.Errorf(ConstStreamReadFailedForStrKindStrErr, id, kind, readErr)
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToUnmarshalObject).Wrap(err)
	}
	return f.ensureStreamBackedResult(obj, kind, secCtx), nil
}

// existsStreamBacked reports whether the object exists at stream_current or stream segment; no CAS.
// Call when StreamStorageEnabledForKind(kind).
func (f *FileObjectStorage) existsStreamBacked(ctx context.Context, id, kind string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	filePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return false, nil
	}
	if segmentPath, _, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		_, err := fileutil.Stat(segmentPath)
		return err == nil, nil
	}
	return fileExistsOnDisk(filePath, ConstStreamFailedToCheckStreamBackedObjectExistence)
}

func fileExistsOnDisk(filePath, errMsg string) (bool, error) {
	_, err := fileutil.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if fileutil.IsNotExist(err) {
		return false, nil
	}
	return false, errfmt.Newf(errMsg).Wrap(err)
}

// updateStreamBacked writes stream_current + change journal and invalidates list cache; no CAS or hash registry.
// Call when StreamStorageEnabledForKind(kind). idUpdated/newID are for audit and notification only.
func (f *FileObjectStorage) updateStreamBacked(ctx context.Context, id, kind string, idUpdated bool, newID string, existing map[string]any, updates map[string]any, previousStateForJournal map[string]any, secCtx *pkgctx.SecurityContext) error {
	data, err := yaml.Marshal(existing)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToMarshalUpdatedObject).Wrap(err)
	}
	logging.LogSwallowedError(createChangeJournalEntry(ctx, f.projectRoot, id, kind, "", OpUpdate, previousStateForJournal, updates, secCtx, f))
	if err := WriteStreamBackedCurrentState(f.projectRoot, kind, id, data); err != nil {
		return errfmt.Errorf(ConstStreamFailedToWriteStreamCurrentForStrKindStrErr, id, kind, err)
	}
	f.InvalidateCachesForKind(kind)
	objectIDForAudit := id
	if idUpdated {
		objectIDForAudit = newID
	}
	changedFields := make([]string, 0, len(updates))
	for k := range updates {
		changedFields = append(changedFields, k)
	}
	logging.LogSwallowedError(createUpdateAuditEvent(ctx, f.projectRoot, id, kind, "", secCtx, changedFields, f))
	executeChangeNotification(ctx, OpUpdate, kind, objectIDForAudit, existing)
	return nil
}
