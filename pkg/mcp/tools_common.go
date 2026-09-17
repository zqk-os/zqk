package mcp

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// mcpToolArgKey* are MCP tool schema / forwarded CLI argument map keys.
const (
	mcpToolArgKeyAutoFix = "auto_fix"
	mcpToolArgKeyFilter  = "filter"
	mcpToolArgKeyForce   = "force"
	mcpToolArgKeyLimit   = "limit"
	mcpToolArgKeyOffset  = "offset"
	mcpToolArgKeySortAsc = "sort_asc"
	mcpToolArgKeySortBy  = "sort_by"
	mcpToolArgKeyView    = "view"
	// mcpToolArgKeyFields forwards to CLI --fields (projection after full load / overlays).
	mcpToolArgKeyFields = "fields"
	// mcpToolArgKeyLinkHydration forwards to CLI --link-hydration (pkg/objectget).
	mcpToolArgKeyLinkHydration = "link_hydration"
)

// RegisterCommonTools registers commonly used CLI commands as built-in tools
// These tools are always available (bypass config whitelisting) and optimized for agent use
func RegisterCommonTools(server *Server) {
	// Object operations - most commonly used by agents
	NewToolBuilder(
		GetToolName("object_list"),
		"List objects with filtering, sorting, and pagination. This is the most commonly used tool for querying the system. Use filters to find specific objects by kind, status, or other properties. Example: "+GetToolName("object_list")+" with kind='backlog_item', filter=['status=in_progress'], format='json'.",
	).
		AddStringProperty(objects.FieldKeyKind, "Object kind to filter by (e.g., 'backlog_item', 'goal', 'milestone')").
		AddArrayProperty(mcpToolArgKeyFilter, "Filter expressions (e.g., ['status=in_progress', 'priority=high'])", "string").
		AddStringProperty(mcpToolArgKeySortBy, "Field to sort by (e.g., 'created_at', 'updated_at', 'priority')").
		AddBooleanPropertyWithDefault(mcpToolArgKeySortAsc, "Sort ascending (true) or descending (false)", true).
		AddNumberPropertyWithDefault(mcpToolArgKeyLimit, "Maximum number of objects to return", 100).
		AddNumberPropertyWithDefault(mcpToolArgKeyOffset, "Number of objects to skip", 0).
		AddStandardFormatProperty().
		Register(server, nil)

	// Server shutdown tool
	NewToolBuilder(
		GetToolName("server_shutdown"),
		"Initiate a graceful shutdown of the MCP server. Use this when you are completely finished with all tasks and the session should end. Example: "+GetToolName("server_shutdown")+" with reason='Tasks completed'.",
	).
		AddStringProperty("reason", "Reason for shutting down the server").
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			reason, _ := args[objects.FieldKeyReason].(string)
			if reason == "" {
				reason = "client requested shutdown via tool"
			}
			server.RequestShutdown(reason)
			return map[string]any{
				objects.FieldKeyStatus: "success",
				"message":              "Shutdown requested: " + reason,
			}, nil
		})

	NewToolBuilder(
		GetToolName("object_get"),
		"Get a single object by ID. Returns full object data including all fields and metadata (unless fields is set). Use this to retrieve detailed information about a specific object. Example: "+GetToolName("object_get")+" with id='BLI-626', format='json'. Optional: fields=['prompt_body'] (same as CLI --fields), view='milestone-completion-report', link_hydration='lazy' (same as zqk object get; see pkg/objectget).",
	).
		AddStringProperty(objects.FieldKeyID, "Object ID (e.g., 'BLI-626', 'GOAL-123')").
		MarkRequired(objects.FieldKeyID).
		AddArrayProperty(mcpToolArgKeyFields, "Optional top-level keys to return (matches CLI --fields; full object is loaded first)", "string").
		AddStringProperty(mcpToolArgKeyView, "Optional view name (e.g. 'milestone-completion-report', 'milestone-progress-report')").
		AddStringProperty(mcpToolArgKeyLinkHydration, "Optional reference overlay depth: 'lazy', 'default', or 'eager' (matches CLI --link-hydration)").
		AddJSONYAMLFormatProperty().
		Register(server, nil)

	NewToolBuilder(
		GetToolName("object_count"),
		"Count objects by kind and optional filters. Use this to quickly get counts without retrieving full object data. Example: "+GetToolName("object_count")+" with kind='backlog_item', filter=['status=in_progress'].",
	).
		AddStringProperty(objects.FieldKeyKind, "Object kind to count (e.g., 'backlog_item', 'goal')").
		AddArrayProperty(mcpToolArgKeyFilter, "Filter expressions (e.g., ['status=in_progress'])", "string").
		AddJSONYAMLFormatProperty().
		Register(server, nil)

	// System operations - critical for health monitoring
	NewToolBuilder(
		GetToolName("system_status"),
		"Get system status and health information. Use this to check system health before performing operations. Returns overall system status, object counts, and health metrics. Example: "+GetToolName("system_status")+" with format='json'.",
	).
		AddJSONYAMLFormatProperty().
		Register(server, nil)

	NewToolBuilder(
		GetToolName("system_check"),
		"Check object health and compliance. Use this to validate specific objects or run system-wide checks. Returns tiered violations (blocking, warnings, informational, recommendations). Example: "+GetToolName("system_check")+" with id='BLI-626', format='json'.",
	).
		AddStringProperty(objects.FieldKeyID, "Object ID to check (optional, checks all if not provided)").
		AddNumberProperty(objects.FieldKeyTier, "Tier to check (1=blocking, 2=warnings, 3=informational, 4=recommendations). Checks all tiers if not specified.").
		AddBooleanPropertyWithDefault(mcpToolArgKeyAutoFix, "Automatically fix issues when possible", false).
		AddBooleanPropertyWithDefault(mcpToolArgKeyForce, "Force fix even if it creates audit events", false).
		AddJSONYAMLFormatProperty().
		Register(server, nil)
}

