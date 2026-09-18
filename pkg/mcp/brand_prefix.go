package mcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// GetExecutableName returns the name of the current executable
// This is used to determine the brand prefix for tool names.
// Symlinks (e.g. bin/zqk-mcp-proxy → bin/zqk) are resolved so tool prefixes
// stay on the product brand, not the role process name.
func GetExecutableName() string {
	execPath, err := fileutil.Executable()
	if err != nil {
		// Fallback to os.Args[0] if os.Executable() fails
		if len(os.Args) > 0 {
			return filepath.Base(os.Args[0])
		}
		return brand.ExecutableName()
	}
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}
	return filepath.Base(execPath)
}

// GetBrandPrefix returns the brand prefix for tool names
// This is derived from the executable name, with common suffixes removed
// Examples:
//   - "zqk" -> "zqk"
//   - "zqk-mcp" -> "zqk" (avoids duplicate tools e.g. zqk_get_* vs zqk-mcp_get_*)
//   - "zqk-mcp-proxy" / "zqk-mcp-daemon" -> "zqk" (role process names via bin/ symlinks)
//   - "mybrand-stable" -> "mybrand"
func GetBrandPrefix() string {
	return stripBrandSuffixes(GetExecutableName())
}

// stripBrandSuffixes removes process/channel suffixes from an executable basename.
// Order matters: role names (-mcp-proxy/-mcp-daemon) before bare -mcp.
func stripBrandSuffixes(execName string) string {
	// Public product executable is "zqk". Leftover binaries named with a -community suffix
	// still strip to zqk_* MCP tools.
	// Role process names first (longest match), then bare -mcp.
	suffixes := []string{"-mcp-ide-adapter", "-mcp-proxy", "-mcp-daemon", "-mcp", "-community", "-stable", "-dev", "-beta", "-alpha", "-rc"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(execName, suffix) {
			return strings.TrimSuffix(execName, suffix)
		}
	}
	return execName
}

// GetToolPrefix returns the prefix for built-in tools
// Format: "{brand}_" (e.g., "zqk_", "mybrand_")
func GetToolPrefix() string {
	return GetBrandPrefix() + "_"
}

// GetToolName creates a tool name with the brand prefix
// Example: GetToolName("object_list") -> "zqk_object_list" (if executable is "zqk")
func GetToolName(toolSuffix string) string {
	return GetToolPrefix() + toolSuffix
}

// GetCommandPath creates a command path with the executable name
// Example: GetCommandPath("object list") -> "zqk object list" (if executable is "zqk")
func GetCommandPath(commandPath string) string {
	brandPrefix := GetBrandPrefix()
	// If command path already starts with the brand prefix, return as-is
	if strings.HasPrefix(commandPath, brandPrefix+" ") {
		return commandPath
	}
	// Otherwise, prepend the brand prefix
	return brandPrefix + " " + commandPath
}

// NormalizeCommandPath removes the executable name prefix from a command path
// This is used when command paths come in with the executable name but we need just the command
// Example: NormalizeCommandPath("zqk object list") -> "object list"
func NormalizeCommandPath(commandPath string) string {
	commandPath = strings.TrimSpace(commandPath)
	brandPrefix := GetBrandPrefix()

	// Remove brand prefix if present
	if strings.HasPrefix(commandPath, brandPrefix+" ") {
		return strings.TrimPrefix(commandPath, brandPrefix+" ")
	}

	return commandPath
}
