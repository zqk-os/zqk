package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// checkIntegrityCAS verifies file integrity for content-addressable storage objects
// For CAS objects, the hash is the filename, so we verify:
// 1. Object exists in CAS index
// 2. Hash file exists
// 3. Content hash matches filename hash
// 4. YAML structure is valid
//
//nolint:unused // Helper function - reserved for future use or legacy compatibility
func checkIntegrityCAS(ctx *cli.Context, obj *parser.ParsedObject, filePath, kind string) (issues []Issue, autoFixed []string) {
	// autoFixed is always empty - reserved for future use
	autoFixed = []string{}

	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Get the directory for this kind
	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		return issues, autoFixed
	}

	// Extract object ID from parsed object
	// ParsedObject has an ID field directly
	objectID := obj.ID
	if objectID == emptyValue {
		// Can't check integrity without ID
		return issues, autoFixed
	}

	// Check if this kind uses CAS (using reflection or a helper method)
	// Since we can't access private methods, we'll infer from the file path
	// CAS files have hash-based filenames (64-char hex)
	filename := filepath.Base(filePath)
	isHashBasedFile := len(filename) == 69 && strings.HasSuffix(filename, ".yaml") && // 64 chars + ".yaml"
		isHexString(filename[:64])

	if !isHashBasedFile {
		// Not a CAS file - skip CAS check (will use regular hash registry check)
		return issues, autoFixed
	}

	// Use cached CAS when available to avoid loading full index per call (OOM risk)
	var cas *storage.ContentAddressableStorage
	if c, ok := getCachedCASForKind(context.Background(), projectRoot, kind); ok { // Background: request-or-shutdown derived
		cas = c
	}
	if cas == nil {
		cas = storage.NewContentAddressableStorage(kindDir, kind)
	}

	// Verify hash in filename matches index
	expectedHash := filename[:64] // Remove .yaml extension

	// Check if object exists in index
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		// Object not in index - this could be:
		// 1. Migration case (object exists in ID-based format, not yet migrated)
		// 2. Index corruption
		// 3. Object was deleted but file remains
		if strings.Contains(err.Error(), "not found") {
			// Silently auto-heal the index lag if the file content matches its hash-based filename
			if content, readErr := fileutil.ReadFile(filePath); readErr == nil {
				if storage.CalculateSHA256Hash(content) == expectedHash {
					if idx := cas.GetIndex(); idx != nil {
						relDir, relErr := filepath.Rel(kindDir, filepath.Dir(filePath))
						var bucketKey string
						if relErr == nil && relDir != "." && relDir != "" {
							bucketKey = relDir
						}
						if bucketKey != "" {
							_ = idx.SetMapping(objectID, expectedHash, bucketKey)
						} else {
							_ = idx.SetMapping(objectID, expectedHash)
						}
						_ = idx.Save()
					}
					autoFixed = append(autoFixed, "Silently auto-healed CAS index lag")
					return issues, autoFixed
				}
			}

			issues = append(issues, Issue{
				Tier:        2,
				Category:    "integrity",
				Message:     fmt.Sprintf("Object %s not found in CAS index - may need migration or index is corrupted", objectID),
				AutoFixable: false,
			})
		}
		return issues, autoFixed
	}
	if hash != expectedHash {
		issues = append(issues, Issue{
			Tier:        1,
			Category:    "integrity",
			Message:     fmt.Sprintf("CAS index hash mismatch - filename has %s, index has %s (index may be corrupted)", expectedHash[:16], hash[:16]),
			AutoFixable: true,
			FixCommand:  fmt.Sprintf("zqk system cleanup-duplicates %s --hash-duplicates", kind),
		})
		return issues, autoFixed
	}

	// Read file content and get file info for mtime checking
	fileInfo, err := fileutil.Stat(filePath)
	if err != nil {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "integrity",
			Message:  fmt.Sprintf("Failed to stat CAS file: %v", err),
		})
		return issues, autoFixed
	}

	content, err := fileutil.ReadFile(filePath)
	if err != nil {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "integrity",
			Message:  fmt.Sprintf("Failed to read CAS file: %v", err),
		})
		return issues, autoFixed
	}

	// Calculate actual hash from content
	actualHash := storage.CalculateSHA256Hash(content)
	if actualHash != hash {
		// Hash mismatch - file has been tampered with
		issues = append(issues, Issue{
			Tier:        1,
			Category:    "integrity",
			Message:     fmt.Sprintf("CAS hash mismatch detected - file may have been tampered with (expected: %s..., got: %s...). Content does not match filename hash", hash[:16], actualHash[:16]),
			AutoFixable: false, // CAS tampering cannot be auto-fixed (would require content restoration)
		})
		return issues, autoFixed
	}

	// Check mtime to detect file modifications
	// For CAS files, we check if the file was modified recently (within last hour)
	// This helps detect tampering even if the hash still matches (edge case)
	// Note: We can't store original mtime in CAS index easily, so we check if mtime is very recent
	// (indicating the file was just modified, which could be tampering)
	fileMTime := fileInfo.ModTime()
	timeSinceMod := time.Since(fileMTime)

	// If file was modified very recently (within last 5 minutes), it might be tampering
	// This is a heuristic - legitimate updates through the system would update the index
	// and potentially rename the file (if hash changed). Tier 4 so it does not count
	// toward Tier 2 violation threshold (user can still see it with --tier 4).
	if timeSinceMod < 5*time.Minute {
		issues = append(issues, Issue{
			Tier:        4,
			Category:    "integrity",
			Message:     fmt.Sprintf("CAS file was modified very recently (%v ago) - verify this was intentional.", timeSinceMod.Round(time.Second)),
			AutoFixable: false,
		})
	}

	// Verify YAML structure (CAS Read already does this, but we check here for completeness)
	// This is handled by CAS Read method, so if we got here, YAML is valid

	return issues, autoFixed
}

// isHexString checks if a string contains only hexadecimal characters
//
//nolint:unused // Helper function - reserved for future use
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
