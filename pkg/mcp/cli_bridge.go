package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp/mcp_helpers" // Added import
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// DiscoveredCommand represents a discovered CLI command with metadata
type DiscoveredCommand struct {
	Use         string
	Short       string
	Long        string
	Command     *cobra.Command
	Subcommands []*DiscoveredCommand
	Path        string   // Full command path (e.g., "object list")
	Permissions []string // Required permissions (e.g., ["read:backlog_item"])
	Roles       []string // Required roles (e.g., ["admin"])
}

var (
	commandsDiscoveredTotal atomic.Int64
)

// GetCLIBridgeStats returns lifetime counter for total discovered CLI commands.
func GetCLIBridgeStats() (discovered int64) {
	return commandsDiscoveredTotal.Load()
}

// DiscoverCLICommands discovers all CLI commands from the root command
// Returns a flat list of all discoverable commands with their metadata
func DiscoverCLICommands(rootCmd *cobra.Command) []*DiscoveredCommand {
	var commands []*DiscoveredCommand
	discoverCommandsRecursive(rootCmd, "", &commands)
	commandsDiscoveredTotal.Add(int64(len(commands)))
	return commands
}

// discoverCommandsRecursive recursively discovers commands and builds command tree
func discoverCommandsRecursive(cmd *cobra.Command, path string, commands *[]*DiscoveredCommand) {
	// Skip hidden commands
	if cmd.Hidden {
		return
	}

	// Build command path
	currentPath := path
	if currentPath == emptyValue {
		currentPath = cmd.Use
	} else {
		currentPath = currentPath + " " + cmd.Use
	}

	// Extract permissions and roles from command annotations in parallel
	// These are independent read-only operations on cmd.Annotations
	var permissions, roles []string
	var wg sync.WaitGroup
	goroutinelabels.NewGoroutine("cli_permissions_extractor", "extracting command permissions").
		WithWaitGroup(&wg).
		StartSimple(func() {
			permissions = extractPermissions(cmd)
		})
	goroutinelabels.NewGoroutine("cli_roles_extractor", "extracting command roles").
		WithWaitGroup(&wg).
		StartSimple(func() {
			roles = extractRoles(cmd)
		})
	wg.Wait()

	// Create discovered command
	discovered := &DiscoveredCommand{
		Use:         cmd.Use,
		Short:       cmd.Short,
		Long:        cmd.Long,
		Command:     cmd,
		Path:        currentPath,
		Permissions: permissions,
		Roles:       roles,
	}

	// Discover subcommands in parallel
	// Each subcommand discovery is independent and read-only on the command tree
	subcommands := cmd.Commands()
	if len(subcommands) > 0 {
		type subCmdResult struct {
			discovered  *DiscoveredCommand
			subCommands []*DiscoveredCommand
		}
		results := make([]subCmdResult, len(subcommands))
		var wg sync.WaitGroup

		for i, subCmd := range subcommands {
			i, subCmd := i, subCmd // Capture loop variables
			goroutinelabels.NewGoroutine(fmt.Sprintf("cli_subcommand_discoverer_%d", i), fmt.Sprintf("discovering subcommands for %s", subCmd.Use)).
				WithWaitGroup(&wg).
				StartSimple(func() {
					// Discover subcommands recursively using local slice to avoid races
					var localCommands []*DiscoveredCommand
					discoverCommandsRecursive(subCmd, currentPath, &localCommands)

					// Extract permissions and roles in parallel (independent read-only operations)
					var perms, roles []string
					var permWg sync.WaitGroup
					goroutinelabels.NewGoroutine("cli_subcmd_permissions", fmt.Sprintf("extracting permissions for %s", subCmd.Use)).
						WithWaitGroup(&permWg).
						StartSimple(func() {
							perms = extractPermissions(subCmd)
						})
					goroutinelabels.NewGoroutine("cli_subcmd_roles", fmt.Sprintf("extracting roles for %s", subCmd.Use)).
						WithWaitGroup(&permWg).
						StartSimple(func() {
							roles = extractRoles(subCmd)
						})
					permWg.Wait()

					// Build discovered command
					discoveredSubCmd := &DiscoveredCommand{
						Use:         subCmd.Use,
						Short:       subCmd.Short,
						Long:        subCmd.Long,
						Command:     subCmd,
						Path:        currentPath + " " + subCmd.Use,
						Permissions: perms,
						Roles:       roles,
					}

					results[i] = subCmdResult{
						discovered:  discoveredSubCmd,
						subCommands: localCommands,
					}
				})
		}
		wg.Wait()

		// Append discovered commands and subcommands (safe - all goroutines completed)
		for _, result := range results {
			*commands = append(*commands, result.subCommands...)
			discovered.Subcommands = append(discovered.Subcommands, result.discovered)
		}
	}

	// Add to list if it's a leaf command (has RunE or Run) or has subcommands
	if cmd.RunE != nil || cmd.Run != nil || len(discovered.Subcommands) > 0 {
		*commands = append(*commands, discovered)
	}
}

