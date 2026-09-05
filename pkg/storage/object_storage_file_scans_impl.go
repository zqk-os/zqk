package storage

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func (f *FileObjectStorage) ScanIDBasedFilesRecursive(kindDir, kind string) map[string]string {
	idToPath := make(map[string]string)

	// Check if this kind uses bucketed storage
	if f.usesBucketedStorage(kind, kindDir) {
		// For bucketed storage, scan all date subdirectories
		entries, err := fileutil.ReadDir(kindDir)
		if err != nil {
			if fileutil.IsNotExist(err) {
				return idToPath
			}
			return idToPath
		}

		// Pattern for date directories (YYYY-MM or YYYY-MM-DD)
		datePattern := regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Check if it's a date directory
			if datePattern.MatchString(entry.Name()) {
				dateDir := filepath.Join(kindDir, entry.Name())
				// Recursively scan this date directory
				dateIDToPath := f.ScanIDBasedFilesWithPaths(dateDir, kind)
				for id, path := range dateIDToPath {
					idToPath[id] = path
				}
			}
		}
	} else {
		// For flat storage, scan the kind directory directly
		flatIDToPath := f.ScanIDBasedFilesWithPaths(kindDir, kind)
		for id, path := range flatIDToPath {
			idToPath[id] = path
		}
	}

	return idToPath
}

// scanIDBasedFilesWithPaths scans for ID-based files and returns a map of ID to file path
func (f *FileObjectStorage) ScanIDBasedFilesWithPaths(kindDir, kind string) map[string]string {
	idToPath := make(map[string]string)
	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		return idToPath
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		// Skip hash-based files (64-char hex) - uses package-level compiled regex
		if filecas.CasHashFilenameRe.MatchString(entry.Name()) {
			continue
		}

		// Skip index files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Get full file path
		filePath := filepath.Join(kindDir, entry.Name())

		// Extract ID from filename
		filenameID := strings.TrimSuffix(entry.Name(), ".yaml")
		filenameID = strings.TrimSuffix(filenameID, ".yml")

		// For accounts, handle account-username format
		if kind == objects.KindAccount && strings.HasPrefix(filenameID, "account-") {
			username := strings.TrimPrefix(filenameID, "account-")
			filenameID = fmt.Sprintf("account:%s", username)
		}

		if filenameID != emptyValue {
			idToPath[filenameID] = filePath
		}
	}

	return idToPath
}

func (f *FileObjectStorage) DiscoverObjectKinds() []string {
	var kinds []string
	seen := make(map[string]bool)

	entries, err := fileutil.ReadDir(f.processDir)
	if err != nil {
		return kinds
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		kind := objects.GetKindFromDirectory(dirName)
		if kind != emptyValue && !seen[kind] {
			kinds = append(kinds, kind)
			seen[kind] = true
		}
	}

	return kinds
}
