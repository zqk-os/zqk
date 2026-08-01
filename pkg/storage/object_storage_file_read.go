package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

func applyRuntimeDeltaOverlay(projectRoot, kind, id string, base map[string]any) map[string]any {
	if base == nil || StreamStorageEnabledForKind(kind) || !RuntimeDeltaEnabledForKind(projectRoot, kind) {
		return base
	}
	overlay := ReadRuntimeDeltaCurrentState(projectRoot, kind, id)
	if overlay == nil {
		return base
	}
	for k, v := range overlay {
		base[k] = v
	}
	base[objects.FieldKeyKind] = kind
	return base
}

// materializeCasYAMLMapAfterLoad is the single path for merging runtime_delta_current into a map
// loaded from CAS bytes or a YAML file on disk (non-stream kinds). Call this after yaml.Unmarshal
// (or readObjectFile*) and before ParseObject / filter / return.
//
// logicalIDHint resolves the overlay filename when base[FieldKeyID] is missing (should be rare);
// when the payload has FieldKeyID (normal case), that id is used so list workers keyed by CAS
// address still load scheduler_job/SCH-101.yaml, etc.
//
// Always sets FieldKeyKind to kind so Read and List agree without duplicate merge rules elsewhere.
func materializeCasYAMLMapAfterLoad(projectRoot, kind, logicalIDHint string, base map[string]any) map[string]any {
	if base == nil {
		return nil
	}
	logicalID, _ := base[objects.FieldKeyID].(string)
	if logicalID == emptyValue {
		logicalID = logicalIDHint
	}
	if logicalID != emptyValue {
		base = applyRuntimeDeltaOverlay(projectRoot, kind, logicalID, base)
	}
	base[objects.FieldKeyKind] = kind
	return base
}

// Read retrieves an object by ID
func (f *FileObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	// Normalize account IDs: convert filename format (account-username) to ID format (account:username)
	// This handles cases where account IDs are passed in filename format instead of ID format
	normalizedID := id
	if strings.HasPrefix(id, "account-") && !strings.HasPrefix(id, "account:") {
		// Convert account-cursor-vscode -> account:cursor-vscode
		username := strings.TrimPrefix(id, "account-")
		normalizedID = fmt.Sprintf("account:%s", username)
	}

	// Infer kind from ID
	if err := f.idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(normalizedID)
	if kind == emptyValue {
		return nil, errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, id)
	}

	// Use normalized ID for file path lookup
	id = normalizedID

	// Check permission
	if err := f.checkPermission(secCtx, "read", kind); err != nil {
		return nil, err
	}

	// Write-behind: merge pending state so same-process reads see unpersisted writes.
	if f.writeBuf != nil {
		if op := f.writeBuf.GetPending(kind, id); op != nil {
			if op.Op == "delete" {
				return nil, ErrObjectNotFound
			}
			if len(op.Data) > 0 {
				var obj map[string]any
				if err := yaml.Unmarshal(op.Data, &obj); err != nil {
					return nil, errfmt.Newf(ConstStreamFailedToUnmarshalPendingObject).Wrap(err)
				}
				obj[objects.FieldKeyKind] = kind
				keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
				if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
					obj = f.applyKeystoreAccessControl(obj, secCtx)
				}
				return obj, nil
			}
		}
	}

	// Stream-backed kinds: read from stream_current or stream segment only; no CAS.
	if StreamStorageEnabledForKind(kind) {
		return f.readStreamBacked(id, kind, secCtx)
	}

	// Check if this kind uses content-addressable storage
	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err != nil {
			return nil, errfmt.Newf(ErrMsgGetCAS).Wrap(err)
		}
		data, err := cas.Read(id)
		if err == nil {
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				return nil, errfmt.Newf(ConstStreamFailedToUnmarshalObject).Wrap(err)
			}
			obj = materializeCasYAMLMapAfterLoad(f.projectRoot, kind, id, obj)
			keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
			if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
				obj = f.applyKeystoreAccessControl(obj, secCtx)
			}
			return obj, nil
		}
		// CAS index miss: resolve path via getObjectFilePath (discovery fallback scans for hash-named files and updates index)
		filePath, pathErr := f.getObjectFilePath(id, kind)
		if pathErr != nil {
			if errors.Is(pathErr, ErrObjectNotFound) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf(ConstStreamCasReadFailedForStrKindStrErr, id, kind, pathErr)
		}
		// Stream-backed: path is "segmentPath::offset" or stream_current overlay file
		if segmentPath, offset, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
			obj, readErr := ReadRecordAt(segmentPath, offset)
			if readErr != nil {
				if errors.Is(readErr, os.ErrNotExist) {
					return nil, ErrObjectNotFound
				}
				return nil, errfmt.Errorf(ConstStreamStreamReadFailedForStrKindStrErr, id, kind, readErr)
			}
			obj[objects.FieldKeyKind] = kind
			keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
			if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
				obj = f.applyKeystoreAccessControl(obj, secCtx)
			}
			return obj, nil
		}
		// Read from file (stream_current overlay or CAS-discovered path)
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return nil, ErrObjectNotFound
			}
			return nil, errfmt.Errorf(ConstStreamReadAfterCasDiscoveryFailedForStrKindStrErr, id, kind, readErr)
		}
		// Stream_current overlay is not content-addressed; skip hash verification
		if !IsStreamCurrentPath(f.projectRoot, filePath) {
			if err := f.VerifyOrReconcileCASHash(ctx, kind, id, filePath, data); err != nil {
				return nil, err
			}
		}
		obj := make(map[string]any)
		if err := yaml.Unmarshal(data, &obj); err != nil {
			return nil, errfmt.Newf(ConstStreamFailedToUnmarshalObjectAfterCasDiscovery).Wrap(err)
		}
		obj = materializeCasYAMLMapAfterLoad(f.projectRoot, kind, id, obj)
		keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
		if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
			obj = f.applyKeystoreAccessControl(obj, secCtx)
		}
		return obj, nil
	}

	// Get file path
	filePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return nil, err
	}

	// Read file
	obj, err := f.readObjectFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	// Ensure kind field matches the inferred kind (not ontology)
	// This fixes cases where objects were written with ontology value instead of kind
	obj[objects.FieldKeyKind] = kind

	// Apply keystore-specific access control
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		obj = f.applyKeystoreAccessControl(obj, secCtx)
	}

	return obj, nil
}
