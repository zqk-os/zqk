package crud

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var ErrObjectNotFound = errfmt.Errorf("object not found")

// NormalizeCASIDPrefix ensures kind prefixes end with "-" so CAS ids stay
// PREFIX-timestamp-random (e.g. TST-1785… not TST1785…). Config entries must
// include the hyphen; this guard prevents silent corruption if one is omitted.
func NormalizeCASIDPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || strings.HasSuffix(prefix, "-") {
		return prefix
	}
	return prefix + "-"
}

// IsTestOrTempProjectRoot returns true if projectRoot is likely a test or temp directory
// (e.g. os.TempDir() or a path containing "-test-"). Used to avoid logging at Warn when
// tests run against global singletons that are already bound to the real project root.
// Callers outside storage (e.g. check command) use this to pick test-scoped storage without
// write-behind/WAL so t.TempDir cleanup does not race with background workers.
func IsTestOrTempProjectRoot(projectRoot string) bool {
	if projectRoot == emptyValue {
		return false
	}
	clean := filepath.Clean(projectRoot)
	if strings.Contains(clean, "-test-") {
		return true
	}
	tmpDir := filepath.Clean(fileutil.TempDir())
	if tmpDir != emptyValue && (clean == tmpDir || strings.HasPrefix(clean+string(filepath.Separator), tmpDir+string(filepath.Separator))) {
		return true
	}
	return false
}

// ============================================================================
// Permissions
// ============================================================================

// ============================================================================
// Metadata
// ============================================================================

func ValidateKindDirectoryName(kind, dirName string) error {
	clean := filepath.Clean(dirName)
	if clean == "." || clean == ".." || filepath.IsAbs(dirName) ||
		strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return errfmt.Errorf("invalid dir mapping for kind %s: %s", kind, dirName)
	}
	return nil
}

// ============================================================================
// Hash Operations
// ============================================================================

var casLookupTable = func() [256]byte {
	var tbl [256]byte
	for i := 0; i < 256; i++ {
		tbl[i] = byte(i)
	}
	return tbl
}()

func sanitizeCASContent(content []byte) []byte {
	if len(content) == 0 {
		return content
	}
	clean := make([]byte, len(content))
	for i, b := range content {
		clean[i] = casLookupTable[b]
	}
	return clean
}

// CalculateSHA256Hash calculates SHA256 hash of content
// This is the shared utility function for all hash calculations to ensure consistency
// and prevent variance from duplicate implementations
func CalculateSHA256Hash(content []byte) string {
	clean := sanitizeCASContent(content)
	hash := sha256.Sum256(clean)
	return hex.EncodeToString(hash[:])
}

// VerifyContentHash verifies that content hashes to expectedHash.
// Used by all CAS read paths (FileObjectStorage discovery, ContentAddressableStorage.Read, Move).
// Returns an error only on mismatch (integrity failure).
func VerifyContentHash(content []byte, expectedHash string) error {
	actual := CalculateSHA256Hash(content)
	if actual != expectedHash {
		return errfmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actual)
	}
	return nil
}

// IsHashRegistrySaveQueueFull reports whether err is due to the hash registry save queue being full (backpressure).
// Such failures are not user-actionable (creates/updates are rolled back and may replay from WAL); log at Warn, not Error.
func IsHashRegistrySaveQueueFull(err error) bool {
	return err != nil && strings.Contains(err.Error(), "save queue is full")
}

// ============================================================================
// File I/O Operations
// ============================================================================

// VerifyEmbeddedChecksum verifies the sha256_checksum if present in the object
func VerifyEmbeddedChecksum(data []byte, obj map[string]any) error {
	if obj == nil {
		return nil
	}
	checksumVal, ok := obj["sha256_checksum"]
	if !ok {
		return nil
	}
	expectedChecksum, ok := checksumVal.(string)
	if !ok || expectedChecksum == "" {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "sha256_checksum:") {
			filtered = append(filtered, line)
		}
	}
	filteredData := []byte(strings.Join(filtered, "\n"))
	actualChecksum := CalculateSHA256Hash(filteredData)
	if actualChecksum != expectedChecksum {
		return errfmt.Errorf("integrity verification failed: expected checksum %s, got %s", expectedChecksum, actualChecksum)
	}
	return nil
}

// ============================================================================
// Keystore Operations
// ============================================================================

// ============================================================================
// Tracking
// ============================================================================

// ============================================================================
// Utilities
// ============================================================================

// IsExpectedMissingErr checks if an error is an expected "not found" error,
// which is common during cleanup or cache invalidation and can be safely ignored
// to reduce log noise without breaking the fail-fast mandate.
func IsExpectedMissingErr(err error) bool {
	if err == nil {
		return false
	}
	if fileutil.IsNotExist(err) || err == ErrObjectNotFound {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "no such file or directory") ||
		strings.Contains(errStr, "object not found") ||
		strings.Contains(errStr, "not in stream") ||
		strings.Contains(errStr, "no such file or directory") ||
		strings.Contains(errStr, "stale cas index") ||
		strings.Contains(errStr, "failed to read file")
}