// extractPermissions is a convenience wrapper around ExtractPermissionsFromCommand.
// Kept for backward compatibility with existing code.
func extractPermissions(cmd *cobra.Command) []string {
	return ExtractPermissionsFromCommand(cmd)
}

// extractRoles is a convenience wrapper around ExtractRolesFromCommand.
// Kept for backward compatibility with existing code.
func extractRoles(cmd *cobra.Command) []string {
	return ExtractRolesFromCommand(cmd)
}

// FilterCommandsByPermissions filters commands based on security context
// Only returns commands that the user has permission to execute
// Parallelizes permission checks for independent commands
func FilterCommandsByPermissions(commands []*DiscoveredCommand, secCtx *pkgctx.SecurityContext) []*DiscoveredCommand {
	if len(commands) == 0 {
		return []*DiscoveredCommand{}
	}

	// Process commands in parallel - each permission check is independent
	type cmdResult struct {
		cmd      *DiscoveredCommand
		hasPerm  bool
		filtered []*DiscoveredCommand
	}
	results := make([]cmdResult, len(commands))
	var wg sync.WaitGroup

	for i, cmd := range commands {
		i, cmd := i, cmd // Capture loop variables
		goroutinelabels.NewGoroutine(fmt.Sprintf("cli_permission_checker_%d", i), fmt.Sprintf("checking permissions for command %s", cmd.Use)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				hasPerm := hasPermission(cmd, secCtx)

				// Filter subcommands in parallel only if parent command has permission
				// This matches the original sequential behavior exactly
				var filteredSubcommands []*DiscoveredCommand
				if hasPerm && len(cmd.Subcommands) > 0 {
					// Process subcommand permission checks in parallel
					type subResult struct {
						subCmd  *DiscoveredCommand
						hasPerm bool
					}
					subResults := make([]subResult, len(cmd.Subcommands))
					var subWg sync.WaitGroup

					for j, subCmd := range cmd.Subcommands {
						j, subCmd := j, subCmd // Capture loop variables
						goroutinelabels.NewGoroutine(fmt.Sprintf("cli_subcmd_permission_checker_%d_%d", i, j), fmt.Sprintf("checking permissions for subcommand %s", subCmd.Use)).
							WithWaitGroup(&subWg).
							StartSimple(func() {
								subResults[j] = subResult{
									subCmd:  subCmd,
									hasPerm: hasPermission(subCmd, secCtx),
								}
							})
					}
					subWg.Wait()

					// Collect filtered subcommands in order
					for _, result := range subResults {
						if result.hasPerm {
							filteredSubcommands = append(filteredSubcommands, result.subCmd)
						}
					}
				}

				results[i] = cmdResult{
					cmd:      cmd,
					hasPerm:  hasPerm,
					filtered: filteredSubcommands,
				}
			})
	}
	wg.Wait()

	// Collect filtered commands in original order (all goroutines done, safe to append)
	// Maintain deterministic ordering by processing results in original index order
	// Create a copy of the command to avoid mutating the original (important for test isolation)
	filtered := make([]*DiscoveredCommand, 0, len(commands))
	for _, result := range results {
		if result.hasPerm {
			// Create a shallow copy to avoid mutating the original command object
			// This is important for test isolation and when the same commands slice
			// is filtered multiple times with different security contexts
			cmdCopy := *result.cmd
			cmdCopy.Subcommands = result.filtered
			filtered = append(filtered, &cmdCopy)
		}
	}

	return filtered
}

