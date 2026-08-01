package system

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// executeAutoFixIssuesCore runs auto-fix for the given issues.
// storageProvider is optional; when non-nil (e.g. async system check) it is reused for all fix operations in this run,
// avoiding per-object storage creation.
func executeAutoFixIssuesCore(ctx *cli.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string, issues []Issue, registry storage.HashRegistryProvider, hashRegistryCache *HashRegistryCacheType, objectIDCache *ObjectIDCache, storageProvider storage.ObjectStorageProvider) []string {
	fixCtx := initializeAutoFixContext(ctx, cmd, obj, filePath, kind, hashRegistryCache, objectIDCache)
	originalProps := make(map[string]any, len(obj.Properties))
	maps.Copy(originalProps, obj.Properties)

	// Log only when there are issues to avoid flooding log-events-*.log when check runs over thousands of objects with issues_count=0
	if len(issues) > 0 {
		logging.Fluent(fixCtx.Logger).Debug("Auto-fix called").
			String("object_id", obj.ID).
			Kind(kind).
			String("file_path", filePath).
			String("filename", filepath.Base(filePath)).
			Int("issues_count", len(issues)).
			Log()
	}

	if !fixCtx.AutoFix && !fixCtx.Force {
		return []string{}
	}

	// Efficiency improvement: Check if object has already been processed
	// Only check if ProcessedObjects map is initialized (shared state)
	if fixCtx.ProcessedObjects != nil {
		if fixCtx.ProcessedObjects[obj.ID] {
			logging.Fluent(fixCtx.Logger).Debug("Skipping already-processed object").
				String("object_id", obj.ID).
				Log()
			return []string{}
		}
		// Initialize map if it's nil but we want to track
	} else {
		// Initialize for this object's processing (local tracking)
		fixCtx.ProcessedObjects = make(map[string]bool)
	}

	// Sort issues by dependency hierarchy (top-level first, then by Tier)
	// This ensures we fix issues with fewer dependencies first
	// (e.g., goal_refs before milestone_refs before backlog_item_refs)
	sortedIssues := sortIssuesByDependency(issues)

	// Efficiency improvement: Batch process hash mismatches first
	// Collect all hash mismatch issues for batch processing
	hashMismatchIssues := make([]Issue, 0)
	otherIssues := make([]Issue, 0)

	for _, issue := range sortedIssues {
		if !shouldProcessIssue(issue, fixCtx) {
			continue
		}
		// Route "Hash mismatch detected" for hash-named (CAS) files to per-issue path so
		// fixCASIndexOutOfSync runs; batch path uses RemoveStaleIndexStrategy and does not fix CAS drift.
		if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
			base := filepath.Base(filePath)
			if len(base) == 69 && strings.HasSuffix(base, ".yaml") && isHexString(base[:64]) {
				otherIssues = append(otherIssues, issue)
			} else {
				hashMismatchIssues = append(hashMismatchIssues, issue)
			}
		} else {
			otherIssues = append(otherIssues, issue)
		}
	}

	// Batch fix all hash mismatches first
	if len(hashMismatchIssues) > 0 {
		batchFixed := batchFixHashMismatches(fixCtx, hashMismatchIssues, registry)
		if len(batchFixed) > 0 {
			logging.Fluent(fixCtx.Logger).Info("Batch fixed hash mismatches").
				String("object_id", obj.ID).
				Int("fixed_count", len(batchFixed)).
				Log()
		}
	}

	// Create SpecBasedAutoFixer once per object (not per issue) - optimization
	specFixer := NewSpecBasedAutoFixer(ctx, fixCtx.Logger)

	// Reuse shared storage when provided (async check); otherwise create once per object (tests, legacy path)
	if storageProvider == nil {
		storageProvider = createStorageProviderForAutoFix(ctx)
	}

	var fixed []string
	// Process other issues (non-hash-mismatch) after batch fixing hash mismatches
	for i, issue := range otherIssues {
		if !shouldProcessIssue(issue, fixCtx) {
			logSkippedIssue(fixCtx, obj.ID, i, issue)
			continue
		}

		logProcessingIssue(fixCtx, obj.ID, i, issue)
		msg := processIssueForAutoFix(fixCtx, issue, registry, specFixer, storageProvider)
		if msg != emptyValue {
			fixed = append(fixed, msg)
			logging.Fluent(fixCtx.Logger).Info("Auto-fix applied").
				String("object_id", obj.ID).
				String("issue_category", issue.Category).
				String("fix_message", msg).
				Log()
		} else {
			// Don't warn for instance_validation + pattern: we attempt normalization (schema_version, datetime);
			// if we still can't fix, it's not actionable to the user—avoid noisy "fix may have failed" logs.
			isPatternMismatch := issue.Category == "instance_validation" && (strings.Contains(issue.Message, "does not match pattern") || strings.Contains(issue.Message, "does not match ID pattern"))
			if isPatternMismatch {
				logging.Fluent(fixCtx.Logger).Debug("Auto-fix produced no change for pattern mismatch (value may need manual correction)").
					String("object_id", obj.ID).
					String("issue_category", issue.Category).
					String("issue_message", issue.Message).
					Log()
			} else if issue.Category == "instance_validation" && strings.Contains(issue.Message, "invalid datatype") {
				// Datatype mismatches are often produced by string-encoded JSON/object payloads.
				// If we still can't coerce, the user-facing warning becomes noisy/repetitive, so keep it at Debug.
				logging.Fluent(fixCtx.Logger).Debug("Auto-fix produced no change for invalid datatype (value may be non-coercible)").
					String("object_id", obj.ID).
					String("issue_category", issue.Category).
					String("issue_message", issue.Message).
					Log()
			} else if issue.AutoFixable || issue.FixCommand != emptyValue {
				logging.Fluent(fixCtx.Logger).Warn("Auto-fix returned empty message (fix may have failed)").
					String("object_id", obj.ID).
					String("issue_category", issue.Category).
					String("issue_message", issue.Message).
					String("issue_auto_fixable", fmt.Sprintf("%v", issue.AutoFixable)).
					Log()
			} else {
				logging.Fluent(fixCtx.Logger).Debug("Auto-fix produced no change for non-auto-fixable issue").
					String("object_id", obj.ID).
					String("issue_category", issue.Category).
					String("issue_message", issue.Message).
					Log()
			}
		}
	}

	// If the object has any non-auto-fixable issues, and its current status is not "error",
	// demote its status to "error" so that it's clear it has validation issues that cannot be resolved automatically.
	hasNonAutoFixable := false
	for _, issue := range sortedIssues {
		if !issue.AutoFixable && issue.Category != "integrity" {
			hasNonAutoFixable = true
			break
		}
	}
	if hasNonAutoFixable {
		if currentStatus, ok := fixCtx.Obj.Properties[objects.FieldKeyStatus].(string); ok && currentStatus != "error" {
			fixCtx.Obj.Properties[objects.FieldKeyStatus] = "error"
			fixCtx.Obj.Status = "error"
			fixed = append(fixed, fmt.Sprintf("Changed status from %q to \"error\" due to non-auto-fixable validation errors", currentStatus))
		}
	}

	// Mark object as processed (if tracking is enabled)
	if fixCtx.ProcessedObjects != nil {
		fixCtx.ProcessedObjects[obj.ID] = true
	}
	if storageProvider != nil && !reflect.DeepEqual(originalProps, fixCtx.Obj.Properties) {
		if applySpecFix(fixCtx, originalProps, fixCtx.Obj.Properties, storageProvider) {
			logging.Fluent(fixCtx.Logger).Info("Persisted cumulative auto-fix changes for object").
				String("object_id", obj.ID).
				Int("applied_fix_count", len(fixed)).
				Log()

			// Since the file might have been written to a new path (for CAS kinds),
			// update the objectIDCache to point to the new path and save the cache on disk.
			if fileStorage := extractFileStorage(storageProvider); fileStorage != nil {
				if newPath, err := fileStorage.GetFilePathForObject(obj.ID, kind); err == nil {
					if objectIDCache != nil {
						_ = objectIDCache.Update(obj.ID, kind, newPath)
						_ = objectIDCache.SaveCache(ProjectRootOrResolve(ctx.ProjectRoot))
					}
				}
			}
		} else {
			logging.Fluent(fixCtx.Logger).Warn("Cumulative auto-fix changes not persisted (object still invalid after staged fixes)").
				String("object_id", obj.ID).
				Int("staged_fix_count", len(fixed)).
				Log()
		}
	}

	return fixed
}

