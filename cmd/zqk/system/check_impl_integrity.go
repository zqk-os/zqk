package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

func checkIntegrity(ctx *cli.Context, cmd *cobra.Command, _ *parser.ParsedObject, filePath, kind string) (issues []Issue, autoFixed []string) {
	autoFixed = []string{} // Initialize to empty slice (reserved for future use)

	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Get the directory for this kind
	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		return issues, autoFixed
	}

	// Load HashRegistry (use system context when cmd has no context, e.g. in tests)
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = pkgctx.NewSystemContext()
	}
	registry := storage.NewHashRegistry(parentCtx, kind, kindDir)
	if err := registry.Load(); err != nil {
		// Registry doesn't exist yet - not an error, just no integrity check
		return issues, autoFixed
	}

	// Read file content
	content, err := fileutil.ReadFile(filePath)
	if err != nil {
		issues = append(issues, Issue{
			Tier:     1,
			Category: "integrity",
			Message:  fmt.Sprintf("Failed to read file: %v", err),
		})
		return issues, autoFixed
	}

	// Calculate hash
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// Get filename relative to kind directory
	filename := filepath.Base(filePath)

	// Check hash
	expectedHash := registry.GetHash(filename)
	if expectedHash == emptyValue {
		// No hash recorded - check if file has been unhashed for too long (>30 seconds)
		// This indicates direct file manipulation outside CLI context
		fileInfo, err := fileutil.Stat(filePath)
		var ageMessage string
		if err == nil {
			age := time.Since(fileInfo.ModTime())
			if age > 30*time.Second {
				ageMessage = fmt.Sprintf(" (file modified %v ago - indicates direct file manipulation outside CLI)", age.Round(time.Second))
			}
		}

		// Tier 4 (recommendation) so it does not count as a violation; auto-fix still applies
		message := "No integrity hash recorded for this file" + ageMessage
		issues = append(issues, Issue{
			Tier:        4,
			Category:    "integrity",
			Message:     message,
			AutoFixable: true,
		})
	} else if expectedHash != hashStr {
		// Hash mismatch - can be auto-fixed if --auto-fix is enabled
		// If --force is also set, it will create an audit event
		issues = append(issues, Issue{
			Tier:        1,
			Category:    "integrity",
			Message:     fmt.Sprintf("Hash mismatch detected - file may have been tampered with (expected: %s..., got: %s...). Use --auto-fix to regenerate hash", expectedHash[:16], hashStr[:16]),
			AutoFixable: true, // Can be auto-fixed with --auto-fix flag
		})
	}

	return issues, autoFixed
}

// checkIntegrityWithRegistry verifies file integrity using a pre-loaded HashRegistry (cached version)
//
//nolint:unused // Replaced by checkIntegrityWithRegistryAndContent - reserved for legacy compatibility
func checkIntegrityWithRegistry(ctx *cli.Context, obj *parser.ParsedObject, filePath, kind string, registry storage.HashRegistryProvider) (issues []Issue, autoFixed []string) {
	// Read file content
	content, err := fileutil.ReadFile(filePath)
	if err != nil {
		return []Issue{
			{
				Tier:     1,
				Category: "integrity",
				Message:  fmt.Sprintf("Failed to read file: %v", err),
			},
		}, nil
	}
	return checkIntegrityWithRegistryAndContent(ctx, obj, filePath, kind, content, registry, nil)
}

