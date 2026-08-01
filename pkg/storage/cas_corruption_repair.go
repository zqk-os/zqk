package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

type CASCorruptionRepairOptions struct {
	DryRun        bool
	QuarantineDir string
	Logger        logging.Logger
}

type CASCorruptionRepairResult struct {
	ObjectID        string
	Kind            string
	OldFilePath     string
	NewFilePath     string
	OldFilenameHash string
	ContentHash     string
	QuarantinedTo   string
	Fixed           bool
	Message         string
}

// RepairCASHFilenameMismatch repairs a CAS file whose filename-hash does not match its content hash.
//
// Repair strategy:
// - Write the content to a correctly-named hash file in the same directory as the corrupt file.
// - Update the CAS index mapping ID -> content-hash synchronously (cross-process locked).
// - Move the corrupt file to a quarantine directory for forensic inspection.
//
// This targets the Tier-1 "CAS file corruption detected" class of issues.
func RepairCASHFilenameMismatch(
	_ context.Context,
	projectRoot string,
	kind string,
	objectID string,
	corruptFilePath string,
	options *CASCorruptionRepairOptions,
) (*CASCorruptionRepairResult, error) {
	if options == nil {
		options = &CASCorruptionRepairOptions{}
	}

	res := &CASCorruptionRepairResult{
		ObjectID:    objectID,
		Kind:        kind,
		OldFilePath: corruptFilePath,
		Fixed:       false,
	}

	if projectRoot == emptyValue {
		return res, errfmt.Errorf(ConstStreamProjectRootIsRequired)
	}
	if kind == emptyValue {
		return res, errfmt.Errorf(ConstStreamKindIsRequired)
	}
	if objectID == emptyValue {
		return res, errfmt.Errorf(ConstStreamObjectidIsRequired)
	}
	if corruptFilePath == emptyValue {
		return res, errfmt.Errorf(ConstStreamCorruptfilepathIsRequired)
	}

	data, err := os.ReadFile(corruptFilePath)
	if err != nil {
		return res, errfmt.Newf(ConstStreamFailedToReadFile).Wrap(err)
	}

	contentHash := CalculateSHA256Hash(data)
	res.ContentHash = contentHash

	base := filepath.Base(corruptFilePath)
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".yaml" && ext != ".yml" {
		return res, errfmt.Errorf(ConstStreamExpectedYamlFileGotQuote, base)
	}
	oldNameHash := strings.TrimSuffix(base, ext)
	res.OldFilenameHash = oldNameHash

	// If the file is already correctly addressed (content hash == filename stem), there is nothing to repair.
	// Without this guard, write+rename would "quarantine" the only copy when newFilePath equals corruptFilePath
	// (e.g. repair invoked after content was fixed but before a mistaken second pass).
	if contentHash == oldNameHash {
		res.Fixed = false
		res.NewFilePath = corruptFilePath
		res.Message = ConstStreamFilenameAlreadyMatchesContentHashNoRepairNeeded
		return res, nil
	}

	// Keep bucket placement stable: write corrected file into the same directory as the corrupt file.
	newFilePath := filepath.Join(filepath.Dir(corruptFilePath), contentHash+ext)
	res.NewFilePath = newFilePath

	quarantineDir := options.QuarantineDir
	if quarantineDir == emptyValue {
		quarantineDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir)
	}
	kindQuarantine := filepath.Join(quarantineDir, kind)
	quarantinePath := filepath.Join(kindQuarantine, fmt.Sprintf("%s-%s%s", objectID, oldNameHash, ext))
	res.QuarantinedTo = quarantinePath

	// Dry-run should be cheap: do NOT load CAS index files or perform any writes.
	if options.DryRun {
		res.Message = ConstStreamDryRunWouldWriteCorrectHashFileUpdateIndexAnd
		return res, nil
	}

	// Determine kind directory for index file.
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return res, errfmt.Errorf(ConstStreamUnknownKindStr, kind)
	}
	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
	cas := NewContentAddressableStorage(kindDir, kind)

	// Create the correctly-addressed file (safe create; does not overwrite).
	if err := cas.writeFileWithSync(newFilePath, data); err != nil {
		return res, errfmt.Newf(ConstStreamFailedToWriteRepairedCasFile).Wrap(err)
	}

	// Update index synchronously so other processes resolve the object to the correct file.
	if err := cas.index.SetMapping(objectID, contentHash); err != nil {
		return res, errfmt.Newf(ConstStreamFailedToUpdateCasIndexMapping).Wrap(err)
	}

	// Quarantine the corrupt original.
	if err := os.MkdirAll(kindQuarantine, paths.DirPerm755); err != nil {
		return res, errfmt.Newf(ConstStreamFailedToCreateQuarantineDir).Wrap(err)
	}

	if err := os.Rename(corruptFilePath, quarantinePath); err != nil {
		// If rename fails (e.g., cross-device), fall back to copy+remove.
		if writeErr := os.WriteFile(quarantinePath, data, paths.FilePerm644); writeErr != nil { //nolint:gosec // quarantine file
			return res, errfmt.Errorf(ConstStreamFailedToQuarantineCorruptFileRenameFailedVal, err, writeErr)
		}
		if removeErr := os.Remove(corruptFilePath); removeErr != nil && options.Logger != nil {
			StorageLog(options.Logger).Error(ConstStreamFailedToRemoveCorruptFileAfterQuarantineCopy, removeErr).
				String("op", "os.Remove").
				String("path", corruptFilePath).
				Log()
		}
	}
	res.QuarantinedTo = quarantinePath

	res.Fixed = true
	res.Message = "repaired CAS filename/content hash mismatch"
	if options.Logger != nil {
		StorageLog(options.Logger).Info(LogEventStorageCASCorruptionRepairedInfo).
			ObjectID(objectID).
			Kind(kind).
			String("old_file", corruptFilePath).
			String("new_file", newFilePath).
			String("content_hash", contentHash).
			Log()
	}

	return res, nil
}

type CASCorruptionRepairItem struct {
	Kind            string
	ObjectID        string
	CorruptFilePath string
}

// BatchRepairCASMismatch performs batch repair of multiple CAS filename/content hash mismatches.
func BatchRepairCASMismatch(
	ctx context.Context,
	projectRoot string,
	items []CASCorruptionRepairItem,
	options *CASCorruptionRepairOptions,
) ([]*CASCorruptionRepairResult, error) {
	results := make([]*CASCorruptionRepairResult, 0, len(items))
	for _, item := range items {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
			res, err := RepairCASHFilenameMismatch(ctx, projectRoot, item.Kind, item.ObjectID, item.CorruptFilePath, options)
			if err != nil && res == nil {
				res = &CASCorruptionRepairResult{
					ObjectID: item.ObjectID,
					Kind:     item.Kind,
					Message:  err.Error(),
				}
			}
			results = append(results, res)
		}
	}
	return results, nil
}
