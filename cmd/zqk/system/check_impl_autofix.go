package system

import (
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/fitness"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
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
			// Do not log "Auto-fix applied" here yet; we must wait for persistence
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

	// Demote to error only for issue_class=process_failure (illegal lifecycle / status
	// preconditions), and only when kind family allows autofix demote. Completeness /
	// employment / referential findings must not yank process position.
	if currentStatus, ok := fixCtx.Obj.Properties[objects.FieldKeyStatus].(string); ok {
		if shouldDemoteStatusToErrorForUnresolvedIssues(kind, currentStatus, sortedIssues) {
			reason := demoteReasonFromIssues(sortedIssues)
			fixCtx.Obj.Properties[objects.FieldKeyStatus] = "error"
			fixCtx.Obj.Status = "error"
			recordAutofixDemoteReason(fixCtx.Obj.Properties, reason)
			fixed = append(fixed, fmt.Sprintf("Demoted status from %q to \"error\" (%s)", currentStatus, reason))
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

			// Log the individual applied fixes only after successful persistence
			for _, msg := range fixed {
				logging.Fluent(fixCtx.Logger).Info("Auto-fix applied").
					String("object_id", obj.ID).
					String("fix_message", msg).
					Log()
			}

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
			// Fail-closed: if we didn't persist, we must not return them as fixed
			fixed = nil
		}
	} else if storageProvider == nil && len(fixed) > 0 {
		// Dry run or no storage provider: log what would be applied
		for _, msg := range fixed {
			logging.Fluent(fixCtx.Logger).Info("Auto-fix planned (dry-run)").
				String("object_id", obj.ID).
				String("fix_message", msg).
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
		if issue.Category == objects.KindLifecycle || strings.Contains(issue.Message, "Invalid lifecycle status") {
			if msg := processInvalidLifecycleStatusIssue(fixCtx, issue); msg != emptyValue {
				return msg
			}
		}

		// Duplicate IDs used to os.Remove the "stale" file. That bypasses object-delete
		// unlink and wipes sole CAS objects when the object-id cache/index is wrong
		// (00dd9c269e; 2026-08-18 GhostRef cohort). Quarantine via cleanup-duplicates
		// or `zqk object delete --unlink-references` — never silent unlink of the file.
		if strings.Contains(issue.Message, "Duplicate object ID") && issue.AutoFixable {
			if fixCtx.AutoFix || fixCtx.Force {
				return paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("refused to auto-delete purported duplicate %s (%s); use cleanup-duplicates quarantine or zqk object delete --unlink-references", filepath.Base(fixCtx.FilePath), fixCtx.Kind))
			}
		}

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

// processReferenceIssue processes reference validation issues.
// Prefer re-adding the target to the object-id cache when the file exists.
// When the target is truly gone, unlink it from the referring object (do not invent a replacement).
func processReferenceIssue(fixCtx *AutoFixContext, issue Issue) string {
	message := strings.ReplaceAll(issue.Message, "\n", " ")
	if !strings.Contains(message, "does not exist") {
		return ""
	}

	refID, refKind, fieldName := parseReferenceIssueMessage(message)
	if refID == emptyValue {
		return ""
	}

	projectRoot := ProjectRootOrResolve(fixCtx.Ctx.ProjectRoot)
	filePath := findObjectFile(projectRoot, refID, refKind)
	if filePath != emptyValue {
		// Object exists on disk but not in cache — reindex into object-id cache.
		if !issue.AutoFixable && !(fixCtx.AutoFix || fixCtx.Force) {
			return ""
		}
		if refKind == emptyValue {
			refKind = inferKindFromID(refID)
		}
		objectIDCache := GetGlobalObjectIDCache()
		if err := objectIDCache.Update(refID, refKind, filePath); err != nil {
			logging.Fluent(fixCtx.Logger).Debug("Failed to update cache for reference").
				String("ref_id", refID).
				String("ref_kind", refKind).
				WithError(err).
				Log()
			return ""
		}
		if projectRoot != emptyValue {
			if err := objectIDCache.SaveCache(projectRoot); err != nil {
				logging.Fluent(fixCtx.Logger).Debug("Failed to save cache after reference fix").
					String("ref_id", refID).
					WithError(err).
					Log()
			}
		}
		return fmt.Sprintf("Reindexed existing object %s (kind: %s) into object-id cache from %s", refID, refKind, filepath.Base(filePath))
	}

	// Target missing on disk: do not strip referrers from --auto-fix / SCH-AUTOFIX.
	// Graph close is explicit: `zqk system kernel-integrity heal-dangling --apply` or --force.
	if !fixCtx.Force {
		return ""
	}
	if fieldName == emptyValue || fixCtx.Obj == nil || fixCtx.Obj.Properties == nil {
		return ""
	}
	if !unlinkMissingRefFromObjectProps(fixCtx.Obj.Properties, fieldName, refID) {
		return ""
	}
	return fmt.Sprintf("Unlinked missing reference %s from field %q (target not found on disk/storage; not inventing a replacement)", refID, fieldName)
}

func parseReferenceIssueMessage(message string) (refID, refKind, fieldName string) {
	parts := strings.Split(message, "Referenced object ")
	if len(parts) < 2 {
		return "", "", ""
	}
	rest := strings.TrimSpace(parts[1])
	if idx := strings.Index(rest, " (kind:"); idx > 0 {
		refID = strings.TrimSpace(rest[:idx])
		kindPart := rest[idx:]
		if kindIdx := strings.Index(kindPart, "kind: "); kindIdx >= 0 {
			kindStart := kindIdx + len("kind: ")
			if kindEnd := strings.Index(kindPart[kindStart:], ")"); kindEnd > 0 {
				refKind = strings.TrimSpace(kindPart[kindStart : kindStart+kindEnd])
			}
		}
	} else if fieldIdx := strings.Index(rest, " in field"); fieldIdx > 0 {
		refID = strings.TrimSpace(rest[:fieldIdx])
	} else if spaceIdx := strings.Index(rest, " "); spaceIdx > 0 {
		refID = strings.TrimSpace(rest[:spaceIdx])
	}
	if fieldIdx := strings.Index(rest, " in field "); fieldIdx >= 0 {
		after := rest[fieldIdx+len(" in field "):]
		end := strings.IndexAny(after, " \t\n")
		if end < 0 {
			fieldName = strings.TrimSpace(after)
		} else {
			fieldName = strings.TrimSpace(after[:end])
		}
	}
	return refID, refKind, fieldName
}

func unlinkMissingRefFromObjectProps(obj map[string]any, fieldName, refID string) bool {
	missing := map[string]struct{}{refID: {}}
	updates, err := buildUnlinkUpdates(obj, fieldName, missing)
	if err != nil || len(updates) == 0 {
		return false
	}
	for k, v := range updates {
		if v == storage.FieldUnset {
			delete(obj, k)
		} else {
			obj[k] = v
		}
	}
	return true
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

// processEmptyReferenceFieldIssue unsets empty *_ref / *_refs left after prior coerce/unlink.
func processEmptyReferenceFieldIssue(fixCtx *AutoFixContext, issue Issue) string {
	if !(fixCtx.AutoFix || fixCtx.Force) || fixCtx.Obj == nil || fixCtx.Obj.Properties == nil {
		return ""
	}
	field := strings.TrimSpace(strings.Split(issue.Message, ":")[0])
	isRefIssue := strings.Contains(issue.Message, "reference cannot be empty") ||
		strings.Contains(issue.Message, "reference_type: reference cannot be empty")
	if field == "" || (!isRefIssue && !strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs")) {
		return ""
	}
	val, ok := fixCtx.Obj.Properties[field]
	if !ok {
		return ""
	}
	empty := false
	switch t := val.(type) {
	case string:
		empty = strings.TrimSpace(t) == ""
	case []any:
		empty = len(t) == 0
	case []string:
		empty = len(t) == 0
	case nil:
		empty = true
	}
	if !empty {
		return ""
	}
	delete(fixCtx.Obj.Properties, field)
	return fmt.Sprintf("Unset empty reference field %q (empty string/list is not a valid reference; not inventing a target)", field)
}

// processInvalidLifecycleStatusIssue demotes to lifecycle status "error" when the kind
// supports it (preserving the check message as demote reason); otherwise falls back to
// the kind origin. Only rewrites when the *current* status is still invalid for the kind.
// shockwave demote muddying — invalid statuses
// must not linger as fake terminals (e.g. agent_task status=complete).
func processInvalidLifecycleStatusIssue(fixCtx *AutoFixContext, issue Issue) string {
	if !(fixCtx.AutoFix || fixCtx.Force) {
		return ""
	}
	if !strings.Contains(issue.Message, "Invalid lifecycle status") && issue.Category != objects.KindLifecycle {
		return ""
	}
	kind := fixCtx.Kind
	if kind == emptyValue && fixCtx.Obj != nil {
		kind = fixCtx.Obj.Kind
	}
	prev, _ := fixCtx.Obj.Properties[objects.FieldKeyStatus].(string)
	// Stale-batch guard: if status is already lifecycle-valid, the snapshotted issue is resolved.
	if valid, err := objects.GetGlobalLifecycleLoader().IsValidStatus(kind, prev); err == nil && valid {
		return ""
	}
	target := "error"
	if valid, err := objects.GetGlobalLifecycleLoader().IsValidStatus(kind, target); err != nil || !valid {
		target = lifecycleOriginStatus(kind)
		if target == emptyValue || target == prev {
			return ""
		}
	}
	if prev == target {
		return ""
	}
	fixCtx.Obj.Properties[objects.FieldKeyStatus] = target
	fixCtx.Obj.Status = target
	recordAutofixDemoteReason(fixCtx.Obj.Properties, issue.Message)
	if target == "error" {
		return fmt.Sprintf("Demoted status from %q to \"error\" (%s)", prev, issue.Message)
	}
	return fmt.Sprintf("Set status from %q to lifecycle origin %q for kind %s (kind has no error status; %s)", prev, target, kind, issue.Message)
}

// recordAutofixDemoteReason persists the system-check demote rationale on the object so
// shockwave/operators can see why status became error without re-running check.
func recordAutofixDemoteReason(props map[string]any, reason string) {
	if props == nil || reason == emptyValue {
		return
	}
	line := "autofix demote: " + reason
	if v, ok := props[objects.FieldKeyChangeLog]; ok {
		switch t := v.(type) {
		case []string:
			props[objects.FieldKeyChangeLog] = append(t, line)
			return
		case []any:
			props[objects.FieldKeyChangeLog] = append(t, line)
			return
		}
	}
	if notes, ok := props[objects.FieldKeyNotes].(string); ok && notes != emptyValue {
		props[objects.FieldKeyNotes] = notes + "\n" + line
		return
	}
	props[objects.FieldKeyNotes] = line
}

var (
	reDuplicateRefTarget = regexp.MustCompile(`duplicate reference "([^"]+)"`)
	reDuplicateRefWithin = regexp.MustCompile(`duplicate reference "[^"]+" within ([a-zA-Z0-9_]+)`)
	reDuplicateRefBoth   = regexp.MustCompile(`target ID appears in both ([a-zA-Z0-9_]+) and ([a-zA-Z0-9_]+)`)
)

// processDuplicateReferenceIssue eliminates duplicate intra-object references.
// For cross-field duplicates, it removes the reference from the less specific / redundant field.
// For within-field duplicates, it deduplicates the list preserving the first occurrence.
func processDuplicateReferenceIssue(fixCtx *AutoFixContext, issue Issue) string {
	if !(fixCtx.AutoFix || fixCtx.Force) || fixCtx.Obj == nil || fixCtx.Obj.Properties == nil {
		return ""
	}

	targetMatches := reDuplicateRefTarget.FindStringSubmatch(issue.Message)
	if len(targetMatches) < 2 {
		return ""
	}
	targetID := strings.TrimSpace(targetMatches[1])
	if targetID == "" {
		return ""
	}

	// Check if this is a within-field duplicate
	if withinMatches := reDuplicateRefWithin.FindStringSubmatch(issue.Message); len(withinMatches) >= 2 {
		field := withinMatches[1]
		return dedupWithinField(fixCtx.Obj.Properties, field, targetID)
	}

	// Check if this is a cross-field duplicate
	if bothMatches := reDuplicateRefBoth.FindStringSubmatch(issue.Message); len(bothMatches) >= 3 {
		fieldA := bothMatches[1]
		fieldB := bothMatches[2]
		redundantField := chooseRedundantField(fieldA, fieldB, targetID)
		if redundantField == "" {
			return ""
		}
		if !fieldContainsRef(fixCtx.Obj.Properties, redundantField, targetID) {
			// If redundantField was already cleared by a prior fix for this object, check
			// whether the other field in this pair is still duplicated in any remaining ref field.
			otherField := fieldA
			if redundantField == fieldA {
				otherField = fieldB
			}
			if fieldContainsRef(fixCtx.Obj.Properties, otherField, targetID) {
				for k := range fixCtx.Obj.Properties {
					if k == otherField || !isRefFieldName(k) {
						continue
					}
					if fieldContainsRef(fixCtx.Obj.Properties, k, targetID) {
						remainingRedundant := chooseRedundantField(otherField, k, targetID)
						if fieldContainsRef(fixCtx.Obj.Properties, remainingRedundant, targetID) {
							return removeRefFromField(fixCtx.Obj.Properties, remainingRedundant, targetID)
						}
					}
				}
			}
			return ""
		}
		return removeRefFromField(fixCtx.Obj.Properties, redundantField, targetID)
	}

	return ""
}

// chooseRedundantField determines which of two fields should yield when both reference the same target ID.
func chooseRedundantField(fieldA, fieldB, targetID string) string {
	// Rule 0: Field-prefix affinity mismatch.
	// A DEC- target never belongs in technical_spec_refs (even vs related_object_refs).
	if strings.HasPrefix(targetID, "DEC-") {
		if fieldA == "technical_spec_refs" || fieldA == "technical_spec_ref" {
			return fieldA
		}
		if fieldB == "technical_spec_refs" || fieldB == "technical_spec_ref" {
			return fieldB
		}
	}

	// Rule 1: related_object_refs is the generic catch-all bucket.
	// Any typed or specific reference field takes precedence over related_object_refs.
	if fieldA == "related_object_refs" && fieldB != "related_object_refs" {
		return fieldA
	}
	if fieldB == "related_object_refs" && fieldA != "related_object_refs" {
		return fieldB
	}

	// Rule 2: Specific persona fields (assignee_persona_ref, stakeholder_refs) take precedence over persona_refs.
	if (fieldA == "assignee_persona_ref" || fieldA == "stakeholder_refs") && fieldB == "persona_refs" {
		return fieldB
	}
	if (fieldB == "assignee_persona_ref" || fieldB == "stakeholder_refs") && fieldA == "persona_refs" {
		return fieldA
	}

	// Rule 3: Singular legacy scalar fields vs plural list fields. Plural list takes precedence.
	if fieldA == "milestone_ref" && fieldB == "milestone_refs" {
		return fieldA
	}
	if fieldB == "milestone_ref" && fieldA == "milestone_refs" {
		return fieldB
	}
	if fieldA == "requirement_ref" && fieldB == "requirement_refs" {
		return fieldA
	}
	if fieldB == "requirement_ref" && fieldA == "requirement_refs" {
		return fieldB
	}
	if fieldA == "workstream_ref" && fieldB == "workstream_refs" {
		return fieldA
	}
	if fieldB == "workstream_ref" && fieldA == "workstream_refs" {
		return fieldB
	}
	if fieldA == "goal_ref" && fieldB == "goal_refs" {
		return fieldA
	}
	if fieldB == "goal_ref" && fieldA == "goal_refs" {
		return fieldB
	}
	if fieldA+"s" == fieldB {
		return fieldA
	}
	if fieldB+"s" == fieldA {
		return fieldB
	}

	// Rule 4: criteria_refs vs evidence_refs.
	if (fieldA == "criteria_refs" && fieldB == "evidence_refs") || (fieldA == "evidence_refs" && fieldB == "criteria_refs") {
		if strings.HasPrefix(targetID, "CRIT-") {
			return "evidence_refs"
		}
		if strings.HasPrefix(targetID, "EVI-") {
			return "criteria_refs"
		}
		return "evidence_refs"
	}

	// Rule 5: dependencies vs dependency_refs.
	if (fieldA == "dependencies" && fieldB == "dependency_refs") || (fieldA == "dependency_refs" && fieldB == "dependencies") {
		return "dependencies"
	}

	// Rule 6: decision_refs vs technical_spec_refs.
	if (fieldA == "decision_refs" && fieldB == "technical_spec_refs") || (fieldA == "technical_spec_refs" && fieldB == "decision_refs") {
		if strings.HasPrefix(targetID, "DEC-") {
			return "technical_spec_refs"
		}
		return "decision_refs"
	}

	// Rule 7: blocked_by_refs vs dependency_refs.
	if (fieldA == "blocked_by_refs" && fieldB == "dependency_refs") || (fieldA == "dependency_refs" && fieldB == "blocked_by_refs") {
		return "dependency_refs"
	}

	// Fallback: If one field begins with "related_", treat it as redundant.
	if strings.HasPrefix(fieldA, "related_") && !strings.HasPrefix(fieldB, "related_") {
		return fieldA
	}
	if strings.HasPrefix(fieldB, "related_") && !strings.HasPrefix(fieldA, "related_") {
		return fieldB
	}

	return fieldB
}

// fieldContainsRef returns true if the specified field in props contains targetID.
func fieldContainsRef(props map[string]any, field, targetID string) bool {
	val, ok := props[field]
	if !ok || val == nil {
		return false
	}
	switch t := val.(type) {
	case string:
		return strings.TrimSpace(t) == targetID
	case []string:
		for _, s := range t {
			if strings.TrimSpace(s) == targetID {
				return true
			}
		}
	case []any:
		for _, s := range t {
			if strings.TrimSpace(fmt.Sprint(s)) == targetID {
				return true
			}
		}
	}
	return false
}

// removeRefFromField removes all occurrences of targetID from field in props.
// If the field becomes empty, the field key is removed from props.
func removeRefFromField(props map[string]any, field, targetID string) string {
	val, ok := props[field]
	if !ok || val == nil {
		return ""
	}
	switch t := val.(type) {
	case string:
		if strings.TrimSpace(t) == targetID {
			delete(props, field)
			return fmt.Sprintf("Removed duplicate reference %q by clearing %s", targetID, field)
		}
	case []string:
		var filtered []string
		removed := 0
		for _, s := range t {
			if strings.TrimSpace(s) == targetID {
				removed++
				continue
			}
			filtered = append(filtered, s)
		}
		if removed > 0 {
			if len(filtered) == 0 {
				delete(props, field)
				return fmt.Sprintf("Removed duplicate reference %q and cleared empty %s", targetID, field)
			}
			props[field] = filtered
			return fmt.Sprintf("Removed duplicate reference %q from %s", targetID, field)
		}
	case []any:
		var filtered []any
		removed := 0
		for _, item := range t {
			if strings.TrimSpace(fmt.Sprint(item)) == targetID {
				removed++
				continue
			}
			filtered = append(filtered, item)
		}
		if removed > 0 {
			if len(filtered) == 0 {
				delete(props, field)
				return fmt.Sprintf("Removed duplicate reference %q and cleared empty %s", targetID, field)
			}
			props[field] = filtered
			return fmt.Sprintf("Removed duplicate reference %q from %s", targetID, field)
		}
	}
	return ""
}

// dedupWithinField removes duplicate occurrences of targetID within a list field, preserving the first occurrence.
func dedupWithinField(props map[string]any, field, targetID string) string {
	val, ok := props[field]
	if !ok || val == nil {
		return ""
	}
	switch t := val.(type) {
	case []string:
		var filtered []string
		seen := false
		removed := 0
		for _, s := range t {
			if strings.TrimSpace(s) == targetID {
				if seen {
					removed++
					continue
				}
				seen = true
			}
			filtered = append(filtered, s)
		}
		if removed > 0 {
			props[field] = filtered
			return fmt.Sprintf("Deduplicated reference %q within %s (removed %d duplicate(s))", targetID, field, removed)
		}
	case []any:
		var filtered []any
		seen := false
		removed := 0
		for _, item := range t {
			if strings.TrimSpace(fmt.Sprint(item)) == targetID {
				if seen {
					removed++
					continue
				}
				seen = true
			}
			filtered = append(filtered, item)
		}
		if removed > 0 {
			props[field] = filtered
			return fmt.Sprintf("Deduplicated reference %q within %s (removed %d duplicate(s))", targetID, field, removed)
		}
	}
	return ""
}

// processInstanceValidationIssue processes an instance validation issue for auto-fixing
func processInstanceValidationIssue(fixCtx *AutoFixContext, issue Issue, specFixer *SpecBasedAutoFixer, storageProvider storage.ObjectStorageProvider) string {
	if strings.Contains(issue.Message, "Invalid lifecycle status") {
		if msg := processInvalidLifecycleStatusIssue(fixCtx, issue); msg != emptyValue {
			return msg
		}
	}
	if strings.Contains(issue.Message, "reference cannot be empty") || strings.Contains(issue.Message, "reference_type: reference cannot be empty") {
		if msg := processEmptyReferenceFieldIssue(fixCtx, issue); msg != emptyValue {
			return msg
		}
	}
	if strings.Contains(issue.Message, "duplicate reference") {
		if msg := processDuplicateReferenceIssue(fixCtx, issue); msg != emptyValue {
			return msg
		}
	}
	// Wall-clock clamp: apply in-memory (then cumulative applySpecFix). Do not use
	// executeFixCommand — literal field=value commands are not supported there.
	if isWallClockClampIssue(issue) {
		if msg := applyWallClockClampAutoFix(fixCtx, issue); msg != emptyValue {
			return msg
		}
		return ""
	}
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
	if entries, err := fileutil.ReadDir(journalDir); err == nil {
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
	auditDir := datacell.StreamCurrentKindDir(projectRoot, objects.KindAuditEvent)
	if entries, err := fileutil.ReadDir(auditDir); err == nil {
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
// validation confusion (e.g., POL-DEBUG-001 issue where stale file had same ID)
// Validation check functions are now in check_impl_validators.go
// Integrity check functions are now in check_impl_integrity.go
// Hash registry functions are now in check_impl_hash.go

// extractFileStorage unwraps Batching/Routing/Hybrid/Mesh wrappers to the file backend.
// Prefer this over a bare *FileObjectStorage assert — factory/cache providers are wrapped.
func extractFileStorage(provider storage.ObjectStorageProvider) *storage.FileObjectStorage {
	return storage.UnwrapToFileObjectStorage(provider)
}

// shouldDemoteStatusToErrorForUnresolvedIssues reports whether auto-fix should set status=error.
// Policy lives in pkg/fitness: only process_failure, and
// never for identity_governance kinds. Kinds without lifecycle "error" are skipped by applySpecFix.
func shouldDemoteStatusToErrorForUnresolvedIssues(kind, currentStatus string, issues []Issue) bool {
	return wouldDemoteStatusToErrorForUnresolvedIssues(kind, currentStatus, issues)
}

func issuesToFitnessInputs(issues []Issue) []fitness.IssueInput {
	out := make([]fitness.IssueInput, 0, len(issues))
	for _, issue := range issues {
		out = append(out, fitness.IssueInput{
			Tier:        issue.Tier,
			Category:    issue.Category,
			Message:     issue.Message,
			AutoFixable: issue.AutoFixable,
		})
	}
	return out
}

// demoteReasonFromIssues picks the highest-signal process_failure message to persist with the demote.
func demoteReasonFromIssues(issues []Issue) string {
	return fitness.DemoteReasonFromIssues(issuesToFitnessInputs(issues))
}

// wouldDemoteStatusToErrorForUnresolvedIssues is the demote policy (also used by tests).
func wouldDemoteStatusToErrorForUnresolvedIssues(kind, currentStatus string, issues []Issue) bool {
	return fitness.ShouldDemoteToError(kind, currentStatus, issuesToFitnessInputs(issues)).Demote
}