// hasPermission is defined in cli_bridge_permissions.go

// ConvertCommandToMCPTool converts a discovered command to an MCP tool
func ConvertCommandToMCPTool(cmd *DiscoveredCommand) Tool {
	// Build input schema from command flags
	properties := make(map[string]any)
	required := []string{}

	// Extract positional arguments from Use string (e.g. "next [task_id]")
	if cmd.Command != nil {
		useParts := strings.Fields(cmd.Command.Use)
		for _, part := range useParts {
			cleanPart := part
			isPositional := false
			if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
				cleanPart = part[1 : len(part)-1]
				isPositional = true
			} else if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
				cleanPart = part[1 : len(part)-1]
				isPositional = true
			}

			if isPositional && cleanPart != "" {
				prop := map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: fmt.Sprintf("Positional argument: %s", cleanPart),
				}
				properties[cleanPart] = prop
				if strings.HasPrefix(part, "<") || cleanPart == "task_id" || cleanPart == "id" {
					required = append(required, cleanPart)
				}
			}
		}
	}

	// Extract flags from command
	if cmd.Command != nil {
		cmd.Command.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Hidden {
				return
			}

			prop := make(map[string]any)
			prop[objects.FieldKeyDescription] = flag.Usage

			// Determine type from flag value type
			switch flag.Value.Type() {
			case "bool":
				prop[objects.FieldKeyType] = "boolean"
				if flag.DefValue == "true" {
					prop["default"] = true
				} else {
					prop["default"] = false
				}
			case "int", "int8", "int16", "int32", "int64":
				prop[objects.FieldKeyType] = "number"
			case "uint", "uint8", "uint16", "uint32", "uint64":
				prop[objects.FieldKeyType] = "number"
			case "float32", "float64":
				prop[objects.FieldKeyType] = "number"
			case "string":
				prop[objects.FieldKeyType] = "string"
				if flag.DefValue != emptyValue {
					prop["default"] = flag.DefValue
				}
			case "stringSlice", "stringArray":
				prop[objects.FieldKeyType] = "array"
				prop["items"] = map[string]any{objects.FieldKeyType: "string"}
			default:
				prop[objects.FieldKeyType] = "string"
			}

			properties[flag.Name] = prop

			// Check if required (no default and not a bool)
			// Note: cobra doesn't have a standard way to mark flags as required via annotations
			// This would need to be done via command validation logic
			// For now, we don't mark flags as required in the schema
		})
	}

	// Inject special properties for object create/update commands
	if strings.HasPrefix(cmd.Path, "object create") || strings.HasPrefix(cmd.Path, "object update") {
		properties[objects.FieldKeyID] = map[string]any{
			objects.FieldKeyType:        "string",
			objects.FieldKeyDescription: "Object ID",
		}
		properties["fields"] = map[string]any{
			objects.FieldKeyType:        "object",
			objects.FieldKeyDescription: "Key-value map of fields to set",
			"additionalProperties":      true,
		}
		// If object update, id is usually required. If create, it might be auto-generated.
		if strings.HasPrefix(cmd.Path, "object update") {
			required = append(required, "id")
		}
	}

	// Add command path as a property
	properties["_command_path"] = map[string]any{
		objects.FieldKeyType:        "string",
		objects.FieldKeyDescription: "Internal command path (auto-filled)",
		"default":                   cmd.Path,
	}

	// Build description
	description := cmd.Short
	if cmd.Long != emptyValue {
		description = cmd.Long
	}

	return Tool{
		Name:        mcp_helpers.SanitizeToolName(cmd.Path),
		Description: description,
		InputSchema: map[string]any{
			objects.FieldKeyType: "object",
			"properties":         properties,
			"required":           required,
		},
	}
}