// autoFixIssues runs system-check auto-fix (pipeline-wrapped for standardized observability).
//
// Behavior parity note: we preserve the existing core logic and call it in the pipeline's COMMIT stage.
func autoFixIssues(
	ctx *cli.Context,
	cmd *cobra.Command,
	obj *parser.ParsedObject,
	filePath, kind string,
	issues []Issue,
	registry storage.HashRegistryProvider,
	hashRegistryCache *HashRegistryCacheType,
	objectIDCache *ObjectIDCache,
	storageProvider storage.ObjectStorageProvider,
) []string {
	fixed, err := RunAutoFixIssuesViaPipeline(ctx, cmd, obj, filePath, kind, issues, registry, hashRegistryCache, objectIDCache, storageProvider)
	if err != nil {
		return []string{}
	}
	return fixed
}

// createStorageProviderForAutoFix creates a storage provider for auto-fix operations
func createStorageProviderForAutoFix(ctx *cli.Context) storage.ObjectStorageProvider {
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil || storageFactory == nil {
		return nil
	}
	return storageFactory.GetStorage()
}

// logSkippedIssue logs when an issue is skipped during auto-fix
func logSkippedIssue(fixCtx *AutoFixContext, objectID string, issueIndex int, issue Issue) {
	logging.Fluent(fixCtx.Logger).Debug("Skipping issue").
		String("object_id", objectID).
		Int("issue_index", issueIndex).
		String("category", issue.Category).
		String("auto_fixable", fmt.Sprintf("%v", issue.AutoFixable)).
		Log()
}

