package system

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// ReferenceCheckContext groups state for reference checking
type ReferenceCheckContext struct {
	Ctx             *cli.Context
	Obj             *parser.ParsedObject
	Kind            string
	RefFields       map[string]any
	CheckedRefs     map[string]bool
	YAMLParser      *parser.YAMLParser
	StorageProvider storage.ObjectStorageProvider // For coordinator events
}

// initializeReferenceCheckContext sets up the reference check context
func initializeReferenceCheckContext(ctx *cli.Context, obj *parser.ParsedObject, kind string, storageProvider storage.ObjectStorageProvider) *ReferenceCheckContext {
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj.Properties)

	return &ReferenceCheckContext{
		Ctx:             ctx,
		Obj:             obj,
		Kind:            kind,
		RefFields:       refFields,
		CheckedRefs:     make(map[string]bool),
		YAMLParser:      yamlParser,
		StorageProvider: storageProvider,
	}
}

// shouldSkipReferenceField determines if a reference field should be skipped
func shouldSkipReferenceField(kind, fieldName string) bool {
	if fieldName == objects.FieldKeyCommitRefs {
		return true
	}
	return kind == objects.KindChangeJournalEntry && fieldName == "object_ref"
}

// referencePlaceholders are values that appear in reference fields but are schema/UI
// placeholders (e.g. "required", "optional"), not actual reference IDs. Skip validation.
var referencePlaceholders = map[string]bool{
	"required":             true,
	"required at creation": true,
	"optional":             true,
	"TBD":                  true,
	"tbd":                  true,
	"":                     true, // empty already skipped by extractReferenceIDs
}

func isReferencePlaceholder(refID string) bool {
	return referencePlaceholders[refID] || strings.TrimSpace(refID) == emptyValue
}

// extractReferenceIDs extracts reference IDs from a reference value
func extractReferenceIDs(refValue any) []string {
	var refIDs []string

	switch v := refValue.(type) {
	case string:
		if v != emptyValue {
			refIDs = []string{v}
		}
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok && str != emptyValue {
				refIDs = append(refIDs, str)
			}
		}
	case []string:
		refIDs = v
	}

	return refIDs
}

// parseReferenceID parses a reference ID to extract kind and actual ID.
// domain:organizational refs (e.g. domain:organizational:department:DEP-001) are
// parsed so refKind=department, actualRefID=DEP-001, matching how the object
// ID cache and file storage are keyed (short id per kind).
func parseReferenceID(refID string) (refKind string, actualRefID string) {
	actualRefID = refID

	accountRefPrefix := objects.KindAccount + ":"
	if strings.HasPrefix(refID, accountRefPrefix) {
		refKind = objects.KindAccount
		actualRefID = strings.TrimPrefix(refID, accountRefPrefix)
		return refKind, actualRefID
	}
	// domain:organizational:kind:id (e.g. domain:organizational:department:DEP-001)
	if strings.HasPrefix(refID, "domain:organizational:") {
		parts := strings.SplitN(refID, ":", 4)
		if len(parts) >= 4 {
			refKind = parts[2]
			actualRefID = parts[3]
			return refKind, actualRefID
		}
	}
	if strings.Contains(refID, ":") {
		parts := strings.SplitN(refID, ":", 2)
		if len(parts) == 2 {
			refKind = parts[0]
			actualRefID = parts[1]
		} else {
			refKind = inferKindFromID(refID)
		}
	} else {
		refKind = inferKindFromID(refID)
	}

	return refKind, actualRefID
}

// getCacheKeyForReference gets the cache key for a reference
func getCacheKeyForReference(refID, actualRefID string) string {
	if strings.HasPrefix(refID, objects.KindAccount+":") {
		return refID
	}
	return actualRefID
}

// wellKnownAccountRefs are account refs that always exist (no cache/storage lookup needed).
// Skipping lookup for these avoids slow Exists() calls that can cause audit_event validation timeouts.
var wellKnownAccountRefs = map[string]bool{
	"account:system": true,
}

