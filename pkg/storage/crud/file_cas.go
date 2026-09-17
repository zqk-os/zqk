package crud

import (
	"path/filepath"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

type ContentAddressableStorage = filecas.ContentAddressableStorage

type IDIndex = filecas.IDIndex

type IndexWriteQueue = filecas.IndexWriteQueue

func RemoveOrphanCASFilesForObjectID(objectID, kindDir string) {
	if objectID == "" || kindDir == "" {
		return
	}
	RemoveOrphanCASFilesForObjectIDs(map[string]struct{}{objectID: {}}, kindDir)
}

func RemoveOrphanCASFilesForObjectIDs(want map[string]struct{}, kindDir string) {
	if len(want) == 0 || kindDir == "" {
		return
	}
	removeMatchesInDir := func(dir string) {
		entries, readErr := fileutil.ReadDir(dir)
		if readErr != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !filecas.CasHashFilenameRe.MatchString(name) {
				continue
			}
			path := filepath.Join(dir, name)
			id := filecas.CasHashFilePeekObjectID(path)
			if id == "" {
				continue
			}
			if _, ok := want[id]; !ok {
				continue
			}
			var _err_83295028 = fileutil.Remove(path)
			if _err_83295028 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("swallowed error", _err_83295028).Log()
			}
		}
	}
	removeMatchesInDir(kindDir)
	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		removeMatchesInDir(filepath.Join(kindDir, e.Name()))
	}
}

func NewContentAddressableStorage(kindDir, kind string, writeQueue ...IndexWriteQueue) *ContentAddressableStorage {
	var q []filecas.IndexWriteQueue
	for _, wq := range writeQueue {
		q = append(q, wq)
	}
	return filecas.NewContentAddressableStorage(kindDir, kind, q...)
}

func NewContentAddressableStorageWithIndex(idx *IDIndex) *ContentAddressableStorage {
	return filecas.NewContentAddressableStorageWithIndex(idx)
}

func SetSkipIndexUpdateWait(skip bool) {
	filecas.SetSkipIndexUpdateWait(skip)
}