// sanitizeToolName converts a command path to a valid MCP tool name
// e.g., "object list" -> "object_list"
// Removes flag placeholders and optional flags to keep names short
// IDE has a 60-character limit for "server:tool_name", so we limit to 53 chars
func sanitizeToolName(path string) string {
	// Remove flag placeholders like <id1,id2,...>, <file>, etc.
	// Pattern: <...> or <...|...>
	path = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(path, "")

	// Remove optional flags like [--cascade]
	path = regexp.MustCompile(`\[[^\]]+\]`).ReplaceAllString(path, "")

	// Remove pipe separators like " | "
	path = strings.ReplaceAll(path, " | ", " ")

	// Remove multiple spaces
	path = regexp.MustCompile(`\s+`).ReplaceAllString(path, " ")

	// Trim spaces
	path = strings.TrimSpace(path)

	// Replace spaces with underscores
	name := strings.ReplaceAll(path, " ", "_")

	// IDE limit: 60 chars total for "server:tool_name"
	// Tool name max length (accounting for server: prefix)
	const maxToolNameLength = 53
	if len(name) > maxToolNameLength {
		// Truncate but try to preserve meaningful parts
		// Keep the first part (usually the command group) and truncate the rest
		parts := strings.Split(name, "_")
		if len(parts) > 1 {
			// Keep first part and as much of the rest as fits
			result := parts[0]
			for i := 1; i < len(parts); i++ {
				candidate := result + "_" + parts[i]
				if len(candidate) <= maxToolNameLength {
					result = candidate
				} else {
					break
				}
			}
			name = result
		} else {
			// Single part, just truncate
			name = name[:maxToolNameLength]
		}
	}

	return name
}

// ExecuteCLICommandViaMCP executes a CLI command via MCP with proper context
// This runs the actual CLI binary with the command and returns the result
//
// Deprecated: Use ExecuteCLICommandViaMCPWithContext instead
func ExecuteCLICommandViaMCP(args map[string]any, secCtx *pkgctx.SecurityContext, projectRoot string) (any, error) {
	// Create minimal context for backward compatibility
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: projectRoot,
	}
	return ExecuteCLICommandViaMCPWithContext(pkgctx.NewSystemContext(), args, secCtx, initCtx)
}

// ExecuteCLICommandViaMCPWithContext executes a CLI command via MCP with context for cancellation
// initCtx provides CLI initialization context (project root, etc.) - always pass context objects, never extract values
func ExecuteCLICommandViaMCPWithContext(ctx context.Context, args map[string]any, secCtx *pkgctx.SecurityContext, initCtx *pkgctx.CliInitializationContext) (any, error) {
	// Extract and normalize command path
	commandPath, err := extractCommandPath(args)
	if err != nil {
		return nil, err
	}

	// Build command arguments (positional args + flags)
	// Server is nil here, so defaults will be used (can be made configurable in future)
	cmdArgs := buildCommandArguments(commandPath, args, nil)

	// Find executable binary
	execPath, err := findExecutableBinary()
	if err != nil {
		return nil, errfmt.Newf("failed to find executable binary").Wrap(err)
	}

	// Build isolated environment for subprocess (no parent PID when no server)
	projectRoot := initCtx.GetProjectRoot()
	env := buildCommandEnvironment(secCtx, projectRoot, 0, nil)

	// Execute command (no process group when called without server - e.g. tests)
	stdout, stderr, execErr := executeCommand(ctx, execPath, cmdArgs, env, projectRoot, nil, "")

	// Filter output to remove logging pollution
	output := filterCommandOutput(stdout.String())

	// Parse filtered output as JSON
	result := parseCommandOutput(output, stderr)

	// Build final result with metadata
	result = buildCommandResult(result, commandPath, cmdArgs, stdout, stderr, execErr)

	return result, execErr
}

// isUnsafeCLIBridgeBinary reports paths that must never be used as the CLI
// subprocess for MCP bridging. go test binaries re-exec'd with CLI args can
// re-enter the harness and fork-bomb (see diagnostics mcp-test-bomb-*).
func isUnsafeCLIBridgeBinary(path string) bool {
	if path == emptyValue {
		return true
	}
	base := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(base, ".test") {
		return true
	}
	if strings.Contains(path, "go-build") || strings.Contains(path, "___go_build") {
		return true
	}
	return false
}

