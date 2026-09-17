package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/handslapper"
)

// extractCommandPath is a convenience wrapper around ExtractCommandPath.
// Kept for backward compatibility with existing code.
func extractCommandPath(args map[string]any) (string, error) {
	return ExtractCommandPath(args)
}

// buildCommandArguments builds the command line arguments from tool arguments
// Separates positional arguments from flags and orders them correctly
func buildCommandArguments(commandPath string, args map[string]any, server *Server) []string {
	parts := strings.Fields(commandPath)
	cmdArgs := make([]string, 0, len(parts)+len(args))
	cmdArgs = append(cmdArgs, parts...)

	// Extract positional arguments and flags separately
	positionalArgs := ExtractPositionalArguments(args, server)
	flags := ExtractFlags(args, server)

	// Insert positional args after command parts but before flags
	flagStartIndex := findFlagStartIndex(cmdArgs)
	if len(positionalArgs) > 0 {
		newArgs := make([]string, 0, len(cmdArgs)+len(positionalArgs))
		newArgs = append(newArgs, cmdArgs[:flagStartIndex]...)
		newArgs = append(newArgs, positionalArgs...)
		newArgs = append(newArgs, cmdArgs[flagStartIndex:]...)
		cmdArgs = newArgs
	}

	// Add flags
	cmdArgs = append(cmdArgs, flags...)

	// Add MCP context profile
	cmdArgs = append(cmdArgs, "--context", DefaultMCPContext)

	return cmdArgs
}

// extractPositionalArguments is a convenience wrapper around ExtractPositionalArguments.
// Kept for backward compatibility with existing code.
func extractPositionalArguments(args map[string]any, server *Server) []string {
	return ExtractPositionalArguments(args, server)
}

// extractFlags is a convenience wrapper around ExtractFlags.
// Kept for backward compatibility with existing code.
func extractFlags(args map[string]any, server *Server) []string {
	return ExtractFlags(args, server)
}

// findFlagStartIndex finds the index where flags start in command arguments
// Flags are arguments starting with "--"
func findFlagStartIndex(cmdArgs []string) int {
	for i, arg := range cmdArgs {
		if strings.HasPrefix(arg, "--") {
			return i
		}
	}
	return len(cmdArgs) // No flags found, insert at end
}

// buildCommandEnvironment builds the environment variables for command execution.
// Isolates MCP-specific variables to prevent environment pollution.
// parentPID: when > 0, the caller is the MCP server and we set ZQK_PARENT_PID so the child
// can exit when the parent dies (platform-independent orphan prevention).
func buildCommandEnvironment(secCtx *pkgctx.SecurityContext, projectRoot string, parentPID int, server *Server) []string {
	// Start with inherited environment
	env := os.Environ()

	// Remove any existing MCP-specific and parent-PID variables to prevent pollution
	filteredEnv := make([]string, 0, len(env))
	mcpPfx := zqkenv.MCPEnvKeyPrefix()
	parentPfx := zqkenv.ParentPID().Name() + "="
	sessionPfx := zqkenv.SessionID().Name() + "="
	for _, e := range env {
		if !strings.HasPrefix(e, mcpPfx) && !strings.HasPrefix(e, parentPfx) && !strings.HasPrefix(e, sessionPfx) {
			filteredEnv = append(filteredEnv, e)
		}
	}
	env = filteredEnv

	// Add MCP-specific environment variables for subprocess only
	if secCtx != nil {
		env = append(env,
			fmt.Sprintf("%s=%s", zqkenv.MCPAccountID(), secCtx.AccountID),
			fmt.Sprintf("%s=%s", zqkenv.MCPRoles(), strings.Join(secCtx.Roles, ",")),
			fmt.Sprintf("%s=%s", zqkenv.MCPPermissions(), strings.Join(secCtx.Permissions, ",")))
	}

	// Attach active MCP session ID for subprocess audit events
	if server != nil {
		if sessionID := server.GetCurrentSessionID(); sessionID != "" {
			env = append(env, fmt.Sprintf("%s=%s", zqkenv.SessionID(), sessionID))
		}
	}

	// Always set project root
	env = append(env, fmt.Sprintf("%s=%s", zqkenv.ProjectRoot().Name(), projectRoot))

	// Platform-independent orphan prevention: child exits when parent (MCP server) dies
	if parentPID > 0 {
		env = append(env, fmt.Sprintf("%s=%d", zqkenv.ParentPID(), parentPID))
	}

	// MCP tool children are spawned from zqk-mcp-daemon (role symlink), so IsParentZqk()
	// name-equality fails and the idle watchdog cancels OperationContext mid-Count.
	// TRACK: BLI-1784969955962654000-dc689643 — remove when IsParentZqk treats role binaries.
	env = append(env, fmt.Sprintf("%s=1", zqkenv.IsParentZqk()))

	// Disable interactive prompts and editors in MCP subprocesses
	env = append(env, "ZQK_NON_INTERACTIVE=1", "NO_COLOR=1", "EDITOR=false")

	return env
}