// objectListTimeout is shorter than other tools so slow/hung "object list" CLI (e.g. graph/storage)
// returns a timeout error quickly instead of hanging for 60s. Client can retry with smaller limit or specific kind.
const objectListTimeout = 20 * time.Second

// mcpFilterExprs normalizes MCP `filter` so list and count share one membrane.
// JSON-schema arrays arrive as []any; some clients send []string or a single string.
// Dropping a mistyped filter made list return a different set than count.
// TRACK: BLI-REDACTED
func mcpFilterExprs(v any) []any {
	switch x := v.(type) {
	case []any:
		if len(x) == 0 {
			return nil
		}
		return x
	case []string:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return []any{x}
	default:
		return nil
	}
}

// HandleObjectList handles the object_list built-in tool
func HandleObjectList(ctx context.Context, server *Server, args map[string]any) (any, error) {
	cmdArgs := map[string]any{
		"_command_path": GetCommandPath("object list"),
	}
	if kind, ok := args[objects.FieldKeyKind].(string); ok && kind != emptyValue {
		cmdArgs[objects.FieldKeyKind] = kind
	}
	if filter := mcpFilterExprs(args[mcpToolArgKeyFilter]); len(filter) > 0 {
		cmdArgs[mcpToolArgKeyFilter] = filter
	}
	if sortBy, ok := args[mcpToolArgKeySortBy].(string); ok && sortBy != emptyValue {
		cmdArgs[mcpToolArgKeySortBy] = sortBy
	}
	if sortAsc, ok := args[mcpToolArgKeySortAsc].(bool); ok {
		cmdArgs[mcpToolArgKeySortAsc] = sortAsc
	}
	switch limit := args[mcpToolArgKeyLimit].(type) {
	case float64:
		cmdArgs[mcpToolArgKeyLimit] = int(limit)
	case int:
		cmdArgs[mcpToolArgKeyLimit] = limit
	}
	switch offset := args[mcpToolArgKeyOffset].(type) {
	case float64:
		cmdArgs[mcpToolArgKeyOffset] = int(offset)
	case int:
		cmdArgs[mcpToolArgKeyOffset] = offset
	}
	if format, ok := args[objects.FieldKeyFormat].(string); ok && format != emptyValue {
		cmdArgs[objects.FieldKeyFormat] = format
	}
	execCtx, cancel := workflowExecContext(ctx, server, objectListTimeout)
	defer cancel()
	return server.executeCLICommandWithContext(execCtx, cmdArgs)
}

