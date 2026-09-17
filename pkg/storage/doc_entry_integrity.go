package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Constants for DocIntegrity validation codes
const (
	DocIntegrityCodeTargetMissing     = "DOC_TARGET_MISSING"
	DocIntegrityCodeTargetUnreadable  = "DOC_TARGET_UNREADABLE"
	DocIntegrityCodeHashMismatch      = "DOC_HASH_MISMATCH"
	DocIntegrityCodeSizeMismatch      = "DOC_SIZE_MISMATCH"
	DocIntegrityCodeUnsealedPublished = "DOC_UNSEALED_PUBLISHED"
)

// DocIntegrityViolation captures a cryptographic leash or drift violation on a doc_entry
type DocIntegrityViolation struct {
	ObjectID     string `json:"object_id"`
	Path         string `json:"path"`
	Code         string `json:"code"`
	Severity     string `json:"severity"` // "error", "warning"
	Message      string `json:"message"`
	ExpectedHash string `json:"expected_hash,omitempty"`
	ActualHash   string `json:"actual_hash,omitempty"`
	ExpectedSize int64  `json:"expected_size,omitempty"`
	ActualSize   int64  `json:"actual_size,omitempty"`
}

// ComputeFileIntegrity calculates the SHA-256 hash and byte size of a target file.
func ComputeFileIntegrity(filePath string) (hash string, size int64, err error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read file for integrity check: %w", err)
	}

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), int64(len(data)), nil
}

// ValidateDocEntryIntegrity verifies that a doc_entry's referenced file is reachable,
// uncorrupted, matches its cryptographic leash, and satisfies lifecycle publication rules.
func ValidateDocEntryIntegrity(projectRoot string, docObj map[string]any) []DocIntegrityViolation {
	var violations []DocIntegrityViolation

	id, _ := docObj[objects.FieldKeyID].(string)
	rawPath, _ := docObj[objects.FieldKeyPath].(string)
	status, _ := docObj[objects.FieldKeyStatus].(string)

	expectedHash, _ := docObj["content_hash"].(string)
	var expectedSize int64
	switch v := docObj["content_size"].(type) {
	case int:
		expectedSize = int64(v)
	case int64:
		expectedSize = v
	case float64:
		expectedSize = int64(v)
	}

	if rawPath == "" {
		violations = append(violations, DocIntegrityViolation{
			ObjectID: id,
			Code:     DocIntegrityCodeTargetMissing,
			Severity: "error",
			Message:  "doc_entry path is empty",
		})
		return violations
	}

	// Resolve the target file path
	resolvedPath, err := paths.ResolveDocEntryPath(projectRoot, rawPath)
	if err != nil || resolvedPath == "" {
		// Fallback to direct path normalization
		normRel := paths.NormalizeDocEntryPathForKey(rawPath)
		resolvedPath = filepath.Join(projectRoot, normRel)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			violations = append(violations, DocIntegrityViolation{
				ObjectID: id,
				Path:     rawPath,
				Code:     DocIntegrityCodeTargetMissing,
				Severity: "error",
				Message:  fmt.Sprintf("target document file not found at %s", resolvedPath),
			})
		} else {
			violations = append(violations, DocIntegrityViolation{
				ObjectID: id,
				Path:     rawPath,
				Code:     DocIntegrityCodeTargetUnreadable,
				Severity: "error",
				Message:  fmt.Sprintf("target document file unreadable at %s: %v", resolvedPath, err),
			})
		}
		return violations
	}

	if info.IsDir() {
		violations = append(violations, DocIntegrityViolation{
			ObjectID: id,
			Path:     rawPath,
			Code:     DocIntegrityCodeTargetUnreadable,
			Severity: "error",
			Message:  fmt.Sprintf("target document path is a directory: %s", resolvedPath),
		})
		return violations
	}

	// Target exists, compute cryptographic hash and size
	actualHash, actualSize, err := ComputeFileIntegrity(resolvedPath)
	if err != nil {
		violations = append(violations, DocIntegrityViolation{
			ObjectID: id,
			Path:     rawPath,
			Code:     DocIntegrityCodeTargetUnreadable,
			Severity: "error",
			Message:  fmt.Sprintf("failed to compute integrity hash for %s: %v", resolvedPath, err),
		})
		return violations
	}

	// Check published/active status requirements
	isPublishedOrActive := strings.EqualFold(status, "published") || strings.EqualFold(status, "active")
	if isPublishedOrActive && expectedHash == "" {
		violations = append(violations, DocIntegrityViolation{
			ObjectID: id,
			Path:     rawPath,
			Code:     DocIntegrityCodeUnsealedPublished,
			Severity: "error",
			Message:  "doc_entry is in published/active status but lacks cryptographic content_hash",
		})
	}

	// Validate hash if expected is provided
	if expectedHash != "" && !strings.EqualFold(expectedHash, actualHash) {
		violations = append(violations, DocIntegrityViolation{
			ObjectID:     id,
			Path:         rawPath,
			Code:         DocIntegrityCodeHashMismatch,
			Severity:     "error",
			Message:      fmt.Sprintf("cryptographic drift detected: expected SHA-256 %s, got %s", expectedHash, actualHash),
			ExpectedHash: expectedHash,
			ActualHash:   actualHash,
		})
	}

	// Validate size if expected is provided (>0)
	if expectedSize > 0 && expectedSize != actualSize {
		violations = append(violations, DocIntegrityViolation{
			ObjectID:     id,
			Path:         rawPath,
			Code:         DocIntegrityCodeSizeMismatch,
			Severity:     "warning",
			Message:      fmt.Sprintf("document size drift detected: expected %d bytes, got %d bytes", expectedSize, actualSize),
			ExpectedSize: expectedSize,
			ActualSize:   actualSize,
		})
	}

	return violations
}
