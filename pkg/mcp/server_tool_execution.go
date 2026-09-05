package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// handleToolCallWithContext handles a tool call with context for cancellation
//
//nolint:gocyclo // High complexity from tool dispatch switch; refactor into router if needed
func (s *Server) handleToolCallWithContext(ctx context.Context, rawName string, args map[string]any) (any, error) {
	ctx = EnsureContext(ctx)

	name := strings.ReplaceAll(rawName, " ", "_")

	// Emit tool started event
	if s.eventEmitter != nil {
		s.eventEmitter.Emit(&Event{
			Type:    EventTypeToolStarted,
			Message: fmt.Sprintf("Tool execution started: %s", name),
			Fields: map[string]any{
				"tool": name,
			},
			Severity: "info",
		})
	}

	// Dispatch built-in tools by name first so they are not mistaken for CLI commands.
	// E.g. zqk_get_current_backlog_item must run HandleGetCurrentBacklogItem (object list under the hood),
	// not "zqk get current backlog item" which does not exist.
	toolPrefix := GetToolPrefix()
	toolSuffix := name
	if strings.HasPrefix(name, toolPrefix) {
		toolSuffix = strings.TrimPrefix(name, toolPrefix)
	}
	switch {
	case name == GetToolName("graph_traversal") || toolSuffix == "graph_traversal" || name == "graph_traversal":
		return HandleGraphTraversal(ctx, args)
	case name == GetToolName("resolve_references") || toolSuffix == "resolve_references" || name == "resolve_references":
		return HandleResolveReferences(ctx, args)
	case name == GetToolName("state_aware_query") || toolSuffix == "state_aware_query" || name == "state_aware_query":
		return HandleStateAwareQuery(ctx, args)
	case name == GetToolName("test_echo") || toolSuffix == "test_echo" || name == "test_echo" || name == "echo":
		return HandleEcho(ctx, args)
	case name == GetToolName("chat_send") || toolSuffix == "chat_send" || name == "chat_send":
		projectRoot := s.GetProjectRoot()
		return HandleChatInject(ctx, s, args, projectRoot)
	case name == GetToolName("ide_bridge_request") || toolSuffix == "ide_bridge_request" || name == "ide_bridge_request":
		return HandleIdeBridgeRequest(ctx, s, args, s.GetProjectRoot())
	case name == GetToolName("execute_bash") || toolSuffix == "execute_bash" || name == "execute_bash":
		return s.handleAgentExecuteBashTool(ctx, args)
	case name == GetToolName("read_file") || toolSuffix == "read_file" || name == "read_file":
		return s.handleAgentReadFileTool(ctx, args)
	case name == GetToolName("write_file") || toolSuffix == "write_file" || name == "write_file":
		return s.handleAgentWriteFileTool(ctx, args)
	case name == GetToolName("read_code") || toolSuffix == "read_code" || name == "read_code":
		return s.handleAgentReadCodeTool(ctx, args)
	case name == GetToolName("observer_search") || toolSuffix == "observer_search" || name == "observer_search":
		return s.handleObserverSearchTool(ctx, args)
	case name == GetToolName("write_code") || toolSuffix == "write_code" || name == "write_code":
		return s.handleAgentWriteCodeTool(ctx, args)
	case name == GetToolName("trigger_verification") || toolSuffix == "trigger_verification" || name == "trigger_verification":
		return s.handleAgentTriggerVerificationTool(ctx, args)
	case name == GetToolName("object_list") || toolSuffix == "object_list" || name == "object_list":
		return HandleObjectList(ctx, s, args)
	case name == GetToolName("object_get") || toolSuffix == "object_get" || name == "object_get":
		return HandleObjectGet(ctx, s, args)
	case name == GetToolName("object_count") || toolSuffix == "object_count" || name == "object_count":
		return HandleObjectCount(ctx, s, args)
	case name == GetToolName("system_status") || toolSuffix == "system_status" || name == "system_status":
		return HandleSystemStatus(ctx, s, args)
	case name == GetToolName("system_check") || toolSuffix == "system_check" || name == "system_check":
		return HandleSystemCheck(ctx, s, args)
	case name == GetToolName("get_current_priority_plan") || toolSuffix == "get_current_priority_plan" || name == "get_current_priority_plan":
		return HandleGetCurrentPriorityPlan(ctx, s, args)
	case name == GetToolName("get_priority_plan_items") || toolSuffix == "get_priority_plan_items" || name == "get_priority_plan_items":
		return HandleGetPriorityPlanItems(ctx, s, args)
	case name == GetToolName("get_current_backlog_item") || toolSuffix == "get_current_backlog_item" || name == "get_current_backlog_item":
		return HandleGetCurrentBacklogItem(ctx, s, args)
	case name == GetToolName("get_next_backlog_item") || toolSuffix == "get_next_backlog_item" || name == "get_next_backlog_item":
		return HandleGetNextBacklogItem(ctx, s, args)
	case name == GetToolName("create_object_interactive") || toolSuffix == "create_object_interactive" || name == "create_object_interactive":
		return HandleCreateObjectInteractive(ctx, s, args)
	case name == GetToolName("get_metrics") || toolSuffix == "get_metrics" || name == "mcp_get_metrics" || name == "get_metrics":
		var handler ToolHandler
		_ = concurrency.RunInRLockWithLogger(
			&s.toolsMu, LockNameMcpServerGetMetricsHandler, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				for _, toolName := range []string{GetToolName("get_metrics"), "mcp_get_metrics", "get_metrics"} {
					if tool, exists := s.tools[toolName]; exists && tool.Handler != nil {
						handler = tool.Handler
						break
					}
				}
				return nil
			},
		)
		if handler != nil {
			return handler(ctx, args)
		}
		return nil, errfmt.Errorf("metrics tool handler not found for: %s", name)
	case name == GetToolName("get_tool_metrics") || toolSuffix == "get_tool_metrics" || name == "mcp_get_tool_metrics" || name == "get_tool_metrics":
		var handler ToolHandler
		_ = concurrency.RunInRLockWithLogger(
			&s.toolsMu, LockNameMcpServerGetToolMetricsHandler, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				for _, toolName := range []string{GetToolName("get_tool_metrics"), "mcp_get_tool_metrics", "get_tool_metrics"} {
					if tool, exists := s.tools[toolName]; exists && tool.Handler != nil {
						handler = tool.Handler
						break
					}
				}
				return nil
			},
		)
		if handler != nil {
			return handler(ctx, args)
		}
		return nil, errfmt.Errorf("tool metrics handler not found for: %s", name)
	}

	// Check if there is a custom registered handler for this tool
	var customHandler ToolHandler
	_ = concurrency.RunInRLockWithLogger(
		&s.toolsMu, "mcp_server_custom_tool_handler", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if tool, exists := s.tools[name]; exists && tool.Handler != nil {
				customHandler = tool.Handler
			} else if tool, exists := s.tools[toolSuffix]; exists && tool.Handler != nil {
				customHandler = tool.Handler
			}
			return nil
		},
	)
	if customHandler != nil {
		return customHandler(ctx, args)
	}

	// Check if this is a CLI command tool
	// CLI tools can be prefixed with "cli_" or "zqk" (legacy prefix for backward compatibility)
	var commandPath string
	var isCLITool bool
	// Extract command path from tool name if it's a CLI tool
	if commandPath, isCLITool = strings.CutPrefix(name, s.cliToolPrefix); isCLITool {
		commandPath = strings.ReplaceAll(commandPath, "_", " ")
	} else if commandPath, isCLITool = strings.CutPrefix(name, GetToolPrefix()); isCLITool {
		// Handle brand-prefixed tool names (e.g., "zqk_system_status" -> "zqk system status")
		// Extract the command path by removing the brand prefix
		commandPath = strings.ReplaceAll(commandPath, "_", " ")
	}

	if isCLITool {

		// Enforce config security before executing (runtime check)
		if s.config != nil {
			// Check if command is allowed
			allowed, reason := isCommandAllowed(commandPath, s.config, s.secCtx)
			if !allowed {
				return nil, NewPermissionDeniedError(commandPath, reason)
			}

			// Check if write operation is allowed
			writeAllowed, writeReason := isWriteOperationAllowed(commandPath, s.config, s.secCtx)
			if !writeAllowed {
				return nil, NewPermissionDeniedError(commandPath, writeReason)
			}
		}

		// Check format permission before executing command (like dry-run)
		// This short-circuits unauthorized access attempts efficiently
		// Format is determined by the --context mcp profile (which sets format to JSON)
		// Extract format from args if explicitly provided, otherwise rely on context profile
		format := "json" // Default: mcp context profile sets format to JSON
		if formatArg, ok := args[objects.FieldKeyFormat].(string); ok && formatArg != emptyValue {
			format = formatArg
		}

		// Check format permission (includes client-level restrictions)
		allowed, reason := s.CheckFormatPermission(ctx, format)
		if !allowed {
			return nil, NewErrorResponseBuilder(InvalidFormat, "Format not allowed").
				WithData("format", format).
				WithData("reason", reason).
				WithData("allowed_formats", s.GetAllowedFormats()).
				WithData("command", commandPath).
				Build()
		}

		// If no format was specified and restrictions exist, use first allowed format
		// This ensures we always use an allowed format
		// Note: The --context mcp profile will set format to JSON, so this is a fallback
		if _, ok := args[objects.FieldKeyFormat].(string); !ok || args[objects.FieldKeyFormat].(string) == emptyValue {
			// No format specified - use first allowed format or default to "json" (from mcp profile)
			if len(s.allowedFormats) > 0 {
				// Use first allowed format
				_ = s.allowedFormats[0] // Format is already set above, this assignment was ineffectual
			}
		}

		// Add command path to args, but preserve any existing _command_path that might include positional arguments
		// If _command_path already exists in args (from tool call), it may include positional args like IDs
		// Only overwrite if it doesn't exist or is empty
		if existingPath, ok := args["_command_path"].(string); ok && existingPath != emptyValue {
			// Use the existing path which may include positional arguments
			// But ensure it starts with the correct command (in case it's malformed)
			normalizedExisting := NormalizeCommandPath(existingPath)
			normalizedCommand := NormalizeCommandPath(commandPath)
			if !strings.HasPrefix(normalizedExisting, normalizedCommand) {
				// Existing path doesn't match expected format, use the constructed one
				args["_command_path"] = GetCommandPath(commandPath)
			}
			// Otherwise, keep the existing path which includes positional args
		} else {
			// No existing path, use the one we constructed from tool name (with brand prefix)
			args["_command_path"] = GetCommandPath(commandPath)
		}

		// Send log via MCP logging channel (application-level logs go to client, not trace file)
		// The trace file is reserved for MCP protocol traces (requests/responses) only
		//nolint:errcheck // Logging errors are non-critical
		_ = s.SendLogInfo(fmt.Sprintf("Executing command: %s", commandPath), map[string]any{ //nolint:errcheck // Logging errors are non-critical
			"tool":                    name,
			objects.FieldKeyCommand:   commandPath,
			objects.FieldKeyOperation: "cli_command",
		})

		// Detach from mid-flight request cancel (IDE). Already-cancelled ctx fails fast.
		// Bound by shutdown + MaxToolCallDuration. TRACK: REDACTED
		var execCtx context.Context
		var cancel context.CancelFunc
		if ctx.Err() != nil {
			execCtx, cancel = context.WithCancel(ctx)
			cancel()
		} else {
			base := pkgctx.NewSystemContext()
			if shutdown := s.GetShutdownContext(); shutdown != nil {
				base = shutdown
			}
			execCtx, cancel = context.WithTimeout(base, MaxToolCallDuration)
		}
		defer cancel()
		result, err := s.executeCLICommandWithContext(execCtx, args)

		// Send log via MCP logging channel
		if err != nil {
			// Check if this is a critical error that warrants a notification
			// Some errors are expected (e.g., validation failures) and don't need notifications
			// Use error codes for reliable detection instead of string matching
			isCritical := s.isCriticalError(err)

			if isCritical {
				// Send critical error notification for significant failures
				// Tool execution happens after initialization, so notifications should be allowed
				// Get client ID for routing to the client that triggered the tool execution
				clientID := s.getClientIDWithRole()
				if clientID == emptyValue {
					// Fallback to stored client ID if role-prefixed version is empty
					_ = concurrency.RunInRLockWithLogger(
						&s.clientIDMu, LockNameMcpServerGetClientIdError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							clientID = s.clientID
							return nil
						},
					)
				}
				_ = s.SendCriticalError( //nolint:errcheck // Notification failures are non-critical
					err,
					"error",
					"tool_execution",
					fmt.Sprintf("Tool execution failed: %s", name),
					[]string{
						"Check the command syntax and parameters",
						"Verify you have the necessary permissions for this operation",
						"Review system logs for detailed error information",
						"Check system health using zqk_system_status",
					},
					map[string]any{
						"tool":                    name,
						objects.FieldKeyCommand:   commandPath,
						objects.FieldKeyOperation: "cli_command",
					},
					clientID, // Route to specific client that triggered the tool
					false,    // Allow notification (tool execution is post-initialization)
				)
			}

			// Always send log message for debugging
			//nolint:errcheck // Logging errors are non-critical
			_ = s.SendLogError(fmt.Sprintf("Command failed: %s", commandPath), map[string]any{ //nolint:errcheck // Logging errors are non-critical
				"tool":                    name,
				objects.FieldKeyCommand:   commandPath,
				"error":                   err.Error(),
				objects.FieldKeyOperation: "cli_command",
			})
		} else {
			//nolint:errcheck // Logging errors are non-critical
			_ = s.SendLogInfo(fmt.Sprintf("Command completed: %s", commandPath), map[string]any{ //nolint:errcheck // Logging errors are non-critical
				"tool":                    name,
				objects.FieldKeyCommand:   commandPath,
				objects.FieldKeyOperation: "cli_command",
			})
		}

		// Emit tool completion event
		if s.eventEmitter != nil {
			eventType := EventTypeToolCompleted
			if err != nil {
				eventType = EventTypeToolFailed
			}
			s.eventEmitter.Emit(&Event{
				Type:    eventType,
				Message: fmt.Sprintf("Tool execution %s: %s", map[bool]string{true: "failed", false: "completed"}[err != nil], name),
				Fields: map[string]any{
					"tool": name,
					"error": func() string {
						if err != nil {
							return err.Error()
						}
						return ""
					}(),
				},
				Severity: map[bool]string{true: "error", false: "info"}[err != nil],
			})
		}

		return result, err
	}

	// RC-2: Tool-Not-Found Correction Loop
	var suggestions []string
	_ = concurrency.RunInRLockWithLogger(
		&s.toolsMu, "mcp_server_suggest_tools", logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for registeredName := range s.tools {
				if strings.Contains(registeredName, name) || strings.Contains(name, registeredName) {
					suggestions = append(suggestions, registeredName)
				}
			}
			return nil
		},
	)

	if len(suggestions) > 0 {
		return nil, errfmt.Errorf("Tool '%s' not found. Did you mean: %s? Use exact names.", rawName, strings.Join(suggestions, ", "))
	}
	return nil, errfmt.Errorf("Tool '%s' not found.", rawName)
}

// executeCLICommandWithContext executes a CLI command via MCP bridge with context
func (s *Server) executeCLICommandWithContext(ctx context.Context, args map[string]any) (any, error) {
	// Delegate to cli_bridge package which can handle the type assertion
	// Pass server instance for config access (positional argument names)
	secCtx := s.secCtx
	if reqSecCtx := pkgctx.GetSecurityContext(ctx); reqSecCtx != nil {
		secCtx = reqSecCtx
	}
	return executeCLICommandWithContextForServer(ctx, args, secCtx, s.GetProjectRoot(), s.permissionCache, s)
}

// BootstrapCLITools bootstraps CLI tools during initialization
// This is called automatically if rootCommand and secCtx are set
// Can also be called manually before Serve() to pre-register tools
func (s *Server) BootstrapCLITools() error {
	// Delegate to cli_bridge package which can handle the type assertion
	// Pass config for security enforcement
	return bootstrapCLIToolsForServerWithConfig(s, s.rootCommand, s.secCtx, s.GetProjectRoot(), s.config)
}
