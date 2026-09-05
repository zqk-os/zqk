package mcp

import (
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// ExtractGroupFromPath extracts the command group from a command path.
// Example: "object list" -> "object", "system status" -> "system"
// Returns empty string if path has only one word or is empty.
func ExtractGroupFromPath(path string) string {
	parts := strings.Fields(path)
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

// ExtractCommandPath extracts the command path from tool arguments.
// Handles both direct _command_path and legacy command/subcommand format.
// Returns normalized command path or error if not found.
func ExtractCommandPath(args map[string]any) (string, error) {
	// Try direct command path first
	if commandPath, ok := args["_command_path"].(string); ok && commandPath != emptyValue {
		return NormalizeCommandPath(commandPath), nil
	}

	// Fallback to legacy command/subcommand format
	command, _ := args[objects.FieldKeyCommand].(string)
	subcommand, _ := args["subcommand"].(string)
	if command != emptyValue {
		commandPath := command
		if subcommand != emptyValue {
			commandPath = commandPath + " " + subcommand
		}
		return NormalizeCommandPath(commandPath), nil
	}

	return "", NewMissingParameterError("_command_path", "ExtractCommandPath")
}

// ParseReference parses a reference string into kind and ID.
// Format: "kind:id" or just "id" (returns empty kind).
// Example: "backlog_item:BLI-123" -> ("backlog_item", "BLI-123")
// Example: "BLI-123" -> ("", "BLI-123")
func ParseReference(ref string) (kind, id string) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ref
}
