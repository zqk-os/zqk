package system

import (
	"fmt"
	"maps"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"

	"github.com/zqk-os/zqk/pkg/objects"
)

// FixCommand represents a parsed fix command
type FixCommand struct {
	ObjectID    string
	Operation   string // "update"
	Field       string
	Operator    string // "+=", "="
	Placeholder string // "<MILESTONE_ID:query_hint>"
}

// parseFixCommand parses a fix command string into a FixCommand struct
// Format: "<CLI_COMMAND> object update OBJ-ID --field field_name+=<PLACEHOLDER:query_hint>"
// Format: "<CLI_COMMAND> object update OBJ-ID --field field_name=<PLACEHOLDER:query_hint>"
func parseFixCommand(cmdStr string) (*FixCommand, error) {
	// Extract object ID
	// Pattern: "<CLI_COMMAND> object update OBJ-ID"
	// Use dynamic CLI command name for regex pattern
	cliCmd := paths.CLICommandName
	if when.IsEmpty(cliCmd) {
		cliCmd = paths.CLICommandNameDefault
	}
	// Escape special regex characters in CLI command name
	cliCmdEscaped := regexp.QuoteMeta(cliCmd)
	objIDRe := regexp.MustCompile(cliCmdEscaped + ` object update\s+([A-Z]+-\d+)`)
	objIDMatches := objIDRe.FindStringSubmatch(cmdStr)
	if len(objIDMatches) < 2 {
		return nil, errfmt.Errorf("failed to extract object ID from command: %s", cmdStr)
	}

	// Extract field name and operator
	// Pattern: "--field field_name+=" or "--field field_name="
	fieldRe := regexp.MustCompile(`--field\s+([\w]+)(\+?)=`)
	fieldMatches := fieldRe.FindStringSubmatch(cmdStr)
	if len(fieldMatches) < 3 {
		return nil, errfmt.Errorf("failed to extract field from command: %s", cmdStr)
	}

	fieldName := fieldMatches[1]
	operator := "="
	if fieldMatches[2] == "+" {
		operator = "+="
	}

	// Extract placeholder
	// Pattern: "<PLACEHOLDER_ID:query_hint>"
	placeholderRe := regexp.MustCompile(`<([A-Z_]+_ID:[^>]+)>`)
	placeholderMatches := placeholderRe.FindStringSubmatch(cmdStr)
	placeholder := ""
	if len(placeholderMatches) >= 2 {
		placeholder = "<" + placeholderMatches[1] + ">"
	}

	return &FixCommand{
		ObjectID:    objIDMatches[1],
		Operation:   "update",
		Field:       fieldName,
		Operator:    operator,
		Placeholder: placeholder,
	}, nil
}

