package system

import (
	stdcontext "context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// ResolveFixCommandPlaceholder resolves a placeholder in a fix command by querying the storage provider
// Example input: "<CLI_COMMAND> object update BLI-917 --field milestone_refs+=<MILESTONE_ID:category=feature&tags=branding,white-label&status=planned>"
// Example output: "<CLI_COMMAND> object update BLI-917 --field milestone_refs+=MIL-045" (if unique match found)
//
// Returns:
//   - resolvedCommand: The command with placeholder resolved (if unique match found)
//   - resolved: true if placeholder was resolved, false if it should remain as placeholder
//   - candidates: List of candidate object IDs that matched (for logging/debugging)
//   - err: Error if resolution failed
func ResolveFixCommandPlaceholder(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	fixCommand string,
) (resolvedCommand string, resolved bool, candidates []string, err error) {
	// Parse the placeholder from the command
	placeholder, targetKind := parsePlaceholderFromCommand(fixCommand)
	if when.IsEmpty(placeholder) {
		// No placeholder found, return command as-is
		return fixCommand, false, nil, nil
	}

	// Extract query hint from placeholder
	// Format: <MILESTONE_ID:category=feature&tags=branding,white-label&status=planned>
	queryHint := extractQueryHintFromPlaceholder(placeholder)
	if when.IsEmpty(queryHint) {
		// No query hint, can't resolve
		return fixCommand, false, nil, nil
	}

	// Parse query hint into ListFilter
	// Query hint format: "category=feature&tags=branding,white-label&status=planned&title_pattern=White-Label Branding System"
	filter := parseQueryHintToFilter(targetKind, queryHint)

	// Query storage provider
	storageCtx := pkgctx.NewStorageContext()
	result, queryErr := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if queryErr != nil {
		return fixCommand, false, nil, errfmt.Errorf("failed to query %s objects: %w", targetKind, queryErr)
	}

	// Extract candidate IDs
	candidates = make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok {
			candidates = append(candidates, id)
		}
	}

	// Apply boolean filter chain resolution
	// If exactly one match: resolve to that ID
	// If multiple matches: could use best match logic or leave as placeholder
	// If no matches: leave as placeholder (or generate create command)
	if len(candidates) == 1 {
		// Unique match found - resolve placeholder
		resolvedCommand = replacePlaceholderInCommand(fixCommand, placeholder, candidates[0])
		return resolvedCommand, true, candidates, nil
	} else if len(candidates) > 1 {
		// Multiple matches - for now, leave as placeholder
		// TODO: Could implement "best match" logic using additional criteria
		// (e.g., most recent, closest title match, etc.)
		return fixCommand, false, candidates, nil
	} else {
		// No matches - leave as placeholder
		// TODO: Could generate a create command here if appropriate
		return fixCommand, false, nil, nil
	}
}

// parsePlaceholderFromCommand extracts the placeholder and target kind from a fix command
// Returns: (placeholder, targetKind)
// Example: "<MILESTONE_ID:category=feature>" -> ("<MILESTONE_ID:category=feature>", "milestone")
func parsePlaceholderFromCommand(command string) (placeholder, targetKind string) {
	// Find placeholder start
	start := strings.Index(command, "<")
	if start == -1 {
		return "", ""
	}

	// Find placeholder end
	end := strings.Index(command[start:], ">")
	if end == -1 {
		return "", ""
	}
	end = start + end + 1

	placeholder = command[start:end]

	// Extract target kind from placeholder
	// Format: <MILESTONE_ID:...> -> "milestone"
	// Format: <PRIORITY_PLAN_ID:...> -> "priority_plan"
	if strings.HasPrefix(placeholder, "<MILESTONE_ID") {
		targetKind = objects.KindMilestone
	} else if strings.HasPrefix(placeholder, "<PRIORITY_PLAN_ID") {
		targetKind = objects.KindPriorityPlan
	} else if strings.HasPrefix(placeholder, "<WORKSTREAM_ID") {
		targetKind = objects.KindWorkstream
	} else if strings.HasPrefix(placeholder, "<GOAL_ID") {
		targetKind = objects.KindGoal
	} else {
		targetKind = ""
	}

	return placeholder, targetKind
}

// extractQueryHintFromPlaceholder extracts the query hint from a placeholder
// Example: "<MILESTONE_ID:category=feature&tags=branding>" -> "category=feature&tags=branding"
func extractQueryHintFromPlaceholder(placeholder string) string {
	// Find the colon after the ID
	colonIdx := strings.Index(placeholder, ":")
	if colonIdx == -1 {
		return ""
	}

	// Extract everything between colon and closing bracket
	hintStart := colonIdx + 1
	hintEnd := len(placeholder) - 1 // Before the closing ">"

	if hintEnd <= hintStart {
		return ""
	}

	return placeholder[hintStart:hintEnd]
}

// parseQueryHintToFilter converts a query hint string to a ListFilter
// Query hint format: "category=feature&tags=branding,white-label&status=planned&title_pattern=White-Label Branding System"
func parseQueryHintToFilter(targetKind, queryHint string) storage.ListFilter {
	filter := storage.ListFilter{
		Kind:    targetKind,
		Filters: make(map[string]any),
	}

	if when.IsEmpty(queryHint) {
		return filter
	}

	// Split by & to get key-value pairs
	for pair := range strings.SplitSeq(queryHint, "&") {
		// Split by = to get key and value
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case objects.FieldKeyCategory:
			// Simple equality filter
			filter.Filters[objects.FieldKeyCategory] = value
		case "tags":
			// Tags: comma-separated list - use $hasAny operator
			// $hasAny checks if the array field contains any of the filter values
			var tags []string
			for tag := range strings.SplitSeq(value, ",") {
				tags = append(tags, tag)
			}
			if len(tags) > 0 {
				filter.Filters[objects.FieldKeyTags] = map[string]any{
					"$hasAny": tags,
				}
			}
		case objects.FieldKeyStatus:
			// Simple equality filter
			filter.Filters[objects.FieldKeyStatus] = value
		case "title_pattern":
			// Title pattern: use $contains operator for substring matching
			filter.Filters[objects.FieldKeyTitle] = map[string]any{
				"$contains": value,
			}
		}
	}

	return filter
}

// replacePlaceholderInCommand replaces the placeholder with the resolved ID
// Example: "<CLI_COMMAND> object update BLI-917 --field milestone_refs+=<MILESTONE_ID:...>" + "MIL-045"
//
//	-> "<CLI_COMMAND> object update BLI-917 --field milestone_refs+=MIL-045"
func replacePlaceholderInCommand(command, placeholder, resolvedID string) string {
	return strings.Replace(command, placeholder, resolvedID, 1)
}