// acceptCLIBridgeBinary returns absPath when path exists and is safe for CLI bridging.
func acceptCLIBridgeBinary(path string) (string, bool) {
	if isUnsafeCLIBridgeBinary(path) {
		return "", false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	if _, err := fileutil.Stat(absPath); err != nil {
		return "", false
	}
	if isUnsafeCLIBridgeBinary(absPath) {
		return "", false
	}
	return absPath, true
}

// findExecutableBinary finds the executable binary using the current executable name
// This supports white-labeling by using the actual executable name
// It checks multiple locations in order of preference:
// 1. ZQK_STABLE_BINARY_PATH environment variable (set by wrapper script)
// 2. Current directory (./<executable-name> or ./<executable-name>-stable)
func findExecutableBinary() (string, error) {
	execName := GetExecutableName()
	// When the process is a go test binary, GetExecutableName is "<pkg>.test".
	// Prefer the product CLI name for discovery so we never LookPath("mcp.test").
	if isUnsafeCLIBridgeBinary(execName) || strings.HasSuffix(strings.ToLower(execName), ".test") {
		execName = brand.ExecutableName()
	}

	// When we are zqk-mcp (mcp-simple), use the full "zqk" binary for CLI tool execution.
	// zqk-mcp has no subcommands (system status, object list, etc.)—only the MCP server.
	// Otherwise we would spawn "zqk-mcp system status" which starts another MCP server, causing extra processes.
	if execName == "zqk-mcp" {
		if path := findFullZqkBinary(); path != emptyValue {
			return path, nil
		}
	}

	// First, check for stable binary path from environment (set by wrapper script)
	if stablePath := zqkenv.StableBinaryPath().Get(); stablePath != emptyValue {
		if absPath, ok := acceptCLIBridgeBinary(stablePath); ok {
			return absPath, nil
		}
	}

	// Try to find the binary that's currently running (os.Args[0])
	// This ensures subprocesses use the same binary as the MCP server.
	// Never use a go test / go-build binary — regardless of IsInTest() — or we
	// self-exec and fork-bomb (brand.ExecutableName used to be "mcp.test", so
	// findFullZqkBinary returned the test binary next to itself).
	if len(os.Args) > 0 {
		execPath := os.Args[0]
		if isUnsafeCLIBridgeBinary(execPath) || zqkenv.IsInTest() {
			if path := findFullZqkBinary(); path != emptyValue {
				return path, nil
			}
			return "", fmt.Errorf("refusing to use test/go-build binary %q for CLI bridging; no safe zqk binary found", execPath)
		}

		// If it's a relative path, make it absolute
		if !filepath.IsAbs(execPath) {
			if absPath, err := filepath.Abs(execPath); err == nil {
				if accepted, ok := acceptCLIBridgeBinary(absPath); ok {
					return accepted, nil
				}
			}
		} else if accepted, ok := acceptCLIBridgeBinary(execPath); ok {
			return accepted, nil
		}
	}

	// Try current directory (executable name and executable-stable)
	for _, name := range []string{fmt.Sprintf("./%s-stable", execName), fmt.Sprintf("./%s", execName)} {
		if absPath, ok := acceptCLIBridgeBinary(name); ok {
			return absPath, nil
		}
	}

	// Try to find in PATH (executable name and executable-stable)
	for _, name := range []string{fmt.Sprintf("%s-stable", execName), execName} {
		if path, err := exec.LookPath(name); err == nil {
			if absPath, ok := acceptCLIBridgeBinary(path); ok {
				return absPath, nil
			}
		}
	}

	// Also try common executable names for compatibility
	commonNames := []string{brand.ZqkStableName, brand.ExecutableName()}
	for _, name := range commonNames {
		if isUnsafeCLIBridgeBinary(name) {
			continue
		}
		if path, err := exec.LookPath(name); err == nil {
			if absPath, ok := acceptCLIBridgeBinary(path); ok {
				return absPath, nil
			}
		}
	}

	if path := findFullZqkBinary(); path != emptyValue {
		return path, nil
	}

	return "", errfmt.Errorf("executable binary '%s' not found. Checked: ZQK_STABLE_BINARY_PATH, current directory (./%s, ./%s-stable), and PATH (%s, %s-stable, and legacy names)", execName, execName, execName, execName, execName)
}

// findFullZqkBinary returns the path to the full zqk CLI binary when the current process is zqk-mcp.
// Prefer same directory as this binary (e.g. bin/zqk next to bin/zqk-mcp), then PATH.
// Never returns a go test / go-build path (fork-bomb guard).
func findFullZqkBinary() string {
	// Always include the product basename so a poisoned brand.ExecutableName
	// (historically "mcp.test" under go test) cannot be the only candidate.
	names := []string{brand.ZqkStableName, brand.ExecutableName(), "zqk"}
	seen := map[string]struct{}{}
	if self, err := fileutil.Executable(); err == nil {
		dir := filepath.Dir(self)
		for _, name := range names {
			if name == emptyValue {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			if isUnsafeCLIBridgeBinary(name) {
				continue
			}
			candidate := filepath.Join(dir, name)
			if absPath, ok := acceptCLIBridgeBinary(candidate); ok {
				return absPath
			}
		}
	}
	for _, name := range names {
		if name == emptyValue || isUnsafeCLIBridgeBinary(name) {
			continue
		}
		if path, err := exec.LookPath(name); err == nil {
			if absPath, ok := acceptCLIBridgeBinary(path); ok {
				return absPath
			}
		}
	}
	return ""
}

// InitializeSecurityContextFromMCP is defined in cli_bridge_security_context.go

// RegisterCLITools registers all CLI commands as MCP tools based on security context
// This is called during MCP server initialization to bootstrap available tools
//
// Deprecated: Use RegisterCLIToolsWithRootCommand instead
func RegisterCLITools(server *Server, secCtx *pkgctx.SecurityContext, projectRoot string) error {
	rootCmd := getRootCommandForMCP()
	if rootCmd == nil {
		return NewConfigurationError(
			"root command not available - use RegisterCLIToolsWithRootCommand",
			map[string]any{"function": "RegisterCLITools"},
		)
	}
	return RegisterCLIToolsWithRootCommand(server, rootCmd, secCtx, projectRoot)
}

// getRootCommandForMCP gets the root command for MCP tool discovery
// This is a placeholder - in production, the root command should be passed
// to RegisterCLITools or set on the Server struct
func getRootCommandForMCP() *cobra.Command {
	// This function should not be called directly
	// Instead, use RegisterCLIToolsWithRootCommand
	return nil
}

// executeCLICommandWithContext executes a CLI command with context
// This avoids import cycles by handling type assertion here
// Deprecated: Use executeCLICommandWithContextForServer for server-aware execution
func executeCLICommandWithContext(ctx context.Context, args map[string]any, secCtxInterface any, projectRoot string, permissionCache any) (any, error) {
	return executeCLICommandWithContextForServer(ctx, args, secCtxInterface, projectRoot, permissionCache, nil)
}

// logMCPCLIDiagnostic logs exec details when a CLI subprocess is slow or failed (for troubleshooting hangs).
func logMCPCLIDiagnostic(execPath, projectRoot string, cmdArgs []string, duration time.Duration, stderr string, execErr error) {
	const maxStderr = 500
	stderrTrim := stderr
	if len(stderrTrim) > maxStderr {
		stderrTrim = stderrTrim[:maxStderr] + "..."
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))
	fe := logging.Fluent(logger).Warn("MCP CLI exec slow or failed").
		ExecPath(execPath).
		ProjectRoot(projectRoot).
		ExecArgs(strings.Join(cmdArgs, " ")).
		ElapsedString(duration.String()).
		StderrSnippet(strings.TrimSpace(stderrTrim))
	// Use fmt.Sprintf for error to avoid panic when execErr is typed nil (e.g. *exec.ExitError)
	if execErr != nil {
		fe.ErrorSummary(fmt.Sprintf("%v", execErr))
	}
	fe.Log()
}

// executeCLICommandWithContextForServer executes a CLI command with context and server instance
// This allows access to server config for positional argument names
func executeCLICommandWithContextForServer(ctx context.Context, args map[string]any, secCtxInterface any, projectRoot string, permissionCache any, serverInterface any) (any, error) {
	secCtx, ok := secCtxInterface.(*pkgctx.SecurityContext)
	if !ok {
		return nil, errfmt.Errorf("invalid security context type")
	}

	// Extract server instance if provided
	var server *Server
	if serverInterface != nil {
		server, _ = serverInterface.(*Server)
	}

	// Create minimal context for backward compatibility (when called with string)
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: projectRoot,
	}

	// Use server-aware execution if server is available
	if server != nil {
		// Extract command path and build arguments with server config
		commandPath, err := extractCommandPath(args)
		if err != nil {
			return nil, err
		}
		cmdArgs := buildCommandArguments(commandPath, args, server)

		// In-process path: use dispatch loop when runner is set (no subprocess; progress to Coordinator)
		if runner := server.GetInProcessCLIRunner(); runner != nil {
			result, execErr := runner(ctx, commandPath, cmdArgs)
			finalResult := result
			if permissionCache != nil && result != nil {
				if resultMap, ok := result.(map[string]any); ok {
					finalResult = applyObjectAccessControl(resultMap, secCtx, permissionCache)
				}
			}
			return finalResult, execErr
		}

		// Subprocess path
		// Find executable binary
		execPath, err := findExecutableBinary()
		if err != nil {
			return nil, errfmt.Newf("failed to find executable binary").Wrap(err)
		}

		// Build isolated environment for subprocess; set ZQK_PARENT_PID so child exits when we die (all platforms)
		env := buildCommandEnvironment(secCtx, projectRoot, os.Getpid(), server)

		// Execute command with isolated environment; register subprocess so shutdown kills it (no orphans)
		pgm := server.GetProcessGroupManager()
		start := time.Now()
		stdout, stderr, execErr := executeCommand(ctx, execPath, cmdArgs, env, projectRoot, pgm, commandPath)
		duration := time.Since(start)

		// Diagnostic: log when CLI exec is slow or failed so we can troubleshoot hangs.
		// Use 1s threshold so we capture stderr and args even on "normal" slow first-call (subprocess init).
		if duration > 1*time.Second || execErr != nil {
			logMCPCLIDiagnostic(execPath, projectRoot, cmdArgs, duration, stderr.String(), execErr)
			if tw := server.getTraceWriter(); tw != nil {
				server.traceLogf("[MCP_CLI_DIAG] path=%s dir=%s args=%v duration=%v stderr_len=%d err=%v",
					execPath, projectRoot, cmdArgs, duration, len(stderr.String()), execErr)
			}
		}

		// Filter output to remove logging pollution
		output := filterCommandOutput(stdout.String())

		// Parse filtered output as JSON
		result := parseCommandOutput(output, stderr)

		// Build final result with metadata
		result = buildCommandResult(result, commandPath, cmdArgs, stdout, stderr, execErr)

		// Apply access control to result if permission cache is available
		var finalResult any = result
		if permissionCache != nil && result != nil {
			finalResult = applyObjectAccessControl(result, secCtx, permissionCache)
		}

		return finalResult, execErr
	}

	// Fallback to original implementation when server not available
	result, err := ExecuteCLICommandViaMCPWithContext(ctx, args, secCtx, initCtx)
	if err != nil {
		return result, err
	}

	// Apply access control to result if permission cache is available
	var finalResult = result
	if permissionCache != nil && result != nil {
		if resultMap, ok := result.(map[string]any); ok {
			finalResult = applyObjectAccessControl(resultMap, secCtx, permissionCache)
		} else {
			finalResult = result
		}
	}

	return finalResult, err
}