// validateReferenceWithCache validates a reference using the cache
func validateReferenceWithCache(refCtx *ReferenceCheckContext, refID, fieldName string, objectIDCache *ObjectIDCache) []Issue {
	refKind, actualRefID := parseReferenceID(refID)
	if refKind == emptyValue {
		return []Issue{
			{
				Tier:     2,
				Category: "reference",
				Message:  fmt.Sprintf("Cannot determine object kind for reference %s in field %s", refID, fieldName),
			},
		}
	}

	// Fast path: well-known account refs (e.g. account:system) are always valid; skip cache/storage lookup
	// to avoid slow Exists() that can cause audit_event validation to timeout.
	if refKind == objects.KindAccount && wellKnownAccountRefs[refID] {
		return nil
	}

	cacheKey := getCacheKeyForReference(refID, actualRefID)
	entry, exists := objectIDCache.Get(cacheKey)
	if !exists {
		// Track cache miss via coordinator (best effort)
		projectRoot := ProjectRootOrResolve(refCtx.Ctx.ProjectRoot)

		// Get storage provider for coordinator events and existence fallback
		storageProvider := refCtx.StorageProvider
		if storageProvider == nil {
			// Fallback: try to get storage provider
			stdctx := pkgctx.NewSystemContext()
			storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
			if err == nil && storageFactory != nil {
				storageProvider = storageFactory.GetStorage()
			}
		}

		// Fallback: object-id cache may have dropped entries whose file paths were stale
		// (e.g. CAS paths changed). Storage (CAS index) is source of truth for existence.
		if storageProvider != nil {
			stdctx := pkgctx.NewSystemContext()
			secCtx := pkgctx.NewSystemSecurityContext()
			existsInStorage, err := storageProvider.Exists(stdctx, secCtx, cacheKey)
			if err == nil && existsInStorage {
				// Reference is valid; cache was stale, not the reference
				return nil
			}
		}

		if storageProvider != nil {
			var entryCount int
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			_ = concurrency.WithRLockTimeout(
				&objectIDCache.mu,
				pkgctx.NewSystemContext(),
				nil,
				logging.NewLockLoggerAdapter(logger),
				LockNameObjectIDCacheGetEntryCount,
				func() error {
					entryCount = len(objectIDCache.idToKind)
					return nil
				},
			)

			emitCacheAvailabilityEventViaCoordinator(
				pkgctx.NewSystemContext(),
				projectRoot,
				storageProvider,
				false, // Cache miss indicates unavailability for this reference
				entryCount,
				fmt.Sprintf("reference_check_%s", refID),
				refCtx.Ctx.Profile,
			)
		}

		// Check if object actually exists in file system - if so, mark as auto-fixable
		// This allows auto-fix to add the missing entry to cache
		autoFixable := false
		filePath := findObjectFile(projectRoot, refID, refKind)
		if filePath != emptyValue {
			// Object exists but not in cache - can be auto-fixed by adding to cache
			autoFixable = true
		}

		return []Issue{
			{
				Tier:        1,
				Category:    "reference",
				Message:     buildCacheMissDiagnostic(refID, refKind, fieldName, cacheKey),
				AutoFixable: autoFixable,
			},
		}
	}

	if entry.Kind != refKind {
		return []Issue{
			{
				Tier:     2,
				Category: "reference",
				Message:  fmt.Sprintf("Reference %s in field %s: inferred kind %s but found as %s", refID, fieldName, refKind, entry.Kind),
			},
		}
	}

	return nil
}

// validateReferenceWithFileSystem validates a reference using file system lookup
func validateReferenceWithFileSystem(refCtx *ReferenceCheckContext, refID, fieldName string) []Issue {
	projectRoot := ProjectRootOrResolve(refCtx.Ctx.ProjectRoot)

	refKind, actualRefID := parseReferenceID(refID)
	if refKind == emptyValue {
		return []Issue{
			{
				Tier:     2,
				Category: "reference",
				Message:  fmt.Sprintf("Cannot determine object kind for reference %s in field %s", refID, fieldName),
			},
		}
	}

	lookupID := getCacheKeyForReference(refID, actualRefID)
	objPath, foundKind := findObjectByID(projectRoot, lookupID)
	if objPath == emptyValue {
		return []Issue{
			{
				Tier:     1,
				Category: "reference",
				Message:  buildFileSystemMissDiagnostic(refID, refKind, fieldName, lookupID),
			},
		}
	}

	if foundKind != refKind {
		return []Issue{
			{
				Tier:     2,
				Category: "reference",
				Message:  fmt.Sprintf("Reference %s in field %s: inferred kind %s but found as %s", refID, fieldName, refKind, foundKind),
			},
		}
	}

	return nil
}

// buildCacheMissDiagnostic builds a diagnostic message for cache misses
func buildCacheMissDiagnostic(refID, refKind, fieldName, cacheKey string) string {
	var msg strings.Builder
	fmt.Fprintf(&msg, "Referenced object %s (kind: %s) in field %s does not exist in object cache", refID, refKind, fieldName)
	fmt.Fprintf(&msg, "\n  Cache key used: %s", cacheKey)
	msg.WriteString("\n  Diagnostic steps:")
	fmt.Fprintf(&msg, "\n    1. Verify object exists: %s object get %s", paths.CLICommandName, refID)
	fmt.Fprintf(&msg, "\n    2. Check if file exists: find %s -name '%s.yaml'", paths.ProcessDir, refID)
	fmt.Fprintf(&msg, "\n    3. Refresh cache: %s system check --refresh-cache", paths.CLICommandName)
	msg.WriteString("\n  If object exists but not in cache, this may indicate:")
	msg.WriteString("\n    - Cache is stale (run with --refresh-cache)")
	msg.WriteString("\n    - Object was created outside CLI")
	msg.WriteString("\n    - Namespace mismatch (check namespace_id field)")
	return msg.String()
}

