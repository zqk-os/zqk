package storage

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RemoveEmptyBucketDirectories removes empty bucket subdirectories under the kind's directory.
// Used after bulk-deleting audit_events or change_journal_entry so empty date folders (e.g. audit/2026-01-20/) are removed.
// Only removes dirs that contain no .yaml/.yml files; dotfiles (e.g. .index, .hashes) are ignored when deciding emptiness.
// Returns the number of directories removed. Invalidates bucket list cache for the kind so next lookup is fresh.
func (f *FileObjectStorage) RemoveEmptyBucketDirectories(kind string) (removed int, err error) {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return 0, nil
	}
	kindDir := filepath.Join(f.processDir, dirName)
	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bucketDir := filepath.Join(kindDir, entry.Name())
		if !isEmptyBucketDir(bucketDir) {
			continue
		}
		if removeErr := fileutil.Remove(bucketDir); removeErr != nil {
			if !fileutil.IsNotExist(removeErr) {
				return removed, removeErr
			}
		} else {
			removed++
		}
	}
	if removed > 0 {
		f.bucketListCache.Delete(kindDir)
		f.bucketedCache.Delete(kindDir)
	}
	return removed, nil
}

// isEmptyBucketDir reports whether dir contains no object files (.yaml/.yml).
// Dotfiles (e.g. .audit_event.index, .hashes) are ignored so we only remove when no objects remain.
func isEmptyBucketDir(dir string) bool {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
			return false
		}
	}
	return true
}