// executeFixCommand synchronously executes a fix command by resolving placeholders and updating the object
func executeFixCommand(
	ctx *cli.Context,
	fixCtx *AutoFixContext,
	issue Issue,
	storageProvider storage.ObjectStorageProvider,
	specLoader *objects.SpecLoader,
) (bool, string) {
	// Parse the fix command
	fixCmd, parseErr := parseFixCommand(issue.FixCommand)
	if parseErr != nil {
		logging.Fluent(fixCtx.Logger).Debug("Failed to parse fix command").
			String("object_id", fixCtx.Obj.ID).
			String("command", issue.FixCommand).
			WithError(parseErr).
			Log()
		return false, fmt.Sprintf("Failed to parse fix command: %v", parseErr)
	}

	// Create field resolver
	resolver := NewFieldResolver(fixCtx.Ctx.ProjectRoot, specLoader, storageProvider)

	// Resolve placeholder if present
	var resolvedValue string
	if !when.IsEmpty(fixCmd.Placeholder) {
		// Extract query hint from placeholder
		queryHint := extractQueryHintFromPlaceholder(fixCmd.Placeholder)
		if when.IsEmpty(queryHint) {
			logging.Fluent(fixCtx.Logger).Debug("No query hint in placeholder").
				String("object_id", fixCtx.Obj.ID).
				String("placeholder", fixCmd.Placeholder).
				Log()
			return false, "No query hint in placeholder"
		}

		queryHints := parseQueryHintToMap(queryHint)

		// Resolve using field resolver
		stdctx := pkgctx.NewSystemContext()
		candidates, resolveErr := resolver.ResolveFieldFromSpec(stdctx, fixCtx.Kind, fixCmd.Field, queryHints)
		if resolveErr != nil {
			logging.Fluent(fixCtx.Logger).Debug("Failed to resolve placeholder").
				String("object_id", fixCtx.Obj.ID).
				String("field", fixCmd.Field).
				WithError(resolveErr).
				Log()
			return false, fmt.Sprintf("Failed to resolve placeholder: %v", resolveErr)
		}

		// Use first candidate if available (or all if list operator)
		if len(candidates) > 0 {
			if fixCmd.Operator == "+=" {
				// List append: use all candidates (comma-separated for now, will be split later)
				resolvedValue = strings.Join(candidates, ",")
			} else {
				// Single value: use first candidate
				resolvedValue = candidates[0]
			}
		} else {
			logging.Fluent(fixCtx.Logger).Debug("No candidates found for placeholder resolution").
				String("object_id", fixCtx.Obj.ID).
				String("field", fixCmd.Field).
				String("query_hint", queryHint).
				Log()
			return false, fmt.Sprintf("No candidates found for placeholder resolution (query_hint: %s)", queryHint)
		}
	} else {
		// No placeholder - command should have explicit value (unlikely for our use case)
		logging.Fluent(fixCtx.Logger).Debug("Fix command has no placeholder - cannot resolve").
			String("object_id", fixCtx.Obj.ID).
			String("command", issue.FixCommand).
			Log()
		return false, "Fix command has no placeholder"
	}

	// Read current object
	stdctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	currentObj, readErr := storageProvider.Read(stdctx, secCtx, fixCmd.ObjectID)
	if readErr != nil {
		logging.Fluent(fixCtx.Logger).Debug("Failed to read object for fix").
			String("object_id", fixCmd.ObjectID).
			WithError(readErr).
			Log()
		return false, fmt.Sprintf("Failed to read object: %v", readErr)
	}

	// Apply fix to object
	updatedObj := make(map[string]any, len(currentObj))
	maps.Copy(updatedObj, currentObj)

	// Apply field update based on operator
	if fixCmd.Operator == "+=" {
		// List append
		var currentList []any
		if existing, ok := updatedObj[fixCmd.Field]; ok {
			if list, ok := existing.([]any); ok {
				currentList = list
			} else if list, ok := existing.([]string); ok {
				// Convert []string to []any
				currentList = make([]any, len(list))
				for i, v := range list {
					currentList[i] = v
				}
			}
		}

		newList := make([]any, len(currentList))
		copy(newList, currentList)

		// Add resolved value(s)
		if strings.Contains(resolvedValue, ",") {
			// Multiple values
			for v := range strings.SplitSeq(resolvedValue, ",") {
				trimmed := strings.TrimSpace(v)
				if !when.IsEmpty(trimmed) {
					newList = append(newList, trimmed)
				}
			}
		} else {
			if !when.IsEmpty(resolvedValue) {
				newList = append(newList, resolvedValue)
			}
		}
		updatedObj[fixCmd.Field] = newList
	} else {
		// Single value assignment
		updatedObj[fixCmd.Field] = resolvedValue
	}

	// Update object via storage provider
	updateErr := storageProvider.Update(stdctx, secCtx, fixCmd.ObjectID, updatedObj)
	if updateErr != nil {
		logging.Fluent(fixCtx.Logger).Debug("Failed to update object with fix").
			String("object_id", fixCmd.ObjectID).
			String("field", fixCmd.Field).
			WithError(updateErr).
			Log()
		return false, fmt.Sprintf("Failed to update object: %v", updateErr)
	}

	fixMessage := fmt.Sprintf("Fixed %s by setting %s=%s (resolved from fix command)", fixCmd.Field, fixCmd.Field, resolvedValue)
	logging.Fluent(fixCtx.Logger).Info("Fix command executed successfully").
		String("object_id", fixCmd.ObjectID).
		String("field", fixCmd.Field).
		String("value", resolvedValue).
		Log()

	return true, fixMessage
}
