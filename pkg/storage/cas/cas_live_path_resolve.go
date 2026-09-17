package cas

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ResolveLiveCASFilePath returns the on-disk hash YAML for objectID when the CAS
// listing index maps to an existing blob. Used when the object-id-cache still
// points at a deleted hash after update/promote/delete (misreported as "stale
// index entry" by system check).
//
// TRACK: BLI-REDACTED — remove when: object-id-cache always
// receives the new CAS path on write and drops entries on delete before check.
func ResolveLiveCASFilePath(projectRoot, kind, objectID string) (string, bool) {
	if projectRoot == "" || kind == "" || objectID == "" {
		return "", false
	}
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		return "", false
	}
	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)
	return ResolveLiveCASFilePathFromKindDir(kindDir, kind, objectID)
}

// ResolveLiveCASFilePathFromKindDir is ResolveLiveCASFilePath when the kind
// directory is already known (object-id-cache cleanup path).
func ResolveLiveCASFilePathFromKindDir(kindDir, kind, objectID string) (string, bool) {
	if kindDir == "" || kind == "" || objectID == "" {
		return "", false
	}
	cas := filecas.NewContentAddressableStorage(kindDir, kind)
	if cas == nil || cas.GetIndex() == nil {
		return "", false
	}
	path, err := cas.GetFilePathForID(objectID)
	if err != nil || path == "" {
		return "", false
	}
	if _, statErr := fileutil.Stat(path); statErr != nil {
		return "", false
	}
	return path, true
}

// CachePathNeedsCASResolve reports whether a cached file path is missing on disk
// (typical after CAS hash rename left the object-id-cache on the deleted blob).
func CachePathNeedsCASResolve(filePath string) bool {
	if filePath == "" {
		return true
	}
	_, err := fileutil.Stat(filePath)
	return err != nil
}
