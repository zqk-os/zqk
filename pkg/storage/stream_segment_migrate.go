// Package storage: in-place migration of stream segment filenames from legacy to canonical.
// Legacy: <kind>_YYYY-MM-DD.jsonl under .zqk/streams/<kind>/.
// Canonical: YYYY-MM-DD_stream.json (same dir). Registry "loc" entries are rewritten to new paths.
package storage

import (
	"os"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

const (
	// Legacy segment filename pattern: <kind>_YYYY-MM-DD.jsonl
	legacySegmentDateLen = 10 // len("YYYY-MM-DD")
)

// MigrateStreamSegmentFilenamesToCanonical renames segment files in each .zqk/streams/<kind>/
// from <kind>_YYYY-MM-DD.jsonl to YYYY-MM-DD_stream.json and rewrites the stream registry
// so "loc" entries point to the new paths. Idempotent: skips dirs with no legacy-named files.
func MigrateStreamSegmentFilenamesToCanonical(projectRoot string) (moved int, kindsUpdated []string, err error) {
	if projectRoot == emptyValue {
		return 0, nil, nil
	}
	streamsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir)
	entries, err := os.ReadDir(streamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, errfmt.Newf(ConstStreamMigrateStreamSegmentsReadStreamsDir).Wrap(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		kind := e.Name()
		kindDir := filepath.Join(streamsDir, kind)
		pathMap := buildLegacyToCanonicalPathMap(kindDir, kind)
		if len(pathMap) == 0 {
			continue
		}
		for oldPath, newPath := range pathMap {
			if renameErr := os.Rename(oldPath, newPath); renameErr != nil {
				if copyErr := copyFile(oldPath, newPath); copyErr != nil {
					return moved, kindsUpdated, errfmt.Errorf(ConstStreamMigrateStreamSegmentsMoveStrErr, oldPath, renameErr)
				}
				var _err_84204490 = os.Remove(oldPath)
				if _err_84204490 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84204490).Log()
				}
			}
			moved++
		}
		registryPath := streamRegistryPath(projectRoot, kind)
		updated, rewriteErr := rewriteStreamRegistryPaths(registryPath, pathMap)
		if rewriteErr != nil {
			return moved, kindsUpdated, errfmt.Errorf(ConstStreamMigrateStreamSegmentsRewriteRegistryStrErr, kind, rewriteErr)
		}
		if updated {
			invalidateStreamRegistryCache(projectRoot, kind)
			kindsUpdated = append(kindsUpdated, kind)
		}
	}
	return moved, kindsUpdated, nil
}

// buildLegacyToCanonicalPathMap finds files in kindDir named <kind>_YYYY-MM-DD.jsonl
// and returns a map from absolute legacy path to absolute canonical path (YYYY-MM-DD_stream.json).
func buildLegacyToCanonicalPathMap(kindDir, kind string) map[string]string {
	pathMap := make(map[string]string)
	legacyPrefix := kind + "_"
	segmentEntries, err := os.ReadDir(kindDir)
	if err != nil {
		return pathMap
	}
	for _, se := range segmentEntries {
		if se.IsDir() {
			continue
		}
		name := se.Name()
		if !strings.HasPrefix(name, legacyPrefix) || !strings.HasSuffix(name, legacyStreamFileSuffix) {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(name, legacyPrefix), legacyStreamFileSuffix)
		if len(date) != legacySegmentDateLen {
			continue
		}
		legacyPath := filepath.Join(kindDir, name)
		canonicalName := date + streamFileSuffix
		canonicalPath := filepath.Join(kindDir, canonicalName)
		if _, err := os.Stat(canonicalPath); err == nil {
			continue // already canonical; skip to avoid overwrite
		}
		pathMap[legacyPath] = canonicalPath
	}
	return pathMap
}