// bootstrapCLIToolsForServerWithConfig bootstraps CLI tools for a server with config
func bootstrapCLIToolsForServerWithConfig(server *Server, rootCmdInterface, secCtxInterface any, projectRoot string, config *ServerConfig) error {
	rootCmd, ok := rootCmdInterface.(*cobra.Command)
	if !ok {
		return errfmt.Errorf("invalid root command type")
	}
	secCtx, ok := secCtxInterface.(*pkgctx.SecurityContext)
	if !ok {
		return errfmt.Errorf("invalid security context type")
	}
	return RegisterCLIToolsWithRootCommandAndConfig(server, rootCmd, secCtx, projectRoot, config)
}

// RegisterCLIToolsWithRootCommand registers CLI commands as MCP tools
// This is the preferred way to register CLI tools - pass the root command explicitly
//
// Deprecated: Use RegisterCLIToolsWithRootCommandAndConfig instead
func RegisterCLIToolsWithRootCommand(server *Server, rootCmd *cobra.Command, secCtx *pkgctx.SecurityContext, projectRoot string) error {
	return RegisterCLIToolsWithRootCommandAndConfig(server, rootCmd, secCtx, projectRoot, nil)
}

// RegisterCLIToolsWithRootCommandAndConfig is defined in cli_bridge_registration.go

// applyObjectAccessControl is defined in cli_bridge_access_control.go
