package mcp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// DefaultPositionalArgumentNames defines the default positional argument names
// These are arguments that should be passed as positional args (not flags) to CLI commands
// Can be overridden via ServerConfig.MCPServer.CLI.PositionalArgumentNames
var DefaultPositionalArgumentNames = []string{"id", "ids", "kind", "kinds", "path", "task_id"}

// getPositionalArgumentNames returns the list of positional argument names
// Checks ServerConfig first, falls back to default
func getPositionalArgumentNames(server *Server) []string {
	if server != nil && server.config != nil && len(server.config.MCPServer.CLI.PositionalArgumentNames) > 0 {
		return server.config.MCPServer.CLI.PositionalArgumentNames
	}
	return DefaultPositionalArgumentNames
}

// ExtractPermissionsFromCommand extracts required permissions from command annotations.
// Looks for "mcp.permissions" annotation and parses comma-separated values.
// Returns empty slice if annotation is missing or invalid.
func ExtractPermissionsFromCommand(cmd *cobra.Command) []string {
	if cmd.Annotations == nil {
		return []string{}
	}

	permsStr, ok := cmd.Annotations["mcp.permissions"]
	if !ok {
		return []string{}
	}

	// Parse comma-separated permissions
	perms := strings.Split(permsStr, ",")
	for i, p := range perms {
		perms[i] = strings.TrimSpace(p)
	}
	return perms
}

// ExtractRolesFromCommand extracts required roles from command annotations.
// Looks for "mcp.roles" annotation and parses comma-separated values.
// Returns empty slice if annotation is missing or invalid.
func ExtractRolesFromCommand(cmd *cobra.Command) []string {
	if cmd.Annotations == nil {
		return []string{}
	}

	rolesStr, ok := cmd.Annotations["mcp.roles"]
	if !ok {
		return []string{}
	}

	// Parse comma-separated roles
	roles := strings.Split(rolesStr, ",")
	for i, r := range roles {
		roles[i] = strings.TrimSpace(r)
	}
	return roles
}

// ExtractPositionalArguments extracts positional arguments from tool arguments.
// Positional args are those with names like "id", "ids", "kind", "kinds", "path".
// Uses DefaultPositionalArgumentNames or ServerConfig if available.
// Returns slice of positional argument values in order.
func ExtractPositionalArguments(args map[string]any, server *Server) []string {
	positionalArgNames := getPositionalArgumentNames(server)

	var positionalArgs []string
	for key, value := range args {
		if strings.HasPrefix(key, "_") || key == "command" || key == "subcommand" {
			continue // Skip internal fields
		}

		// Special case: 'id' is a flag field for object create/update, not positional
		if key == "id" {
			if cmdPath, ok := args["_command_path"].(string); ok && (strings.HasPrefix(cmdPath, "object create") || strings.HasPrefix(cmdPath, "object update")) {
				continue
			}
		}

		if !slices.Contains(positionalArgNames, key) {
			continue // Not a positional argument
		}

		// Add as positional argument
		if strVal, ok := value.(string); ok && strVal != "" {
			positionalArgs = append(positionalArgs, strVal)
		} else if arrVal, ok := value.([]any); ok {
			// Handle array of positional args (e.g., multiple IDs)
			for _, item := range arrVal {
				if strItem, ok := item.(string); ok && strItem != emptyValue {
					positionalArgs = append(positionalArgs, strItem)
				}
			}
		}
	}

	return positionalArgs
}

// ExtractFlags extracts flag arguments from tool arguments.
// Converts tool arguments to CLI flag format (--flag value).
// Excludes positional arguments and internal fields.
// Flag names use hyphens (e.g. sort-by, sort-asc) to match Cobra CLI conventions.
// Returns slice of flag arguments in format: ["--flag1", "value1", "--flag2", "value2"]
func ExtractFlags(args map[string]any, server *Server) []string {
	var flags []string
	positionalArgNames := getPositionalArgumentNames(server)

	for key, value := range args {
		// Skip internal fields and positional arguments
		if strings.HasPrefix(key, "_") || key == "command" || key == "subcommand" {
			continue
		}

		isPositional := slices.Contains(positionalArgNames, key)
		if key == "id" {
			if cmdPath, ok := args["_command_path"].(string); ok && (strings.HasPrefix(cmdPath, "object create") || strings.HasPrefix(cmdPath, "object update")) {
				isPositional = false
			}
		}

		if isPositional {
			continue
		}

		// CLI flags use hyphens (e.g. --sort-by, --sort-asc); MCP args use underscores
		flagKey := strings.ReplaceAll(key, "_", "-")

		// Resiliency layer: auto-correct pluralization hallucinations (field vs fields)
		if server != nil && server.rootCommand != nil {
			if rootCmd, ok := server.rootCommand.(*cobra.Command); ok {
				if cmdPath, ok := args["_command_path"].(string); ok {
					parts := strings.Fields(cmdPath)
					if cmd, _, err := rootCmd.Find(parts); err == nil && cmd != nil {
						if flagKey == "fields" && cmd.Flags().Lookup("field") != nil && cmd.Flags().Lookup("fields") == nil {
							flagKey = "field"
						} else if flagKey == "field" && cmd.Flags().Lookup("fields") != nil && cmd.Flags().Lookup("field") == nil {
							flagKey = "fields"
						}
					}
				}
			}
		}

		flagName := "--" + flagKey
		switch val := value.(type) {
		case bool:
			// Boolean flags: only include if true
			if val {
				flags = append(flags, flagName)
			}
		case []any:
			// Handle array values - add flag multiple times (cobra stringArray pattern)
			for _, item := range val {
				flags = append(flags, flagName, fmt.Sprintf("%v", item))
			}
		case []string:
			// Handle string array values
			for _, item := range val {
				flags = append(flags, flagName, item)
			}
		case map[string]any:
			// If it's a map (like fields={...}), convert each k/v pair into a --field flag
			// This makes the tool robust against AI hallucinating a JSON object for fields
			// instead of the CLI's --field stringArray format.
			if flagKey == "fields" || flagKey == "field" {
				for k, v := range val {
					flags = append(flags, "--field", fmt.Sprintf("%s=%v", k, v))
				}
			} else {
				// Fallback for other map arguments (though unlikely)
				b, err := json.Marshal(val)
				if err == nil {
					flags = append(flags, flagName, string(b))
				} else {
					flags = append(flags, flagName, fmt.Sprintf("%v", value))
				}
			}
		default:
			// Special handling for 'id' on object create/update commands
			if flagKey == "id" {
				if cmdPath, ok := args["_command_path"].(string); ok && (strings.HasPrefix(cmdPath, "object create") || strings.HasPrefix(cmdPath, "object update")) {
					// Translate id to --field id=value
					flags = append(flags, "--field", fmt.Sprintf("id=%v", value))
					continue
				}
			}
			// Convert other types to string
			flags = append(flags, flagName, fmt.Sprintf("%v", value))
		}
	}

	return flags
}