// executeCommand executes the CLI command and captures output.
// CRITICAL: Uses CommandContext so the process is killed when ctx is cancelled (e.g. on server shutdown).
// When pgm is non-nil, the subprocess is registered so ProcessGroupManager.Shutdown() can kill it
// even if the cancellation path hasn't run yet (deterministic process tree, no orphans).
func executeCommand(ctx context.Context, execPath string, cmdArgs []string, env []string, projectRoot string, pgm *ProcessGroupManager, subprocessName string) (stdout, stderr bytes.Buffer, err error) {
	// HandSlapper Integration: Block kernel bypass attempts
	hs := handslapper.NewService(true) // strict mode enabled
	fullCmd := execPath + " " + strings.Join(cmdArgs, " ")
	if driftErr := hs.MonitorCommand(ctx, "mcp-agent", fullCmd); driftErr != nil {
		return bytes.Buffer{}, bytes.Buffer{}, driftErr
	}

	cmd := execwrap.CommandContext(ctx, execPath, cmdArgs...)
	cmd.Env = env
	cmd.Dir = projectRoot

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	// Use empty stdin so the subprocess never blocks on the MCP JSON-RPC pipe.
	// Commands that need stdin (e.g. object create from pipe) get EOF and must use --file or --data.
	cmd.Stdin = bytes.NewReader(nil)

	// Start the command
	if err := cmd.Start(); err != nil {
		return stdoutBuf, stderrBuf, err
	}

	// Register with process group manager so shutdown kills this subprocess (no orphans)
	var subprocessID string
	if pgm != nil && cmd.Process != nil {
		subprocessID = fmt.Sprintf("cli-%s-%d", strings.ReplaceAll(subprocessName, " ", "_"), time.Now().UnixNano())
		name := subprocessName
		if name == emptyValue {
			name = "CLI tool"
		}
		process := cmd.Process
		pgm.RegisterSubprocess(subprocessID, name, "CLI command invoked via MCP tool", process.Pid, false, func() error {
			if process != nil {
				return process.Kill()
			}
			return nil
		})
		defer pgm.UnregisterSubprocess(subprocessID)
	}

	// Wait for command to complete
	// CommandContext will kill the process if ctx is cancelled
	err = cmd.Wait()

	// If context was cancelled, ensure process is killed
	if ctx.Err() != nil && cmd.Process != nil {
		_ = cmd.Process.Kill() //nolint:errcheck // Best effort cleanup
	}

	return stdoutBuf, stderrBuf, err
}

// filterCommandOutput filters stdout to remove logging pollution
// Removes fallback log messages and debug log JSON objects
func filterCommandOutput(output string) string {
	if output == emptyValue {
		return output
	}

	// First, filter out fallback log messages (lines starting with [warn] or [error])
	output = filterFallbackLogMessages(output)

	// Then, filter out debug log JSON objects
	output = filterDebugLogObjects(output)

	// Finally, extract the first complete JSON object if multiple exist
	output = extractFirstCompleteJSONObject(output)

	return output
}

// filterFallbackLogMessages removes fallback log messages from output
// Fallback messages start with [warn] or [error] and should never be on stdout
func filterFallbackLogMessages(output string) string {
	lines := strings.Split(output, "\n")
	filteredLines := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[warn]") || strings.HasPrefix(trimmed, "[error]") {
			continue // Skip fallback log messages
		}
		filteredLines = append(filteredLines, line)
	}
	return strings.Join(filteredLines, "\n")
}

// filterDebugLogObjects removes debug log JSON objects from output
// Returns the last valid (non-debug) JSON object found
func filterDebugLogObjects(output string) string {
	return FilterDebugLogObjectsFromOutput(output)
}

// extractFirstCompleteJSONObject is a convenience wrapper around ExtractFirstCompleteJSONObject.
// Kept for backward compatibility with existing code.
func extractFirstCompleteJSONObject(output string) string {
	return ExtractFirstCompleteJSONObject(output)
}