// HandleObjectGet handles the object_get built-in tool
func HandleObjectGet(ctx context.Context, server *Server, args map[string]any) (any, error) {
	id, ok := args[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		return nil, NewElicitationError(
			"Missing required parameter: id",
			[]ElicitationParam{
				ElicitParamWithExample(
					objects.FieldKeyID,
					"The object ID to retrieve (e.g., 'BLI-626', 'GOAL-123')",
					"string",
					true,
					"BLI-626",
				),
			},
		)
	}
	cmdArgs := map[string]any{
		"_command_path":    GetCommandPath("object get"),
		objects.FieldKeyID: id,
	}
	if format, ok := args[objects.FieldKeyFormat].(string); ok && format != emptyValue {
		cmdArgs[objects.FieldKeyFormat] = format
	}
	if view, ok := args[mcpToolArgKeyView].(string); ok && view != emptyValue {
		cmdArgs[mcpToolArgKeyView] = view
	}
	if lh, ok := args[mcpToolArgKeyLinkHydration].(string); ok && lh != emptyValue {
		cmdArgs[mcpToolArgKeyLinkHydration] = lh
	}
	if fields, ok := args[mcpToolArgKeyFields].([]any); ok && len(fields) > 0 {
		cmdArgs[mcpToolArgKeyFields] = fields
	}
	execCtx, cancel := workflowExecContext(ctx, server, 60*time.Second)
	defer cancel()
	return server.executeCLICommandWithContext(execCtx, cmdArgs)
}

// HandleObjectCount handles the object_count built-in tool
func HandleObjectCount(ctx context.Context, server *Server, args map[string]any) (any, error) {
	cmdArgs := map[string]any{
		"_command_path": GetCommandPath("object count"),
	}
	if kind, ok := args[objects.FieldKeyKind].(string); ok && kind != emptyValue {
		cmdArgs[objects.FieldKeyKind] = kind
	}
	if filter := mcpFilterExprs(args[mcpToolArgKeyFilter]); len(filter) > 0 {
		cmdArgs[mcpToolArgKeyFilter] = filter
	}
	if format, ok := args[objects.FieldKeyFormat].(string); ok && format != emptyValue {
		cmdArgs[objects.FieldKeyFormat] = format
	}
	execCtx, cancel := workflowExecContext(ctx, server, 60*time.Second)
	defer cancel()
	return server.executeCLICommandWithContext(execCtx, cmdArgs)
}

// HandleSystemStatus handles the system_status built-in tool
func HandleSystemStatus(ctx context.Context, server *Server, args map[string]any) (any, error) {
	cmdArgs := map[string]any{
		"_command_path": GetCommandPath("system status"),
	}
	if format, ok := args[objects.FieldKeyFormat].(string); ok && format != emptyValue {
		cmdArgs[objects.FieldKeyFormat] = format
	}
	execCtx, cancel := workflowExecContext(ctx, server, 60*time.Second)
	defer cancel()
	return server.executeCLICommandWithContext(execCtx, cmdArgs)
}

// HandleSystemCheck handles the system_check built-in tool
func HandleSystemCheck(ctx context.Context, server *Server, args map[string]any) (any, error) {
	cmdArgs := map[string]any{
		"_command_path": GetCommandPath("system check"),
	}
	if id, ok := args[objects.FieldKeyID].(string); ok && id != emptyValue {
		cmdArgs[objects.FieldKeyID] = id
	}
	switch v := args[objects.FieldKeyTier].(type) {
	case float64:
		cmdArgs[objects.FieldKeyTier] = int(v)
	case int:
		cmdArgs[objects.FieldKeyTier] = v
	}
	if autoFix, ok := args[mcpToolArgKeyAutoFix].(bool); ok {
		cmdArgs[mcpToolArgKeyAutoFix] = autoFix
	}
	if force, ok := args[mcpToolArgKeyForce].(bool); ok {
		cmdArgs[mcpToolArgKeyForce] = force
	}
	if format, ok := args[objects.FieldKeyFormat].(string); ok && format != emptyValue {
		cmdArgs[objects.FieldKeyFormat] = format
	}
	execCtx, cancel := workflowExecContext(ctx, server, 60*time.Second)
	defer cancel()
	return server.executeCLICommandWithContext(execCtx, cmdArgs)
}
