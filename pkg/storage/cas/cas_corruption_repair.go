package cas

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
// TRACK: BLI-CEF-R14-RCV-CAS-REPAIR-001 / CRIT-CEF-R14-RCV-CAS-REPAIR-001 / REQ-CEF-R14-RCV-SEC-001
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

	if projectRoot == "" {
		return res, errfmt.Errorf("project root is required")
	}
	if kind == "" {
		return res, errfmt.Errorf("kind is required")
	}
	if objectID == "" {
		return res, errfmt.Errorf("object id is required")
	}
	if corruptFilePath == "" {
		return res, errfmt.Errorf("corrupt file path is required")
	}

	data, err := fileutil.ReadFile(corruptFilePath)
	if err != nil {
		return res, errfmt.Newf("failed to read file").Wrap(err)
	}

	contentHash := calculateSHA256Hash(data)
	res.ContentHash = contentHash

	base := filepath.Base(corruptFilePath)
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".yaml" && ext != ".yml" {
		return res, errfmt.Errorf("expected .yaml or .yml file, got %q", base)
	}
	oldNameHash := strings.TrimSuffix(base, ext)
	res.OldFilenameHash = oldNameHash

	// If the file is already correctly addressed (content hash == filename stem), there is nothing to repair.
	// Without this guard, write+rename would "quarantine" the only copy when newFilePath equals corruptFilePath
	// (e.g. repair invoked after content was fixed but before a mistaken second pass).
	if contentHash == oldNameHash {
		res.Fixed = false
		res.NewFilePath = corruptFilePath
		res.Message = "filename already matches content hash, no repair needed"
		return res, nil
	}

	// Keep bucket placement stable: write corrected file into the same directory as the corrupt file.
	newFilePath := filepath.Join(filepath.Dir(corruptFilePath), contentHash+ext)
	res.NewFilePath = newFilePath

	quarantineDir := options.QuarantineDir
	if quarantineDir == "" {
		quarantineDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir)
	}
	kindQuarantine := filepath.Join(quarantineDir, kind)
	quarantinePath := filepath.Join(kindQuarantine, fmt.Sprintf("%s-%s%s", objectID, oldNameHash, ext))
	res.QuarantinedTo = quarantinePath

	// Dry-run should be cheap: do NOT load CAS index files or perform any writes.
	if options.DryRun {
		res.Message = "[DRY RUN] would write correct hash file, update index, and quarantine corrupt file"
		return res, nil
	}

	// Determine kind directory for index file.
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		return res, errfmt.Errorf("unknown kind: %s", kind)
	}
	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
	cas := filecas.NewContentAddressableStorage(kindDir, kind)

	// Create the correctly-addressed file (safe create; does not overwrite).
	if err := cas.WriteFileWithSync(newFilePath, data); err != nil {
		return res, errfmt.Newf("failed to write repaired CAS file").Wrap(err)
	}

	// Update index synchronously so other processes resolve the object to the correct file.
	if err := cas.GetIndex().SetMapping(objectID, contentHash); err != nil {
		return res, errfmt.Newf("failed to update CAS index mapping").Wrap(err)
	}

	// Quarantine the corrupt original.
	if err := fileutil.MkdirAll(kindQuarantine, paths.DirPerm755); err != nil {
		return res, errfmt.Newf("failed to create quarantine directory").Wrap(err)
	}

	if err := fileutil.Rename(corruptFilePath, quarantinePath); err != nil {
		// If rename fails (e.g., cross-device), fall back to copy+remove.
		if writeErr := fileutil.WriteFile(quarantinePath, data, paths.FilePerm644); writeErr != nil { //nolint:gosec // quarantine file
			return res, errfmt.Errorf("failed to quarantine corrupt file: rename failed: %v, copy failed: %w", err, writeErr)
		}
		if removeErr := fileutil.Remove(corruptFilePath); removeErr != nil && options.Logger != nil {
			options.Logger.Error("failed to remove corrupt file", removeErr)
		}
	}
	res.QuarantinedTo = quarantinePath

	res.Fixed = true
	res.Message = "repaired CAS filename/content hash mismatch"
	if options.Logger != nil {
		options.Logger.Info("cas_corruption_repaired")
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
