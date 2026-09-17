package storage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/storage/crud"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

const (
	updateMutationClassRuntimeDelta = "runtime_delta"
	updateMutationClassStructural   = "structural"
	updateMutationClassIDChange     = "id_change"
)

// Update updates an existing object
//
//nolint:gocyclo // Function orchestrates multiple update phases including optimistic locking, CAS/file handling, and hash registry updates; complexity reduced via helper methods

// applyUpdateFromBuffer persists an update from the write-behind buffer to CAS or file.
// Used by ObjectWriteBehindWorker for "update" ops. Does not call FlushKind.
// When data is nil and kind is stream-only (minimal WAL replay), skips apply (best-effort: already in stream).
// For stream-backed kinds, updates go to change journal + stream_current overlay only (no CAS hash overhead).
func (f *FileObjectStorage) applyUpdateFromBuffer(ctx context.Context, id, kind string, data []byte, secCtx *pkgctx.SecurityContext) error {
	ctx = withSkipWriteBehind(ctx)

	if len(data) == 0 {
		if StreamStorageEnabledForKind(kind) {
			return nil
		}
		return errfmt.Errorf("applyUpdateFromBuffer: empty data for %s/%s", kind, id)
	}
	logicalPath, err := f.getObjectFilePath(id, kind)
	if err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	if logicalPath == emptyValue {
		var err error
		logicalPath, err = f.prepareObjectPath(id, kind)
		if err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}
	dirName, err := f.kindDirectoryName(kind)
	if err != nil {
		return err
	}
	config := GetStorageConfig()

	// Stream-backed: persist via change journal (audit) + stream_current overlay only; no CAS.
	if StreamStorageEnabledForKind(kind) {
		var previousState map[string]any
		if existing, readErr := f.Read(ctx, secCtx, id); readErr == nil {
			previousState = existing
		}
		var newObj map[string]any
		if err := yaml.Unmarshal(data, &newObj); err != nil {
			return errfmt.Newf(ErrMsgStreamUpdateUnmarshal).Wrap(err)
		}
		updates := crud.ComputeUpdatesMap(previousState, newObj)
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, logicalPath, OpUpdate, previousState, updates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := WriteStreamBackedCurrentState(f.projectRoot, kind, id, data); err != nil {
			return errfmt.Newf(ErrMsgStreamUpdateWriteState).Wrap(err)
		}
		f.InvalidateCachesForKind(kind)
		executeChangeNotification(ctx, OpUpdate, kind, id, nil)
		oldState, _ := previousState[objects.FieldKeyStatus].(string)
		newState, _ := newObj[objects.FieldKeyStatus].(string)
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		return nil
	}

	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err != nil {
			return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
		}
		// Read old object before write to compute delta-only updates and lifecycle transitions.
		var oldObj map[string]any
		if existing, readErr := f.Read(ctx, secCtx, id); readErr == nil {
			oldObj = existing
		}
		var newObj map[string]any
		if err := yaml.Unmarshal(data, &newObj); err != nil {
			return errfmt.Newf(ErrMsgUnmarshalUpdateBuf).Wrap(err)
		}
		runtimeDeltaOnly := updateIsRuntimeDeltaOnly(f.projectRoot, kind, crud.ComputeUpdatesMap(oldObj, newObj))
		if runtimeDeltaOnly {
			if err := WriteRuntimeDeltaCurrentState(f.projectRoot, kind, id, data); err != nil {
				return errfmt.Newf(ErrMsgWriteRuntimeDelta).Wrap(err)
			}
			if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, logicalPath, OpUpdate, oldObj, crud.ComputeUpdatesMap(oldObj, newObj), secCtx, f); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, logicalPath, secCtx, nil, f); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			f.InvalidateCachesForKind(kind)
			f.InvalidateCASCacheForKind(kind)
			executeChangeNotification(ctx, OpUpdate, kind, id, nil)
			oldState, _ := oldObj[objects.FieldKeyStatus].(string)
			newState, _ := newObj[objects.FieldKeyStatus].(string)
			if newState != emptyValue && oldState != newState {
				hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
				if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !IsExpectedMissingErr(err) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
			}
			return nil
		}
		if err := cas.Update(id, data); err != nil {
			return errfmt.Newf(ErrMsgUpdateCAS).Wrap(err)
		}
		kindDir := filepath.Join(f.processDir, dirName)
		hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
		if err := hashRegistry.Load(); err == nil {
			hash := CalculateSHA256Hash(data)
			filename := fmt.Sprintf("%s%s", id, config.YAMLExtension)
			hashRegistry.SetHash(filename, hash)
			if err := f.saveHashRegistry(hashRegistry); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Update reverse reference index (best effort)
					Error(ErrMsgSwallowedError, err).Log()
			}
		}

		if newObj != nil {
			updateReverseReferenceIndexOnUpdate(id, oldObj, newObj)
		}
		if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, logicalPath, secCtx, nil, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.InvalidateCachesForKind(kind)
		executeChangeNotification(ctx, OpUpdate, kind, id, nil)
		oldState, _ := oldObj[objects.FieldKeyStatus].(string)
		newState, _ := newObj[objects.FieldKeyStatus].(string)
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		return nil
	}

	filePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return err
	}
	// Read old object for reverse reference index update
	var oldObj map[string]any
	if existing, readErr := f.Read(ctx, secCtx, id); readErr == nil {
		oldObj = existing
	}
	if err := f.writeObjectToFile(ctx, id, kind, filePath, data); err != nil {
		return errfmt.Newf(ErrMsgWriteUpdatedObjFile).Wrap(err)
	}
	// Update reverse reference index (best effort - unmarshal new object data)
	var newObj map[string]any
	if err := yaml.Unmarshal(data, &newObj); err == nil {
		updateReverseReferenceIndexOnUpdate(id, oldObj, newObj)
	}
	if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, nil, f); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	f.InvalidateCachesForKind(kind)
	f.InvalidateCASCacheForKind(kind)
	executeChangeNotification(ctx, OpUpdate, kind, id, nil)
	oldState, _ := oldObj[objects.FieldKeyStatus].(string)
	newState, _ := newObj[objects.FieldKeyStatus].(string)
	if newState != emptyValue && oldState != newState {
		hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
		if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}
	return nil
}

// dispatchStatusGateway is kept on the update path for binary compatibility.
// Status listeners are status_reactive kinds reached via executeLifecycleHook →
// ApplyDependencyRefEvents (catalyst outbound stubs, one hop). Do not walk parents here.
// TRACK: BLI-REDACTED
func (f *FileObjectStorage) dispatchStatusGateway(ctx context.Context, secCtx *SecurityContext, obj map[string]any, kind, oldState string) error {
	_ = ctx
	_ = secCtx
	_ = obj
	_ = kind
	_ = oldState
	return nil
}

// liveCASBlobUnreadable is true when Get found a live hash path but could not
// unmarshal (or hash-verify) it. Update cannot merge with that blob.
