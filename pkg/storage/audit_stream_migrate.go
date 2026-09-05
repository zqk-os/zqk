// Package storage: one-time migration of audit_event stream from legacy path to canonical path.
// Canonical: .zqk/streams/audit_event/ with segment files YYYY-MM-DD_stream.json.
// Legacy: .zqk/audit_streams/ with segment files audit_stream_YYYY-MM-DD.jsonl.
package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	// Legacy filename pattern: audit_stream_YYYY-MM-DD.jsonl
	legacyAuditStreamFilenamePrefix = "audit_stream_"
	legacyAuditStreamFilenameSuffix = ".jsonl"
)

// MigrateAuditStreamToCanonicalLocation moves segment files from the legacy .zqk/audit_streams/
// to the canonical .zqk/streams/audit_event/ and rewrites the stream registry so all "loc"
// entries point to the new paths. Idempotent: if legacy dir is missing or empty, no-op.
// Call after path-cache is built so future writes use canonical path.
func MigrateAuditStreamToCanonicalLocation(projectRoot string) (moved int, registryUpdated bool, err error) {
	if projectRoot == emptyValue {
		return 0, false, nil
	}

	legacyDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AuditStreamsDir)
	canonicalDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, auditEventKind)

	entries, err := fileutil.ReadDir(legacyDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, errfmt.Newf(ConstAuditMigrateAuditStreamReadLegacyDir).Wrap(err)
	}

	// Build mapping: legacy absolute path -> canonical absolute path (for registry rewrite).
	pathMap := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, legacyAuditStreamFilenamePrefix) || !strings.HasSuffix(name, legacyAuditStreamFilenameSuffix) {
			continue
		}
		// audit_stream_2026-03-03.jsonl -> 2026-03-03_stream.json
		date := strings.TrimSuffix(strings.TrimPrefix(name, legacyAuditStreamFilenamePrefix), legacyAuditStreamFilenameSuffix)
		if len(date) != 10 {
			continue
		}
		legacyPath := filepath.Join(legacyDir, name)
		canonicalName := date + streamFileSuffix
		canonicalPath := filepath.Join(canonicalDir, canonicalName)
		pathMap[legacyPath] = canonicalPath
	}

	if len(pathMap) == 0 {
		return 0, false, nil
	}

	if err := fileutil.EnsureDir(canonicalDir); err != nil {
		return 0, false, errfmt.Newf(ConstAuditMigrateAuditStreamMkdirCanonical).Wrap(err)
	}

	for legacyPath, canonicalPath := range pathMap {
		if err := fileutil.Rename(legacyPath, canonicalPath); err != nil {
			// Fallback: copy then remove (e.g. cross-device)
			if copyErr := copyFile(legacyPath, canonicalPath); copyErr != nil {
				return moved, false, errfmt.Errorf(ConstAuditMigrateAuditStreamMove, legacyPath, err)
			}
			if rErr := fileutil.Remove(legacyPath); rErr != nil && !fileutil.IsNotExist(rErr) {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstAuditFailedToRemoveLegacyAuditStreamFileBackslashn, legacyPath, rErr), nil).Log()
			}
		}
		moved++
	}

	// Rewrite stream registry: replace old paths with new paths in "loc" field.
	registryPath := streamRegistryPath(projectRoot, auditEventKind)
	updated, err := rewriteStreamRegistryPaths(registryPath, pathMap)
	if err != nil {
		return moved, false, errfmt.Newf(ConstAuditMigrateAuditStreamRewriteRegistry).Wrap(err)
	}
	if updated {
		registryUpdated = true
		invalidateStreamRegistryCache(projectRoot, auditEventKind)
	}

	return moved, registryUpdated, nil
}

func copyFile(src, dst string) error {
	data, err := fileutil.ReadFile(src)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(dst, data)
}

// rewriteStreamRegistryPaths reads the registry file line-by-line, rewrites each "loc"
// using pathMap (old path -> new path), and writes to a temp file then renames.
// Returns true if any line was changed.
func rewriteStreamRegistryPaths(registryPath string, pathMap map[string]string) (bool, error) {
	f, err := fileutil.Open(registryPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	tmpPath := registryPath + SuffixTempFile
	tmp, err := fileutil.Create(tmpPath)
	if err != nil {
		return false, err
	}
	defer tmp.Close()

	sc := bufio.NewScanner(f)
	var changed bool
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == emptyValue {
			continue
		}
		var row map[string]string
		if json.Unmarshal([]byte(line), &row) != nil || row[FieldKeyLoc] == emptyValue {
			if _, wErr := tmp.Write(append([]byte(line), '\n')); wErr != nil {
				return false, wErr
			}
			continue
		}
		loc := row[FieldKeyLoc]
		before, after, ok := splitLocPathOffset(loc)
		if !ok {
			if _, wErr := tmp.Write(append([]byte(line), '\n')); wErr != nil {
				return false, wErr
			}
			continue
		}
		if newPath, ok := pathMap[before]; ok {
			row[FieldKeyLoc] = newPath + SeparatorPathOffset + after
			changed = true
			newLine, err := json.Marshal(row)
			if err != nil {
				return false, errfmt.Newf(ConstAuditMarshalRowDuringMigration).Wrap(err)
			}
			if _, wErr := tmp.Write(append(newLine, '\n')); wErr != nil {
				return false, wErr
			}
		} else {
			if _, wErr := tmp.Write(append([]byte(line), '\n')); wErr != nil {
				return false, wErr
			}
		}
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if !changed {
		if rErr := fileutil.Remove(tmpPath); rErr != nil && !fileutil.IsNotExist(rErr) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstAuditFailedToRemoveTempRegistryFileBackslashn, tmpPath, rErr), nil).Log()
		}
		return false, nil
	}
	if err := fileutil.Rename(tmpPath, registryPath); err != nil {
		return false, err
	}
	return true, nil
}

var locPathOffsetRe = regexp.MustCompile(`^(.+)` + SeparatorPathOffset + `(\d+)$`)

func splitLocPathOffset(loc string) (path, offset string, ok bool) {
	matches := locPathOffsetRe.FindStringSubmatch(loc)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}
