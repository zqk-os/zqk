package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// runningExecutableFingerprint is the production identity mixed into the
// validation cache checksum. A rebuild (size or mtime change) drops stale
// Layer 1 hits on the next Load. TRACK: PRI-CEF-R26-LIFECYCLE-EXAM-001
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

type cachedFileFingerprint struct {
	modTimeNano int64
	size        int64
	dataHash    []byte
}

var (
	fileFingerprintMu    sync.RWMutex
	fileFingerprintCache = make(map[string]cachedFileFingerprint)
)

// computeValidationCodeChecksum fingerprints the running executable plus
// checker/lifecycle sources so system check reloads when the verdict logic
// changes. Globs are the coverage contract; ValidationCodeChecksumFiles is a
// union for sparse test roots. TRACK: PRI-CEF-R26-LIFECYCLE-EXAM-001
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
		info, err := fileutil.Stat(fullPath)
		if err != nil {
			continue
		}
		anyMaterial = true
		hasher.Write([]byte(relPath))
		hasher.Write([]byte(lineSeparator))

		modNano := info.ModTime().UnixNano()
		size := info.Size()

		fileFingerprintMu.RLock()
		cached, found := fileFingerprintCache[fullPath]
		fileFingerprintMu.RUnlock()

		if found && cached.modTimeNano == modNano && cached.size == size {
			hasher.Write(cached.dataHash)
		} else {
			data, err := fileutil.ReadFile(fullPath)
			if err != nil {
				continue
			}
			h := sha256.Sum256(data)
			fileFingerprintMu.Lock()
			fileFingerprintCache[fullPath] = cachedFileFingerprint{
				modTimeNano: modNano,
				size:        size,
				dataHash:    h[:],
			}
			fileFingerprintMu.Unlock()
			hasher.Write(h[:])
		}
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
