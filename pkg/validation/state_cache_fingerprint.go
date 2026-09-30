package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// runningExecutableFingerprint is the production identity mixed into the
// validation cache checksum. A rebuild (size or mtime change) drops stale
// Layer 1 hits on the next Load.
func runningExecutableFingerprint() string {
	exe, err := fileutil.Executable()
	if err != nil || exe == emptyValue {
		return emptyValue
	}
	info, err := fileutil.Stat(exe)
	if err != nil {
		return exe
	}
	return fmt.Sprintf("%s:%d:%d", exe, info.Size(), info.ModTime().UnixNano())
}

var fileContentHashes stampmemo.Table[[]byte] // keyed by checker/lifecycle path (closed glob set)

// computeValidationCodeChecksum fingerprints the running executable plus
// checker/lifecycle sources so system check reloads when the verdict logic
// changes. Globs are the coverage contract; ValidationCodeChecksumFiles is a
// union for sparse test roots.
func computeValidationCodeChecksum(projectRoot string) string {
	hasher := sha256.New()
	anyMaterial := false

	exe := runningExecutableFingerprint()
	if exe != emptyValue {
		anyMaterial = true
		hasher.Write([]byte("exe:"))
		hasher.Write([]byte(exe))
		hasher.Write([]byte(lineSeparator))
	}

	for _, relPath := range validationFingerprintRelPaths(projectRoot) {
		fullPath := filepath.Join(projectRoot, filepath.FromSlash(relPath))
		stamp := stampmemo.Of(fullPath)
		if stamp == 0 {
			continue
		}
		anyMaterial = true
		hasher.Write([]byte(relPath))
		hasher.Write([]byte(lineSeparator))

		sum, err := fileContentHashes.Load(fullPath, stamp, func() ([]byte, error) {
			data, err := fileutil.ReadFile(fullPath)
			if err != nil {
				return nil, err
			}
			h := sha256.Sum256(data)
			return h[:], nil
		})
		if err != nil {
			continue
		}
		hasher.Write(sum)
		hasher.Write([]byte(lineSeparator))
	}

	if !anyMaterial {
		return NoSourceCodeAvailableChecksum
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func validationFingerprintRelPaths(projectRoot string) []string {
	seen := make(map[string]struct{})
	for _, p := range ValidationCodeChecksumFiles {
		if p != emptyValue {
			seen[filepath.ToSlash(p)] = struct{}{}
		}
	}
	if projectRoot == emptyValue {
		return sortedRelPaths(seen)
	}
	for _, pattern := range ValidationCodeChecksumGlobs {
		matches, err := filepath.Glob(filepath.Join(projectRoot, filepath.FromSlash(pattern)))
		if err != nil {
			continue
		}
		for _, full := range matches {
			base := filepath.Base(full)
			if appledouble.IsSidecarFileName(base) {
				continue
			}
			if strings.HasSuffix(base, "_test.go") {
				continue
			}
			rel, err := filepath.Rel(projectRoot, full)
			if err != nil {
				continue
			}
			seen[filepath.ToSlash(rel)] = struct{}{}
		}
	}
	return sortedRelPaths(seen)
}

func sortedRelPaths(seen map[string]struct{}) []string {
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