// logProcessingIssue logs when processing an issue for auto-fix
func logProcessingIssue(fixCtx *AutoFixContext, objectID string, issueIndex int, issue Issue) {
	logging.Fluent(fixCtx.Logger).Debug("Processing auto-fixable issue").
		String("object_id", objectID).
		Int("issue_index", issueIndex).
		String("category", issue.Category).
		String("message", issue.Message).
		Log()
}

// processIssueForAutoFix processes a single issue for auto-fixing
// Enhanced to handle all tiers and categories
func processIssueForAutoFix(fixCtx *AutoFixContext, issue Issue, registry storage.HashRegistryProvider, specFixer *SpecBasedAutoFixer, storageProvider storage.ObjectStorageProvider) string {
	switch issue.Category {
	case "integrity":
		success, msg := fixIntegrityIssue(fixCtx, issue, registry, storageProvider)
		if success {
			return msg
		}
		return ""

	case "instance_validation":
		return processInstanceValidationIssue(fixCtx, issue, specFixer, storageProvider)

	case "documentation_policy":
		// Tier 4: Policy-based auto-fix for documentation recommendations (--auto-fix or --force)
		if fixCtx.AutoFix || fixCtx.Force {
			return processDocumentationPolicyIssue(fixCtx, issue)
		}
		return ""

	case "reference":
		// Auto-fix reference cache miss issues by adding missing objects to cache
		return processReferenceIssue(fixCtx, issue)

	case "registration", objects.KindLifecycle, objects.KindPolicy:
		// When a fix command is present, execute it (supports human-guided fixes)
		if issue.FixCommand != emptyValue && storageProvider != nil {
			success, msg := specFixer.ExecuteFixCommandOnly(fixCtx, issue, storageProvider)
			if success {
				invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)
				return msg
			}
		}
		return ""

	default:
		return ""
	}
}

