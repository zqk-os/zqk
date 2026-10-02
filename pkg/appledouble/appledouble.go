// Package appledouble identifies macOS AppleDouble / resource-fork sidecar files (names starting with "._").
// These are not valid YAML/spec payloads but often appear next to real files after Finder copy or bootstrap.
package appledouble

import (
	"path/filepath"
	"strings"
)

// SidecarPrefix is the filename prefix for AppleDouble sidecar entries.
const SidecarPrefix = "._"

// IsSidecarFileName reports whether name (typically filepath.Base(path)) is a sidecar file.
func IsSidecarFileName(name string) bool {
	return strings.HasPrefix(filepath.Base(name), SidecarPrefix)
}

// PathHasSidecarSegment reports whether any path segment starts with SidecarPrefix
// (e.g. cli_specs/system/._x.yaml or ._dir/file.yaml).
func PathHasSidecarSegment(path string) bool {
	path = filepath.ToSlash(path)
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, SidecarPrefix) {
			return true
		}
	}
	return false
}

// SkipPathInTreeWalk is the policy used across filepath.Walk / WalkDir scans for specs, YAML,
// and codegen: ignore any path that contains an AppleDouble sidecar segment. It is equivalent
// to PathHasSidecarSegment; prefer this name at call sites so the reason for skipping is obvious.
func SkipPathInTreeWalk(path string) bool {
	return PathHasSidecarSegment(path)
}

// SkipNameInReadDir is the policy used across os.ReadDir (flat directory) scans: ignore
// sidecar filenames. It is equivalent to IsSidecarFileName; prefer this name at call sites
// so directory-listing skips are clearly about AppleDouble junk, not generic filtering.
func SkipNameInReadDir(name string) bool {
	return IsSidecarFileName(name)
}

// SkipDirOrSidecar reports whether a tree walk entry should be skipped (if it is a directory or AppleDouble sidecar).
func SkipDirOrSidecar(isDir bool, path string) bool {
	return isDir || SkipPathInTreeWalk(path)
}