// checkIntegrityWithRegistryAndContent verifies file integrity using pre-read content (optimized version)
// This function is called LAST in the check sequence to ensure all edits have completed and hashes have been computed
// CRITICAL: All kinds now use CAS - HashRegistry is deprecated. This function now uses CAS index exclusively.
// When storageProvider is non-nil and is *storage.FileObjectStorage, uses cached CAS (one index per kind) to avoid
// loading the full index per validation and OOM (see daemon memory sampling: loadLocked + json.Unmarshal hot path).
func checkIntegrityWithRegistryAndContent(ctx *cli.Context, obj *parser.ParsedObject, filePath, kind string, content []byte, _ storage.HashRegistryProvider, storageProvider storage.ObjectStorageProvider) (issues []Issue, autoFixed []string) {
	if ctx != nil {
		root := ProjectRootOrResolve(ctx.ProjectRoot)
		if storage.IsObjectDraftPlanePath(root, filePath) {
			// Draft-plane YAML is id-keyed and intentionally absent from the CAS index.
			return issues, autoFixed
		}
	}
	// Calculate hash from pre-read content
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// Detect hash-based CAS filenames: <64-hex>.yaml (or .yml)
	// This lets us distinguish:
	// - real tampering/corruption: filename-hash != content-hash
	// - internal index drift: CAS index hash != filename-hash (but filename-hash == content-hash)
	filename := filepath.Base(filePath)
	fileHashFromName := ""
	{
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".yaml" || ext == ".yml" {
			base := strings.TrimSuffix(filename, ext)
			if len(base) == 64 {
				isHex := true
				for _, c := range base {
					if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
						isHex = false
						break
					}
				}
				if isHex {
					fileHashFromName = strings.ToLower(base)
				}
			}
		}
	}

	// CRITICAL: All kinds use CAS now - check CAS index instead of HashRegistry
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Get kind directory
	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		// Unknown kind - return error to stop masking issues
		issues = append(issues, Issue{
			Tier:     1,
			Category: "integrity",
			Message:  fmt.Sprintf("Cannot check integrity: unknown kind '%s' for file %s", kind, filePath),
		})
		return issues, autoFixed
	}

	// Use cached CAS when available (async validation / scheduler) to avoid loading full index per object.
	// Storage from system check is typically Batching(Routing(...)) — bare *FileObjectStorage asserts miss
	// and reloaded .doc_entry.index (~170KB) on every object under concurrency → validation timeouts.
	// keep unwrap when: check uses factory/cache providers.
	var cas *storage.ContentAddressableStorage
	if fileStorage := extractFileStorage(storageProvider); fileStorage != nil {
		if c, err := fileStorage.GetContentAddressableStorage(kind); err == nil && c != nil {
			cas = c
		}
	}
	if cas == nil {
		cas = storage.NewContentAddressableStorage(kindDir, kind)
	}

	// Get object ID from parsed object
	objectID := obj.ID
	if objectID == emptyValue {
		// Try to extract from file content if not in parsed object
		if id, _ := objects.ExtractIDAndKindFromYAMLPrefix(content); id != emptyValue {
			objectID = id
		}
	}

	if objectID == emptyValue {
		// Cannot check integrity without object ID
		issues = append(issues, Issue{
			Tier:     1,
			Category: "integrity",
			Message:  fmt.Sprintf("Cannot check integrity: object ID not found in file %s", filePath),
		})
		return issues, autoFixed
	}

	// Check CAS index for this object
	expectedHash, err := cas.GetHashForID(objectID)
	if err != nil {
		// Object not in CAS index - this is a missing hash violation
		//
		// IMPORTANT: If the file is already hash-addressed and the content hash matches the filename,
		// this is strong evidence of internal index lag/races (not external tampering).
		// We silently auto-heal the index upon detection without raising user-facing errors.
		if fileHashFromName != emptyValue && hashStr == fileHashFromName {
			// In-memory + write queue only — never sync SetMapping/Save on the validation hot path
			// (file lock budget is 5s, same as per-object validation timeout).
			cas.HealIndexMappingAsync(objectID, fileHashFromName)
			autoFixed = append(autoFixed, "Silently auto-healed CAS index lag")
			return issues, autoFixed
		}

		fileInfo, statErr := fileutil.Stat(filePath)
		var ageWarning bool
		var ageMessage string
		if statErr == nil {
			age := time.Since(fileInfo.ModTime())
			if age > 30*time.Second {
				ageWarning = true
				ageMessage = fmt.Sprintf(" (file modified %v ago - indicates direct file manipulation outside CLI)", age.Round(time.Second))
			}
		}

		// Tier 4 (recommendation) so it does not count as a violation; auto-fix still applies.
		tier := 4
		message := "No integrity hash recorded for this file (object not in CAS index)"
		if ageWarning {
			message += ageMessage
		}

		issues = append(issues, Issue{
			Tier:        tier,
			Category:    "integrity",
			Message:     message,
			AutoFixable: true,
		})
		return issues, autoFixed
	}

	// Verify integrity, distinguishing real tampering from index drift.
	if fileHashFromName != emptyValue {
		// Hash-addressed CAS file
		if hashStr != fileHashFromName {
			// File content changed (e.g. edit) so filename hash no longer matches. Auto-fix by re-writing content-addressed file and updating index.
			// Use "Hash mismatch detected" prefix so tests and auto-fix logic that match that phrase work for CAS files too.
			issues = append(issues, Issue{
				Tier:        1,
				Category:    "integrity",
				Message:     fmt.Sprintf("Hash mismatch detected (CAS file corruption detected): filename hash %s... does not match file content hash %s... for object %s. Use --auto-fix to regenerate.", fileHashFromName[:16], hashStr[:16], objectID),
				AutoFixable: true,
			})
			return issues, autoFixed
		}

		// Content matches filename; if index differs, it's an internal consistency problem, not tampering.
		if !strings.EqualFold(expectedHash, fileHashFromName) {
			// IMPORTANT: CAS.Update intentionally retains old hash files (version history) until garbage-collected.
			// If the indexed hash file exists, then this file is simply a stale version and should not be treated
			// as a Tier-2 warning. If the indexed hash file is missing, that's a true index/file inconsistency.
			if casHashFileExists(kindDir, expectedHash) {
				issues = append(issues, Issue{
					Tier:     4,
					Category: "integrity",
					Message: fmt.Sprintf(
						"Stale CAS version detected for object %s: current index hash %s... differs from this file hash %s.... This is expected when old CAS versions are retained; run %s system cleanup-duplicates %s --hash-duplicates to quarantine old versions.",
						objectID, expectedHash[:16], fileHashFromName[:16], paths.CLICommandName, kind,
					),
					AutoFixable: true,
				})
			} else {
				issues = append(issues, Issue{
					Tier:        4,
					Category:    "integrity",
					Message:     fmt.Sprintf("CAS index out of sync for object %s: index hash %s... != file hash %s.... Indexed hash file is missing; this is an internal indexing/state issue, not external tampering.", objectID, expectedHash[:16], fileHashFromName[:16]),
					AutoFixable: true,
				})
			}
		}

		return issues, autoFixed
	}

	// Legacy/non-hash filename: we cannot reliably infer tampering from filename alone.
	// If the CAS index doesn't match the file's content hash, treat as index drift/migration state, not tampering.
	// Tier 4 (recommendation) so it does not count as a violation; auto-fix still applies.
	if expectedHash != hashStr {
		issues = append(issues, Issue{
			Tier:        4,
			Category:    "integrity",
			Message:     fmt.Sprintf("CAS index out of sync for object %s: index hash %s... != file content hash %s.... This is usually internal index drift or legacy (non-CAS) file state, not a definitive tampering signal.", objectID, expectedHash[:16], hashStr[:16]),
			AutoFixable: true,
		})
	}

	return issues, autoFixed
}
func casHashFileExists(kindDir, hash string) bool {
	if kindDir == emptyValue || hash == emptyValue {
		return false
	}
	base := filepath.Join(kindDir, hash+".yaml")
	if _, err := fileutil.Stat(base); err == nil {
		return true
	}
	entries, err := fileutil.ReadDir(kindDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p := filepath.Join(kindDir, entry.Name(), hash+".yaml")
		if _, err := fileutil.Stat(p); err == nil {
			return true
		}
	}
	return false
}
