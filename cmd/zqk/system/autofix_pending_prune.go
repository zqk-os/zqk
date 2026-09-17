package system

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// PrunePendingAutofixBatchesForObjectID removes objectID from pending AUTOFIX-*.json
// batches under .zqk/autofix/. Empty batches are deleted. Best-effort sync for shockwave /
// CascadeOnObjectChange so deferred apply cannot replay issues already resolved by promote
// or other mutations.
// TRACK: BLI-1785723654802038000-b14064bc — remove when: pending autofix is always
// revalidated against live state and batches are id-indexed (no full-dir scan needed).
func PrunePendingAutofixBatchesForObjectID(projectRoot, objectID string) (rewritten, deleted int) {
	if projectRoot == "" || objectID == "" {
		return 0, 0
	}
	autofixDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.AutofixDir)
	entries, err := fileutil.ReadDir(autofixDir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, autoFixBatchFilePrefix) || !strings.HasSuffix(name, autoFixBatchFileSuffix) {
			continue
		}
		path := filepath.Join(autofixDir, name)
		data, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			continue
		}
		// Cheap reject: skip files that cannot contain this id.
		if !strings.Contains(string(data), objectID) {
			continue
		}
		var batch AutoFixBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			continue
		}
		changed := false
		if len(batch.Objects) > 0 {
			kept := batch.Objects[:0]
			for _, obj := range batch.Objects {
				if obj.ObjectID == objectID {
					changed = true
					continue
				}
				kept = append(kept, obj)
			}
			batch.Objects = kept
		}
		if len(batch.Issues) > 0 {
			kept := batch.Issues[:0]
			for _, iss := range batch.Issues {
				if iss.ObjectID == objectID {
					changed = true
					continue
				}
				kept = append(kept, iss)
			}
			batch.Issues = kept
		}
		if !changed {
			continue
		}
		if len(batch.Objects) == 0 && len(batch.Issues) == 0 {
			if err := fileutil.Remove(path); err == nil {
				deleted++
			}
			continue
		}
		out, err := json.MarshalIndent(batch, "", "  ")
		if err != nil {
			continue
		}
		if err := fileutil.WriteFile(path, out, 0o644); err == nil {
			rewritten++
		}
	}
	return rewritten, deleted
}

// systemCheckIsFullKernelScan reports whether this check invocation scanned the whole kernel
// (check / check all) rather than a kind, id list, or --ids-from-file subset.
func systemCheckIsFullKernelScan(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if idsFile, _ := cmd.Flags().GetString("ids-from-file"); strings.TrimSpace(idsFile) != "" {
		return false
	}
	args := cmd.Flags().Args()
	return len(args) == 0 || (len(args) == 1 && args[0] == "all")
}

// checkResultsHaveTierIssues reports whether any result has an issue at tier <= maxTier.
func checkResultsHaveTierIssues(results []CheckResult, maxTier int) bool {
	for _, r := range results {
		for _, issue := range r.Issues {
			if issue.Tier > 0 && issue.Tier <= maxTier {
				return true
			}
		}
	}
	return false
}

// maybeClearStaleAutofixBatchesAfterLiveGreen deletes pending AUTOFIX-*.json when a full
// kernel check found no tier-1/2/3 issues. Those batches are stale snapshots; leaving them
// blocked "healthy" and invited deferred churn. Scoped checks never clear (other kinds may
// still need their batches).
// TRACK: BLI-1785723654802038000-b14064bc
func maybeClearStaleAutofixBatchesAfterLiveGreen(cmd *cobra.Command, projectRoot string, results []CheckResult) int {
	if projectRoot == "" || !systemCheckIsFullKernelScan(cmd) {
		return 0
	}
	if checkResultsHaveTierIssues(results, 3) {
		return 0
	}
	pending := countUnprocessedAutofixBatches(projectRoot)
	if pending == 0 {
		return 0
	}
	clearUnprocessedAutofixBatchesWhenGreen(projectRoot)
	return pending
}