// buildFileSystemMissDiagnostic builds a diagnostic message for file system misses
func buildFileSystemMissDiagnostic(refID, refKind, fieldName, lookupID string) string {
	var msg strings.Builder
	fmt.Fprintf(&msg, "Referenced object %s (kind: %s) in field %s does not exist", refID, refKind, fieldName)
	fmt.Fprintf(&msg, "\n  Lookup ID used: %s", lookupID)
	msg.WriteString("\n  Diagnostic steps:")
	fmt.Fprintf(&msg, "\n    1. Verify object exists: %s object get %s", paths.CLICommandName, refID)
	fmt.Fprintf(&msg, "\n    2. Search for file: find %s -name '%s.yaml'", paths.ProcessDir, refID)
	msg.WriteString("\n    3. Check if object is in graph backend (not file backend)")
	msg.WriteString("\n  If object exists but not found, this may indicate:")
	msg.WriteString("\n    - Object is stored in graph backend instead of file backend")
	msg.WriteString("\n    - File path resolution issue (check namespace or bucketing)")
	msg.WriteString("\n    - Object was created outside file storage")
	return msg.String()
}

// isLikelyRepoPathDocumentRef is true when refID is a repo-relative file path in document_refs
// (not an inferable object id). Paths use '/' or the OS separator; object ids use known prefixes.
func isLikelyRepoPathDocumentRef(refID string) bool {
	if refID == "" {
		return false
	}
	if inferKindFromID(refID) != "" {
		return false
	}
	return strings.Contains(refID, "/") || strings.Contains(refID, string(filepath.Separator))
}

// validateDocumentPathRef checks that a document_refs path exists under project root and is a file.
func validateDocumentPathRef(projectRoot, refID, fieldName string) []Issue {
	if projectRoot == "" {
		return nil
	}
	rel := filepath.Clean(refID)
	if rel == "." || strings.HasPrefix(rel, "..") {
		return []Issue{{
			Tier:     2,
			Category: "reference",
			Message:  fmt.Sprintf("Invalid document_refs path in field %s: %q", fieldName, refID),
		}}
	}
	full := filepath.Join(projectRoot, rel)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return []Issue{{
			Tier:     1,
			Category: "reference",
			Message:  fmt.Sprintf("document_refs path not found or not a file in field %s: %s (resolved: %s)", fieldName, refID, full),
		}}
	}
	return nil
}

// processReferenceField processes a single reference field
func processReferenceField(refCtx *ReferenceCheckContext, fieldName string, refValue any, objectIDCache *ObjectIDCache, useCache bool) []Issue {
	var issues []Issue

	if shouldSkipReferenceField(refCtx.Kind, fieldName) {
		return nil
	}

	refIDs := extractReferenceIDs(refValue)
	for _, refID := range refIDs {
		if isReferencePlaceholder(refID) || refCtx.CheckedRefs[refID] {
			continue
		}
		refCtx.CheckedRefs[refID] = true

		// document_refs may list repo-relative paths (markdown, scripts, specs), not only object IDs.
		if fieldName == objects.FieldKeyDocumentRefs && isLikelyRepoPathDocumentRef(refID) {
			fieldIssues := validateDocumentPathRef(ProjectRootOrResolve(refCtx.Ctx.ProjectRoot), refID, fieldName)
			issues = append(issues, fieldIssues...)
			continue
		}

		if useCache && objectIDCache != nil {
			fieldIssues := validateReferenceWithCache(refCtx, refID, fieldName, objectIDCache)
			issues = append(issues, fieldIssues...)
		} else {
			fieldIssues := validateReferenceWithFileSystem(refCtx, refID, fieldName)
			issues = append(issues, fieldIssues...)
		}
	}

	return issues
}

// findObjectFile finds the file path for an object by ID and kind
// Returns empty string if not found
func findObjectFile(projectRoot, objectID, kind string) string {
	if projectRoot == emptyValue {
		return ""
	}

	// Try common locations based on kind
	searchDirs := []string{
		datacell.ProcessPrimaryDir(projectRoot),
	}

	// If kind is known, try kind-specific directory first
	if kind != emptyValue {
		kindDir := datacell.CellCASPrimaryDir(projectRoot, kind)
		searchDirs = append([]string{kindDir}, searchDirs...)
	}

	// Search for file with object ID
	for _, dir := range searchDirs {
		// Try direct path first (for flat structures)
		directPath := filepath.Join(dir, objectID+".yaml")
		if info, err := os.Stat(directPath); err == nil && !info.IsDir() {
			return directPath
		}
	}

	// Fallback: walk directory tree (slower but more thorough)
	for _, dir := range searchDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}

		var foundPath string
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if appledouble.SkipPathInTreeWalk(path) {
				return nil
			}
			if filepath.Base(path) == objectID+".yaml" {
				foundPath = path
				return filepath.SkipAll // Stop walking once found
			}
			return nil
		})

		if foundPath != emptyValue {
			return foundPath
		}
	}

	return ""
}
