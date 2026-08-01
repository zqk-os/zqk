// Package storage: runtime current state for stream-backed kinds (no CAS on update).
// For high-volume/stream-backed kinds, updates are persisted as change journal entries plus a
// single "current state" file per object under .zqk/state/stream_current/<kind>/<id>.yaml.
// This avoids CAS hash compute and index overhead for routine touches (e.g. zqk_session updated_at).
// See docs/architecture/HIGH_VOLUME_STORAGE_DEPRECATION.md and ZQK_SESSION_STREAM_BEHAVIOR.md.

package storage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// streamCurrentPath returns the file path for a stream-backed object's current state overlay.
// ID is sanitized for use as filename (colons and slashes replaced with underscore).
// Uses [datacell.CellStreamOverlayKindDir] so layout matches data-cell primary_path / membrane read paths.
func streamCurrentPath(projectRoot, kind, id string) string {
	safeID := strings.ReplaceAll(id, ":", "_")
	safeID = strings.ReplaceAll(safeID, "/", "_")
	return filepath.Join(datacell.CellStreamOverlayKindDir(projectRoot, kind), safeID+".yaml")
}

// WriteStreamBackedCurrentState writes the current state for a stream-backed object (runtime delta).
// Overwrites any existing file; no CAS, no hash. Used by applyUpdateFromBuffer for stream-backed kinds.
func WriteStreamBackedCurrentState(projectRoot, kind, id string, data []byte) error {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue || len(data) == 0 {
		return nil
	}
	p := streamCurrentPath(projectRoot, kind, id)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}
	return os.WriteFile(p, data, paths.FilePerm600)
}

// ReadStreamBackedCurrentState returns the current state bytes for a stream-backed object if the overlay file exists.
// Returns (nil, false) when the file does not exist (caller should use stream base).
func ReadStreamBackedCurrentState(projectRoot, kind, id string) ([]byte, bool) {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return nil, false
	}
	p := streamCurrentPath(projectRoot, kind, id)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return data, true
}

// StreamCurrentPathExists returns true when the stream_current overlay file exists for this object.
func StreamCurrentPathExists(projectRoot, kind, id string) bool {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return false
	}
	p := streamCurrentPath(projectRoot, kind, id)
	_, err := os.Stat(p)
	return err == nil
}

// GetStreamBackedObjectFilePath returns the path to use for reading: stream_current overlay if present, else "".
// Caller uses empty to mean "use stream location". Returns the overlay file path when it exists.
func GetStreamBackedObjectFilePath(projectRoot, kind, id string) string {
	if !StreamCurrentPathExists(projectRoot, kind, id) {
		return ""
	}
	return streamCurrentPath(projectRoot, kind, id)
}

// IsStreamCurrentPath returns true when the path is under stream_current (runtime delta file, not CAS).
// Uses path segment only so it works when projectRoot is not available at read time.
func IsStreamCurrentPath(projectRoot, path string) bool {
	norm := filepath.ToSlash(path)
	return strings.Contains(norm, "/"+paths.StreamCurrentSubdir+"/")
}

// RemoveStreamBackedCurrentState removes the current-state overlay (e.g. on object delete).
func RemoveStreamBackedCurrentState(projectRoot, kind, id string) error {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return nil
	}
	p := streamCurrentPath(projectRoot, kind, id)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ParseStreamBackedCurrentState unmarshals YAML from stream_current overlay; returns nil on error.
func ParseStreamBackedCurrentState(data []byte) (map[string]any, error) {
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}
