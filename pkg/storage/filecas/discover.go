package filecas

import (
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func DiscoverCASFilePathByScanning(objectID, kindDir string) (filePath, hash string, err error) {
	if objectID == emptyValue || kindDir == emptyValue {
		return "", "", errfmt.Errorf("empty object ID or kind directory")
	}
	found, scanErr := DiscoverCASFilePathsByScanning([]string{objectID}, kindDir)
	if scanErr != nil {
		return "", "", scanErr
	}
	if e, ok := found[objectID]; ok {
		return e.path, e.hash, nil
	}
	return "", "", errfmt.Errorf(ConstStreamIdStrNotFoundInCasDirectoryStr, objectID, kindDir)
}

type casDiscoveredPath struct {
	path string
	hash string
}

// DiscoverCASFilePathsByScanning walks kindDir once and returns path/hash for each requested ID
// found. Prefer this over N×DiscoverCASFilePathByScanning for BatchDelete index misses.
func DiscoverCASFilePathsByScanning(objectIDs []string, kindDir string) (map[string]casDiscoveredPath, error) {
	want := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		if id != emptyValue {
			want[id] = struct{}{}
		}
	}
	out := make(map[string]casDiscoveredPath, len(want))
	if len(want) == 0 || kindDir == emptyValue {
		return out, nil
	}
	scanDir := func(dir string) {
		entries, readErr := fileutil.ReadDir(dir)
		if readErr != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !CasHashFilenameRe.MatchString(name) {
				continue
			}
			path := filepath.Join(dir, name)
			id := CasHashFilePeekObjectID(path)
			if id == emptyValue {
				continue
			}
			if _, ok := want[id]; !ok {
				continue
			}
			if _, already := out[id]; already {
				continue
			}
			out[id] = casDiscoveredPath{
				path: path,
				hash: strings.TrimSuffix(name, filepath.Ext(name)),
			}
			if len(out) == len(want) {
				return
			}
		}
	}
	scanDir(kindDir)
	if len(out) < len(want) {
		entries, err := fileutil.ReadDir(kindDir)
		if err != nil {
			return out, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			scanDir(filepath.Join(kindDir, e.Name()))
			if len(out) == len(want) {
				break
			}
		}
	}
	return out, nil
}