// processReferenceIssue processes reference validation issues
// Auto-fixes cache miss issues by adding missing objects to cache
func processReferenceIssue(fixCtx *AutoFixContext, issue Issue) string {
	// Only process auto-fixable reference issues (cache misses where object exists)
	if !issue.AutoFixable {
		return ""
	}

	// Extract reference ID and kind from message
	// Message format: "Referenced object CRIT-9085 (kind: criteria) in field criteria_refs does not exist in object cache"
	// Note: Message may contain newlines, so we need to handle that
	message := strings.ReplaceAll(issue.Message, "\n", " ")
	if !strings.Contains(message, "does not exist in object cache") {
		return ""
	}

	// Extract object ID (e.g., "CRIT-9085")
	var refID, refKind string
	parts := strings.Split(message, "Referenced object ")
	if len(parts) < 2 {
		return ""
	}
	rest := strings.TrimSpace(parts[1])
	// Extract ID (everything before " (kind:")
	if idx := strings.Index(rest, " (kind:"); idx > 0 {
		refID = strings.TrimSpace(rest[:idx])
		// Extract kind (between "kind: " and ")")
		kindPart := rest[idx:]
		if kindIdx := strings.Index(kindPart, "kind: "); kindIdx >= 0 {
			kindStart := kindIdx + len("kind: ")
			if kindEnd := strings.Index(kindPart[kindStart:], ")"); kindEnd > 0 {
				refKind = strings.TrimSpace(kindPart[kindStart : kindStart+kindEnd])
			}
		}
	} else {
		// Fallback: try to extract ID from beginning of rest (before first space)
		if spaceIdx := strings.Index(rest, " "); spaceIdx > 0 {
			refID = strings.TrimSpace(rest[:spaceIdx])
		} else {
			// No space found - try to extract ID before "in field"
			if fieldIdx := strings.Index(rest, " in field"); fieldIdx > 0 {
				refID = strings.TrimSpace(rest[:fieldIdx])
			}
		}
	}

	if refID == emptyValue {
		return ""
	}

	// Find the object file
	projectRoot := ProjectRootOrResolve(fixCtx.Ctx.ProjectRoot)

	filePath := findObjectFile(projectRoot, refID, refKind)
	if filePath == emptyValue {
		// Object doesn't exist - can't auto-fix
		return ""
	}

	// Verify file exists and get kind if not already known
	if refKind == emptyValue {
		// Try to infer from file path or read the file
		refKind = inferKindFromID(refID)
	}

	// Add object to cache
	objectIDCache := GetGlobalObjectIDCache()
	if err := objectIDCache.Update(refID, refKind, filePath); err != nil {
		logging.Fluent(fixCtx.Logger).Debug("Failed to update cache for reference").
			String("ref_id", refID).
			String("ref_kind", refKind).
			WithError(err).
			Log()
		return ""
	}

	// Save cache to disk
	if projectRoot != emptyValue {
		if err := objectIDCache.SaveCache(projectRoot); err != nil {
			logging.Fluent(fixCtx.Logger).Debug("Failed to save cache after reference fix").
				String("ref_id", refID).
				WithError(err).
				Log()
			// Don't fail - cache update succeeded, just save failed
		}
	}

	return fmt.Sprintf("Added missing reference %s (kind: %s) to object cache", refID, refKind)
}

// processDocumentationPolicyIssue processes tier 4 documentation policy recommendations
// Applies policy-based fixes when --force is used
func processDocumentationPolicyIssue(fixCtx *AutoFixContext, issue Issue) string {
	// Extract recommendation from message if present
	// Format: "Violation: Message (Recommendation: <fix>)"
	if !strings.Contains(issue.Message, "Recommendation:") {
		return ""
	}

	// Parse recommendation from message
	parts := strings.Split(issue.Message, "Recommendation:")
	if len(parts) < 2 {
		return ""
	}

	recommendation := strings.TrimSpace(parts[1])
	if recommendation == emptyValue {
		return ""
	}

	// Apply recommendation-based fix
	// For now, log the recommendation - actual implementation depends on specific policy
	logging.Fluent(fixCtx.Logger).Info("Policy-based auto-fix recommendation").
		String("object_id", fixCtx.Obj.ID).
		String("recommendation", recommendation).
		Log()

	// Return message indicating recommendation was applied
	return fmt.Sprintf("Applied policy recommendation for %s: %s", fixCtx.Obj.ID, recommendation)
}

// processInstanceValidationIssue processes an instance validation issue for auto-fixing
func processInstanceValidationIssue(fixCtx *AutoFixContext, issue Issue, specFixer *SpecBasedAutoFixer, storageProvider storage.ObjectStorageProvider) string {
	success, msg, updatedObj := specFixer.FixInstanceValidationIssueWithStorage(fixCtx, issue, storageProvider)
	if !success {
		return ""
	}

	if updatedObj != nil {
		// Important: even when persistence is deferred/aborted (because the object still fails
		// instance validation), we must still apply the fix to the in-memory object state.
		// Otherwise, each subsequent issue fix for the same object starts from the original
		// invalid object, and the "final validation then persist" step will never converge.
		fixCtx.Obj.Properties = updatedObj

		// Layer 1: Stage in-memory changes now; persistence happens once per object at end.
		return msg
	}

	// Layer 2: Fix command executed (executeFixCommand already updated the object)
	// CRITICAL: Invalidate validation cache after fix command execution
	// The hash registry was updated by executeFixCommand, but cache still has old state
	invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)
	return msg
}