// parseCommandOutput parses the filtered command output as JSON.
// Primary source is stdout (output). If stdout is empty and stderr looks like JSON
// (e.g. CLI previously wrote result to stderr), parses stderr as a fallback so
// MCP tools do not get empty_output when the result is on the wrong stream.
// Returns a result map with error information if parsing fails.
func parseCommandOutput(output string, stderr bytes.Buffer) map[string]any {
	toParse := output
	if toParse == emptyValue && stderr.Len() > 0 {
		stderrStr := strings.TrimSpace(stderr.String())
		// Only treat stderr as result if it looks like a single JSON object (CLI result)
		if strings.HasPrefix(stderrStr, "{") {
			toParse = stderrStr
		}
	}
	if toParse == emptyValue {
		builder := NewCommandResultBuilder("", nil).
			WithError(errfmt.Errorf("command output was empty")).
			WithStderr(stderr).
			WithErrorCode(InvalidParameter).
			WithErrorType("empty_output").
			WithData("output", "")
		if stderr.Len() > 0 {
			stderrStr := stderr.String()
			if strings.Contains(stderrStr, "[warn]") || strings.Contains(stderrStr, "[error]") {
				builder.WithData("warning", "Command output was empty - fallback log messages detected in stderr")
			}
		}
		return builder.Build()
	}

	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(toParse))
	if err := decoder.Decode(&result); err != nil {
		return buildParseErrorResult(toParse, stderr, err)
	}
	return result
}

// buildParseErrorResult builds an error result when JSON parsing fails
// Uses CommandResultBuilder for consistent error structure
func buildParseErrorResult(output string, stderr bytes.Buffer, parseErr error) map[string]any {
	// Create builder (command metadata will be added later by buildCommandResult)
	builder := NewCommandResultBuilder("", nil).
		WithSuccess(false).
		WithError(parseErr).
		WithStderr(stderr).
		WithErrorCode(InvalidParameter). // JSON parsing failure is an invalid parameter/format issue
		WithErrorType("parse_error").
		WithData("parse_error", parseErr.Error())

	// Check if output still contains fallback messages (shouldn't happen after filtering)
	if strings.Contains(output, "[warn]") || strings.Contains(output, "[error]") {
		return builder.
			WithData("output", "").         // Empty because output is polluted/unreliable
			WithData("raw_output", output). // Include polluted output for debugging
			WithData("warning", "stdout pollution detected - fallback messages could not be filtered").
			WithData("note", "JSON parsing failed due to stdout pollution from logging framework. See 'raw_output' for the actual (polluted) stdout content.").
			Build()
	}

	// JSON parsing failed for non-polluted output (e.g. CLI emitted plain text/table under --context mcp)
	return builder.
		WithData("output", output).
		WithData("note", "JSON parsing failed - command output was not valid JSON").
		Build()
}

// buildCommandResult builds the final result map with metadata
// Uses CommandResultBuilder for consistent structure and error handling
func buildCommandResult(result map[string]any, commandPath string, cmdArgs []string, stdout, stderr bytes.Buffer, execErr error) map[string]any {
	builder := NewCommandResultBuilder(commandPath, cmdArgs).
		WithResult(result).
		WithStdout(stdout).
		WithStderr(stderr)

	if execErr != nil {
		builder.WithError(execErr)
	} else if parseErr, isParseErr := result["error_type"].(string); isParseErr && parseErr == "parse_error" {
		builder.WithSuccess(false)
	} else if successVal, hasSuccess := result["success"].(bool); hasSuccess && !successVal {
		builder.WithSuccess(false)
	} else {
		builder.WithSuccess(true)
	}

	return builder.Build()
}

// BuildCLICommandResultFromOutput builds the same result map as the subprocess path from raw stdout/stderr.
// Used by in-process CLI execution (e.g. from cmd/zqk) so the runner can return the same shape without
// pkg/mcp importing pkg/dispatch. See docs/architecture/DISPATCH_LOOP_AND_MCP.md.
func BuildCLICommandResultFromOutput(commandPath string, cmdArgs []string, stdoutStr, stderrStr string, execErr error) map[string]any {
	stdoutBuf := bytes.Buffer{}
	stdoutBuf.WriteString(stdoutStr)
	stderrBuf := bytes.Buffer{}
	stderrBuf.WriteString(stderrStr)
	output := filterCommandOutput(stdoutStr)
	parsed := parseCommandOutput(output, stderrBuf)
	return buildCommandResult(parsed, commandPath, cmdArgs, stdoutBuf, stderrBuf, execErr)
}