// issueStillAppliesToObject reports whether a snapshotted autofix issue is still relevant
// for the live object. Cheap predicates only — fail closed (return true) when unsure.
// TRACK: BLI-1785723654802038000-b14064bc
func issueStillAppliesToObject(kind string, props map[string]any, issue Issue) bool {
	if props == nil {
		return false
	}
	msg := issue.Message
	msgLower := strings.ToLower(msg)

	if strings.Contains(msg, "Invalid lifecycle status") || issue.Category == objects.KindLifecycle {
		status, _ := props[objects.FieldKeyStatus].(string)
		if status == "" {
			return true
		}
		valid, err := objects.GetGlobalLifecycleLoader().IsValidStatus(kind, status)
		if err != nil {
			return true
		}
		return !valid
	}

	if strings.Contains(msg, "reference cannot be empty") {
		field := fieldNamePrefixFromIssueMessage(msg)
		if field == "" {
			return true
		}
		return propertyIsEmptyOrMissing(props, field)
	}

	if strings.Contains(msgLower, "is required") {
		field := fieldNamePrefixFromIssueMessage(msg)
		if field == "" {
			field = fieldNameFromRequiredMessage(msg)
		}
		if field == "" {
			return true
		}
		return propertyIsEmptyOrMissing(props, field)
	}

	if strings.Contains(msg, "Referenced object") && strings.Contains(msg, "does not exist") {
		refID, _, fieldName := parseReferenceIssueMessage(strings.ReplaceAll(msg, "\n", " "))
		if refID == "" || fieldName == "" {
			return true
		}
		// Stale if the referrer no longer points at the missing id.
		return propertyStillReferences(props, fieldName, refID)
	}

	if strings.Contains(msgLower, "must be one of") {
		field := fieldNamePrefixFromIssueMessage(msg)
		allowed := parseMustBeOneOfValues(msg)
		if field == "" || len(allowed) == 0 {
			return true
		}
		cur, _ := props[field].(string)
		if strings.TrimSpace(cur) == "" {
			return true
		}
		for _, a := range allowed {
			if a == cur {
				return false
			}
		}
		return true
	}

	return true
}

// filterStaleAutoFixBatchIssues drops snapshotted issues that no longer apply to live state.
func filterStaleAutoFixBatchIssues(kind string, props map[string]any, issues []AutoFixBatchIssue) (live, stale []AutoFixBatchIssue) {
	live = make([]AutoFixBatchIssue, 0, len(issues))
	for _, bi := range issues {
		if issueStillAppliesToObject(kind, props, bi.Issue) {
			live = append(live, bi)
		} else {
			stale = append(stale, bi)
		}
	}
	return live, stale
}

func fieldNamePrefixFromIssueMessage(msg string) string {
	field := strings.TrimSpace(strings.Split(msg, ":")[0])
	if field == "" || strings.Contains(field, " ") {
		return ""
	}
	return field
}

func fieldNameFromRequiredMessage(msg string) string {
	// "Field title is required"
	const prefix = "Field "
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(msg[idx+len(prefix):])
	end := strings.IndexAny(rest, " \t")
	if end <= 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func propertyIsEmptyOrMissing(props map[string]any, field string) bool {
	val, ok := props[field]
	if !ok || val == nil {
		return true
	}
	switch t := val.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	default:
		return false
	}
}

func propertyStillReferences(props map[string]any, field, refID string) bool {
	val, ok := props[field]
	if !ok || val == nil {
		return false
	}
	switch t := val.(type) {
	case string:
		return t == refID
	case []string:
		for _, s := range t {
			if s == refID {
				return true
			}
		}
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && s == refID {
				return true
			}
		}
	}
	return false
}

func parseMustBeOneOfValues(msg string) []string {
	lower := strings.ToLower(msg)
	idx := strings.Index(lower, "must be one of")
	if idx < 0 {
		return nil
	}
	rest := strings.TrimSpace(msg[idx+len("must be one of"):])
	rest = strings.TrimPrefix(rest, ":")
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, "[]()")
	if rest == "" {
		return nil
	}
	parts := strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || r == '|' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