// updateHashInRegistry updates the hash in the HashRegistry (creates new registry instance)
// in the current month as recently updated at the START of the check cycle when in fix mode.
// This prevents tail-chasing where hash regeneration creates new objects that then show hash mismatches.
// By marking them at the start, we ensure they're skipped for hash validation throughout the entire check cycle.
//
//nolint:unused // Reserved for future use or legacy compatibility
func markAllInternalObjectsAsRecentlyUpdated(ctx *cli.Context) {
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)

	now := time.Now()
	month := now.Format("2006-01")

	// Mark all change journal entries in the current month as recently updated
	journalDir := filepath.Join(datacell.ProcessPrimaryDir(projectRoot), "change_journal", month)
	if entries, err := os.ReadDir(journalDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			// Extract ID from filename (e.g., "CHA-004.yaml" -> "CHA-004")
			// Note: Hash integrity is now checked LAST, so no need to track recently updated objects
			_ = strings.TrimSuffix(entry.Name(), ".yaml")
		}
	}

	// Mark all audit events in the current month as recently updated
	auditDir := filepath.Join(projectRoot, paths.ProcessAuditDir, month)
	if entries, err := os.ReadDir(auditDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			// Extract ID from filename (e.g., "AUD-33.yaml" -> "AUD-33")
			// Note: Hash integrity is now checked LAST, so no need to track recently updated objects
			_ = strings.TrimSuffix(entry.Name(), ".yaml")
		}
	}
}

// markRecentlyCreatedObjects marks recently created change journal entries and audit events as recently updated
// This is a no-op now since we mark all internal objects at the start of the check cycle in fix mode
//
//nolint:unused // No-op function - reserved for legacy compatibility
func markRecentlyCreatedObjects(ctx *cli.Context, parentKind, parentID string) {
	// No-op: All internal objects are already marked at the start of the check cycle in fix mode
}

// reloadHashRegistriesForKinds reloads hash registries for the specified kinds
// This ensures that newly created objects (change journal entries, audit events) have their hashes
// available in the cache after being created during hash mismatch fixes
//
//nolint:unused // Reserved for future use or legacy compatibility

// updateHashInRegistry updates the hash in the HashRegistry (creates new registry instance)
// Uses the file's directory to handle bucketed objects correctly

// updateHashInRegistryWithInstance updates the hash using the provided registry instance
// This allows updating a cached registry instance to keep the cache in sync
// Since the registry instance is from the cache, updating it directly updates the cache
// No need to reload - we're updating the cache directly with the CURRENT hash

// AuditEventBuffer buffers audit events during bulk operations and updates existing events
// with occurrence counts and timestamps instead of creating duplicate events
// createHashMismatchFixAuditEvent creates an audit event for a hash mismatch fix
// Uses buffering during bulk operations to reduce event count
// Uses buffering during bulk operations to update existing events with occurrence counts
// validateIDFormat is deprecated - use validation.GetIDValidator() instead
// Kept for backward compatibility but delegates to configurable validator
//
//nolint:unused // Deprecated function - reserved for backward compatibility

// filterResultsByTier filters results to only include objects that have issues of the specified tier

// prepareOutputData prepares output data structure for format handlers

// outputJSONL outputs results in JSONL format (one JSON object per line)
// This is used for ai-agent context to provide machine-readable streaming output

// wrapText wraps text to the specified width, preserving indentation

// checkDuplicateIDs checks if an object ID is duplicated across multiple files
// This detects the scenario where multiple files have the same ID, which can cause
// validation confusion (e.g., POLICY-DEBUG-001 issue where stale file had same ID)
// Validation check functions are now in check_impl_validators.go
// Integrity check functions are now in check_impl_integrity.go
// Hash registry functions are now in check_impl_hash.go

func extractFileStorage(provider storage.ObjectStorageProvider) *storage.FileObjectStorage {
	if provider == nil {
		return nil
	}
	switch v := provider.(type) {
	case *storage.FileObjectStorage:
		return v
	case *storage.BatchingObjectStorage:
		return extractFileStorage(v.ObjectStorageProvider)
	case *storage.TransactionalStorageWrapper:
		return extractFileStorage(v.GetUnderlying())
	case *storage.HybridObjectStorage:
		if fs := extractFileStorage(v.GetPrimary()); fs != nil {
			return fs
		}
		if fs := extractFileStorage(v.GetSecondary()); fs != nil {
			return fs
		}
	case *storage.MeshObjectStorage:
		return extractFileStorage(v.GetLocal())
	}

	return nil
}
