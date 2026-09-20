package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
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
	if fieldName == "commit_refs" || fieldName == objects.FieldKeyCommitHashes {
		return true
	}
	// Git branch name (spec type string). Suffix matching would Layer-1 warn
	// "Cannot determine object kind" for integration/pri-* values.
	// TRACK: PRI-CEF-R21-ENVELOPE-REMEASURE-001 / PRI-CEF-R26-LIFECYCLE-EXAM-001
	if fieldName == objects.FieldKeyBranchRef {
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

// getCacheKeyForReference gets the cache key for a reference.
// Account refs must be ACC-* (object-id-cache keys). Legacy account:username is
// resolved upstream via authcred.CanonicalAccountID before this is called.
func getCacheKeyForReference(refID, actualRefID string) string {
	if strings.HasPrefix(refID, "ACC-") {
		return refID
	}
	if strings.HasPrefix(actualRefID, "ACC-") {
		return actualRefID
	}
	// Unresolved legacy account:username — keep for diagnostic Cache key used: …
	if strings.HasPrefix(refID, objects.KindAccount+":") {
		return refID
	}
	return actualRefID
}

// wellKnownAccountRefs are account refs that always exist (no cache/storage lookup needed).
// Skipping lookup for these avoids slow Exists() calls that can cause audit_event validation timeouts.
var wellKnownAccountRefs = map[string]bool{
	pkgctx.SystemAccountID:             true, // account:system (migrated)
	authcred.DefaultSwarmWorkerAccount: true, // account:swarm_worker (migrated)
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

	projectRoot := ProjectRootOrResolve(refCtx.Ctx.ProjectRoot)

	// Canonicalize retired account:username → ACC-* so cache keys match object-id-cache.
	// TRACK: BLI-1785905136581480000-1f317f44 — Cache key used: account:swarm_worker was a false miss.
	lookupRef := refID
	if refKind == objects.KindAccount {
		if canon := authcred.CanonicalAccountID(projectRoot, refID); canon != "" {
			lookupRef = canon
			actualRefID = canon
		}
	}

	// Fast path: well-known account refs (e.g. ACC-1785920548450214012-68b850c0) are always valid; skip cache/storage lookup
	// to avoid slow Exists() that can cause audit_event validation to timeout.
	if refKind == objects.KindAccount && (wellKnownAccountRefs[lookupRef] || wellKnownAccountRefs[refID]) {
		return nil
	}

	cacheKey := getCacheKeyForReference(lookupRef, actualRefID)

	// Mid-refresh: CAS mutation pending object-id-cache true-up — soft-exclude, do not Layer-0/1 hard-fail.
	// TRACK: BLI-1785895580100186000-c5539372
	if storage.IsObjectIDCachePending(projectRoot, cacheKey) || storage.IsObjectIDCachePending(projectRoot, refID) {
		return []Issue{
			{
				Tier:     3,
				Category: categoryCacheCoherence,
				Message: fmt.Sprintf(
					"Excluded from blocking check: object-id-cache not yet trued after CAS mutation for %s in field %s. Re-run after cache refresh / EnsureObjectIDCacheReady.",
					refID, fieldName,
				),
			},
		}
	}

	entry, exists := objectIDCache.Get(cacheKey)
	if !exists {
		// Track cache miss via coordinator (best effort)
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
				// High-volume kinds (scheduler_job, audit_event, metrics, …) are intentionally
				// omitted from object-id-cache; Exists+miss is expected, not CacheLag.
				// TRACK: BLI-1786387465409533000-45bd780c — honest GhostRef vs CacheLag remediation.
				if storage.IsHighVolumeKindForCache(refKind) {
					return nil
				}
				// Reference is valid; cache was stale, not the reference
				return []Issue{
					{
						Tier:     3,
						Category: categoryCacheLag,
						Message: fmt.Sprintf(
							"CacheLag: Referenced object %s exists in storage but is missing from object-id-cache. Re-run with --refresh-cache.",
							refID,
						),
					},
				}
			}
		}

		if storageProvider != nil {
			entryCount := objectIDCache.GetEntryCount()

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

		// Not AutoFixable: --auto-fix / SCH-AUTOFIX must not unlink or delete.
		// Close the graph with explicit `zqk system kernel-integrity heal-dangling --apply`
		// after restoring or intentionally dropping the target.
		// TRACK: BLI-1786387465409533000-45bd780c — honest GhostRef vs CacheLag remediation.
		return []Issue{
			{
				Tier:        1,
				Category:    "GhostRef",
				Message:     buildGhostRefDiagnostic(refID, refKind, fieldName, cacheKey),
				AutoFixable: false,
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

	// Cache hit but path still names a deleted CAS blob while live resolve works — lag, not dangling.
	if caspkg.CachePathNeedsCASResolve(entry.FilePath) {
		if live, ok := caspkg.ResolveLiveCASFilePath(projectRoot, entry.Kind, cacheKey); ok && live != emptyValue {
			return []Issue{
				{
					Tier:     3,
					Category: categoryCacheCoherence,
					Message: fmt.Sprintf(
						"Excluded from blocking check: object-id-cache path stale for %s in field %s (live CAS path resolvable). Re-run after cache refresh.",
						refID, fieldName,
					),
				},
			}
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
				Tier:        1,
				Category:    "reference",
				Message:     buildFileSystemMissDiagnostic(refID, refKind, fieldName, lookupID),
				AutoFixable: false, // do not unlink referrers from --auto-fix / SCH-AUTOFIX; use heal-dangling --apply
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

// buildGhostRefDiagnostic builds a Tier-1 message after object-id cache miss and storage Exists false.
// Do not lead with --refresh-cache: that is CacheLag only (Exists true). See KERNEL_COHERENCE_AND_REF_GRAPH.md.
func buildGhostRefDiagnostic(refID, refKind, fieldName, cacheKey string) string {
	var msg strings.Builder
	fmt.Fprintf(&msg, "GhostRef: Referenced object %s (kind: %s) in field %s does not exist", refID, refKind, fieldName)
	fmt.Fprintf(&msg, "\n  Cache key used: %s (miss; storage Exists also false or unavailable)", cacheKey)
	msg.WriteString("\n  Diagnostic steps:")
	fmt.Fprintf(&msg, "\n    1. Confirm absence: %s object get %s", paths.CLICommandName, refID)
	fmt.Fprintf(&msg, "\n    2. Close the graph: %s system kernel-integrity heal-dangling (dry-run, then --apply)", paths.CLICommandName)
	fmt.Fprintf(&msg, "\n    3. Going forward: %s object delete <id> --unlink-references (do not leave dangling edges)", paths.CLICommandName)
	msg.WriteString("\n  Only if object get SUCCEEDS but check still fails (CacheLag):")
	fmt.Fprintf(&msg, "\n    - Then refresh: %s system check --refresh-cache", paths.CLICommandName)
	msg.WriteString("\n  Architecture: docs/architecture/KERNEL_COHERENCE_AND_REF_GRAPH.md")
	return msg.String()
}

// buildCacheMissDiagnostic is retained for tests/callers that still match the legacy phrase.
// Prefer buildGhostRefDiagnostic on the production miss path after Exists false.
func buildCacheMissDiagnostic(refID, refKind, fieldName, cacheKey string) string {
	return buildGhostRefDiagnostic(refID, refKind, fieldName, cacheKey)
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
	info, err := fileutil.Stat(full)
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

		// leftover document_refs may list repo-relative paths (markdown, scripts, specs).
		// Kernel pointers live on doc_entry_refs (DOC-* only).
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
		if info, err := fileutil.Stat(directPath); err == nil && !info.IsDir() {
			return directPath
		}
	}

	// Fallback: walk directory tree (slower but more thorough)
	for _, dir := range searchDirs {
		if _, err := fileutil.Stat(dir); err != nil {
			continue
		}

		var foundPath string
		_ = filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
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
