package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// runningExecutableFingerprint is the production identity mixed into the
// validation cache checksum. Normalizes executable path to its canonical family
// and uses build identity rather than transient mtime to prevent mutual cache
// invalidation between CLI binaries (e.g. bin/zqk vs .zqk/bin/zqk-stable)
// built or promoted from the same build.
func runningExecutableFingerprint() string {
	exe, err := fileutil.Executable()
	if err != nil || exe == emptyValue {
		return emptyValue
	}
	return executableFingerprintForPath(exe)
}

func executableFingerprintForPath(exe string) string {
	if exe == emptyValue {
		return emptyValue
	}
	info, err := fileutil.Stat(exe)
	if err != nil {
		return exe
	}
	base := filepath.Base(exe)
	if strings.HasPrefix(base, "zqk") || strings.HasPrefix(base, "zcom") {
		base = "zqk"
	}
	buildID := binaryBuildIdentifier(exe)
	return fmt.Sprintf("%s:%d:%s", base, info.Size(), buildID)
}

func binaryBuildIdentifier(exe string) string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != emptyValue {
				return s.Value
			}
		}
		if bi.Main.Version != emptyValue && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	// Fallback: fast hash of first 64KB of executable (covers binary header + build-id)
	f, err := fileutil.OpenFile(exe, fileutil.O_RDONLY, 0)
	if err == nil {
		defer func() { _ = f.Close() }()
		buf := make([]byte, 64*1024)
		n, _ := f.Read(buf)
		if n > 0 {
			h := sha256.Sum256(buf[:n])
			return hex.EncodeToString(h[:8])
		}
	}
	return "static"
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
