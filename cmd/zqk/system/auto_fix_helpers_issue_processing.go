package system

import (
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// IsIssueFixableForBatch returns true if the issue is fixable and should be included in scheduler batching.
// Matches the same set as shouldProcessIssue when --auto-fix or --force: AutoFixable, Tier 4, or has FixCommand.
// Wall-clock clamp FixCommands are advisory unless AutoFixable (completed_at gate).
func IsIssueFixableForBatch(issue Issue) bool {
	if isDestructiveGraphOrFileIssue(issue) {
		return false
	}
	if isWallClockClampIssue(issue) {
		return issue.AutoFixable
	}
	return issue.AutoFixable || issue.Tier == 4 || issue.FixCommand != emptyValue
}

// isDestructiveGraphOrFileIssue is true for findings whose "fix" deletes process YAML
// or unlinks kernel refs. Those stay human/CLI-gated (cleanup-duplicates, object delete,
// heal-dangling --apply) — never SCH-AUTOFIX.
// TRACK: BLI-1785723654802038000-b14064bc
func isDestructiveGraphOrFileIssue(issue Issue) bool {
	if issue.Category == "GhostRef" {
		return true
	}
	if strings.Contains(issue.FixCommand, "cleanup-duplicates") {
		return true
	}
	msg := issue.Message
	return strings.Contains(msg, "Duplicate object ID") ||
		strings.Contains(msg, "Duplicate CAS blob") ||
		strings.Contains(msg, "Untracked traditional process file") ||
		strings.Contains(msg, "not deleted to protect sole objects")
}

// shouldProcessIssue determines if an issue should be processed.
// Policy: everything fixable when possible; Tier 4 and guidance-style fixes allowed under --auto-fix (not only --force).
func shouldProcessIssue(issue Issue, fixCtx *AutoFixContext) bool {
	isHashMismatch := strings.Contains(issue.Message, "Hash mismatch detected")

	// Hash mismatches require --auto-fix or --force
	if isHashMismatch && !fixCtx.AutoFix && !fixCtx.Force {
		return false
	}

	// Advisory wall-clock FixCommand must not ride along with blanket tier-3 --auto-fix.
	// Only process when safety-gated AutoFixable (completed_at present).
	// TRACK: PRI-1785885772223315000-0649f401
	if isWallClockClampIssue(issue) {
		return issue.AutoFixable && (fixCtx.AutoFix || fixCtx.Force)
	}

	if isDestructiveGraphOrFileIssue(issue) {
		// Duplicate-ID refusal path still runs when AutoFixable is set in tests / old batches.
		if strings.Contains(issue.Message, "Duplicate object ID") && issue.AutoFixable {
			return true
		}
		return false
	}

	// If issue is explicitly marked as auto-fixable, process it
	if issue.AutoFixable {
		return true
	}

	// For tier 1-3 issues, allow auto-fix when --auto-fix is enabled
	if fixCtx.AutoFix || fixCtx.Force {
		if issue.Tier == 1 || issue.Tier == 2 || issue.Tier == 3 {
			if issue.Category == "instance_validation" || issue.Category == "integrity" || issue.Category == "reference" {
				return true
			}
		}
	}

	// Tier 4: allow under --auto-fix (not only --force) so recommendations can be fixed or get guidance
	if (fixCtx.AutoFix || fixCtx.Force) && issue.Tier == 4 {
		return true
	}

	// Any issue with a fix command is fixable (human can run it or we execute it when possible)
	if (fixCtx.AutoFix || fixCtx.Force) && issue.FixCommand != emptyValue {
		return true
	}

	// Default: only process explicitly auto-fixable issues or hash mismatches
	return isHashMismatch
}

// determineFixKind determines the kind to use for fixing.
// Kind-from-ID is source of truth so CAS and other fixes write to the correct directory
// (e.g. namespace_registry -> namespace_registries, not namespaces).
func determineFixKind(fixCtx *AutoFixContext) string {
	if fixKind := inferKindFromID(fixCtx.Obj.ID); fixKind != emptyValue {
		return fixKind
	}
	if fixCtx.Obj.Kind != emptyValue {
		return fixCtx.Obj.Kind
	}
	return fixCtx.Kind
}

// getOriginalHash retrieves the original hash for audit events
func getOriginalHash(filePath, fixKind string, fixCtx *AutoFixContext) string {
	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	kindDir := getKindDirectory(projectRoot, fixKind)
	if kindDir == emptyValue {
		return ""
	}

	// Use command context from fixCtx to maintain proper context hierarchy
	registry := storage.NewHashRegistry(fixCtx.Cmd.Context(), fixKind, kindDir)
	if err := registry.Load(); err != nil {
		return ""
	}

	filename := filepath.Base(filePath)
	return registry.GetHash(filename)
}

// isImmutableObject checks if an object is immutable
func isImmutableObject(obj *parser.ParsedObject, fixKind string) bool {
	return storage.IsBuiltIn(obj.Properties) || fixKind == objects.KindAuditEvent || fixKind == objects.KindChangeJournalEntry
}
