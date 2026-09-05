package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	workflowFormatJSON              = "json"
	workflowFormatYAML              = "yaml"
	workflowFormatTable             = "table"
	workflowSchemaObject            = "object"
	workflowSchemaString            = "string"
	workflowParamProps              = "properties"
	workflowParamEnum               = "enum"
	workflowParamDefault            = "default"
	workflowOutputFormat            = "Output format"
	workflowErrNoPlan               = "no active priority plan found"
	workflowErrNoCurrent            = "no in-progress backlog item found"
	workflowErrNoNextItem           = "no next backlog item found (no planned or exploring items)"
	workflowErrBadFormat            = "unexpected result format: %T"
	workflowKeyObjects              = "objects"
	workflowKeyCmdPath              = "_command_path"
	workflowKeyFilter               = "filter"
	workflowKeySortBy               = "sort_by"
	workflowKeySortAsc              = "sort_asc"
	workflowKeyLimit                = "limit"
	workflowKeyPlanID               = "priority_plan_id"
	workflowSortPriority            = "priority_tier"
	workflowStatusPlanned           = "status=planned"
	workflowStatusExplore           = "status=exploring"
	workflowStatusInProg            = "status=in_progress"
	workflowCmdListBacklog          = "object list backlog_item"
	workflowCmdWhatsNext            = "workflow whats-next"
	workflowKeyPriorityPlan         = "priority_plan"
	workflowSchemaRequired          = "required"
	workflowOutputJsonExampleSuffix = " with format='json'."
)

// IsStudioPackToolsEnabled returns true if studio ontology pack tools should be registered.
// TRACK: BLI-KERNEL-PACK-MCP-001 / CRIT-KERNEL-PACK-MCP-001.
// When false, default open-core MCP server excludes pack-tagged tools.
func IsStudioPackToolsEnabled(secCtx *pkgctx.SecurityContext) bool {
	if os.Getenv(zqkenv.EnableStudioPackTools()) == "1" || os.Getenv(zqkenv.StudioDogfood()) == "1" {
		return true
	}
	if secCtx != nil && slices.Contains(secCtx.GetRoles(), "admin") {
		return true
	}
	return false
}

// RegisterWorkflowTools registers workflow-aware built-in tools
// These tools are context-aware and optimized for agent workflows
// They can be role-based (filtered by security context)
func RegisterWorkflowTools(server *Server, secCtx *pkgctx.SecurityContext) {
	if !IsStudioPackToolsEnabled(secCtx) {
		// Open-core default: exclude studio workflow pack tools
		return
	}

	// Priority Plan Tools - available to all roles (read operations)
	server.RegisterTool(
		GetToolName("get_current_priority_plan"),
		"Get the current execution-facing priority plan (whats-next Gantt lead). Not the lowest active_order grooming column. Example: "+GetToolName("get_current_priority_plan")+workflowOutputJsonExampleSuffix,
		map[string]any{
			objects.FieldKeyType: workflowSchemaObject,
			workflowParamProps: map[string]any{
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        workflowSchemaString,
					workflowParamEnum:           []string{workflowFormatJSON, workflowFormatYAML, workflowFormatTable},
					workflowParamDefault:        workflowFormatJSON,
					objects.FieldKeyDescription: workflowOutputFormat,
				},
			},
		},
		nil,
	)

	server.RegisterTool(
		GetToolName("get_priority_plan_items"),
		"Get all backlog items for a specific priority plan, organized by priority tier (P0, P1, P2, P3). This is the workflow-aware way to see what items are in a priority plan. Example: "+GetToolName("get_priority_plan_items")+" with priority_plan_id='PRI-210', format='json'.",
		map[string]any{
			objects.FieldKeyType:   workflowSchemaObject,
			workflowSchemaRequired: []string{workflowKeyPlanID},
			workflowParamProps: map[string]any{
				workflowKeyPlanID: map[string]any{
					objects.FieldKeyType:        workflowSchemaString,
					objects.FieldKeyDescription: "Priority plan ID (e.g., 'PRI-210')",
				},
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        workflowSchemaString,
					workflowParamEnum:           []string{workflowFormatJSON, workflowFormatYAML, workflowFormatTable},
					workflowParamDefault:        workflowFormatJSON,
					objects.FieldKeyDescription: workflowOutputFormat,
				},
			},
		},
		nil,
	)

	// Backlog Item Tools - available to all roles (read operations)
	server.RegisterTool(
		GetToolName("get_current_backlog_item"),
		"Get the currently in-progress backlog item. Returns the backlog item with status='in_progress' and highest priority_tier (P0 > P1 > P2 > P3). If multiple items are in progress, returns the one with the highest priority. This is the workflow-aware way to see what the agent is currently working on. Example: "+GetToolName("get_current_backlog_item")+workflowOutputJsonExampleSuffix,
		map[string]any{
			objects.FieldKeyType: workflowSchemaObject,
			workflowParamProps: map[string]any{
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        workflowSchemaString,
					workflowParamEnum:           []string{workflowFormatJSON, workflowFormatYAML, workflowFormatTable},
					workflowParamDefault:        workflowFormatJSON,
					objects.FieldKeyDescription: workflowOutputFormat,
				},
			},
		},
		nil,
	)

	server.RegisterTool(
		GetToolName("get_next_backlog_item"),
		"Get the next immediate backlog item to work on. Returns the backlog item with status='planned' and highest priority_tier (P0 > P1 > P2 > P3), sorted by priority_tier. If no planned items exist, returns the next item in 'exploring' status. This is the workflow-aware way to see what the agent should work on next. Example: "+GetToolName("get_next_backlog_item")+workflowOutputJsonExampleSuffix,
		map[string]any{
			objects.FieldKeyType: workflowSchemaObject,
			workflowParamProps: map[string]any{
				objects.FieldKeyFormat: map[string]any{
					objects.FieldKeyType:        workflowSchemaString,
					workflowParamEnum:           []string{workflowFormatJSON, workflowFormatYAML, workflowFormatTable},
					workflowParamDefault:        workflowFormatJSON,
					objects.FieldKeyDescription: workflowOutputFormat,
				},
			},
		},
		nil,
	)
}

// workflowObjectListTimeout is used for workflow tools that run "object list" under the hood.
// Increased from 20s to 60s to tolerate storage lock contention during concurrent agent startup.
const workflowObjectListTimeout = 60 * time.Second

// getNextBacklogItemTotalTimeout caps the entire get_next_backlog_item operation (planned + exploring).
// Increased from 15s to 60s.
const getNextBacklogItemTotalTimeout = 60 * time.Second

// getNextBacklogItemPerCallTimeout is the per-CLI-call timeout for get_next_backlog_item.
// Increased from 10s to 30s.
const getNextBacklogItemPerCallTimeout = 30 * time.Second

// workflowExecContext returns a context for workflow tool CLI execution that is cancelled on
// server shutdown or after timeout. Mid-flight request cancel (IDE) is ignored so successive
// in-process CLI tools/calls are not poisoned — already-cancelled request ctx still fails fast.
// TRACK: REDACTED — re-wire polite client cancel without killing CLI.
func workflowExecContext(ctx context.Context, server *Server, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx = EnsureContext(ctx)
	if ctx.Err() != nil {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return cctx, func() {}
	}
	base := pkgctx.NewSystemContext()
	if server != nil {
		if shutdown := server.GetShutdownContext(); shutdown != nil {
			base = shutdown
		}
	}
	if timeout <= 0 {
		timeout = workflowObjectListTimeout
	}
	return context.WithTimeout(base, timeout)
}

// HandleGetCurrentPriorityPlan handles the get_current_priority_plan built-in tool.
// Resolves the same Gantt lead as `zqk workflow whats-next` (not lowest active_order).
func HandleGetCurrentPriorityPlan(ctx context.Context, server *Server, _ map[string]any) (any, error) {
	ctx = EnsureContext(ctx)
	cliArgs := map[string]any{
		workflowKeyCmdPath:     GetCommandPath(workflowCmdWhatsNext),
		"skip-measure":         true,
		objects.FieldKeyFormat: workflowFormatJSON,
	}

	execCtx, cancel := workflowExecContext(ctx, server, workflowObjectListTimeout)
	defer cancel()
	result, err := server.executeCLICommandWithContext(execCtx, cliArgs)
	if err != nil {
		return nil, errfmt.Newf("failed to get current priority plan").Wrap(err)
	}

	priorityPlan, found, err := extractWhatsNextLeadPlan(result)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{objects.FieldKeyStatus: objects.ObjectStatusNotFound, "message": workflowErrNoPlan}, nil
	}
	return priorityPlan, nil
}

// HandleGetPriorityPlanItems handles the get_priority_plan_items built-in tool
func HandleGetPriorityPlanItems(ctx context.Context, server *Server, args map[string]any) (any, error) {
	ctx = EnsureContext(ctx)
	// Get required priority_plan_id
	priorityPlanID, ok := args[workflowKeyPlanID].(string)
	if !ok || priorityPlanID == emptyValue {
		return nil, NewElicitationError(
			"Missing required parameter: priority_plan_id",
			[]ElicitationParam{
				ElicitParamWithExample(
					workflowKeyPlanID,
					"The priority plan ID (e.g., 'PRI-210')",
					"string",
					true,
					"PRI-210",
				),
			},
		)
	}

	// Get format (default to json)
	format := workflowFormatJSON
	if f, ok := args[objects.FieldKeyFormat].(string); ok && f != emptyValue {
		format = f
	}

	// Use CLI command with brand prefix. Omit "kind" — it's already in the path (object list backlog_item).
	cliArgs := map[string]any{
		workflowKeyCmdPath:     GetCommandPath(workflowCmdListBacklog),
		workflowKeyFilter:      []string{fmt.Sprintf("priority_plan_ref=%s", priorityPlanID)},
		workflowKeySortBy:      workflowSortPriority,
		workflowKeySortAsc:     true,
		objects.FieldKeyFormat: format,
	}

	execCtx, cancel := workflowExecContext(ctx, server, workflowObjectListTimeout)
	defer cancel()
	return server.executeCLICommandWithContext(execCtx, cliArgs)
}

// HandleGetCurrentBacklogItem handles the get_current_backlog_item built-in tool
func HandleGetCurrentBacklogItem(ctx context.Context, server *Server, args map[string]any) (any, error) {
	ctx = EnsureContext(ctx)
	// Get format (default to json)
	format := workflowFormatJSON
	if f, ok := args[objects.FieldKeyFormat].(string); ok && f != emptyValue {
		format = f
	}

	// Use CLI command with brand prefix. Omit "kind" — it's already in the path (object list backlog_item).
	cliArgs := map[string]any{
		workflowKeyCmdPath:     GetCommandPath(workflowCmdListBacklog),
		workflowKeyFilter:      []string{workflowStatusInProg},
		workflowKeySortBy:      workflowSortPriority,
		workflowKeySortAsc:     true,
		workflowKeyLimit:       1,
		objects.FieldKeyFormat: format,
	}

	execCtx, cancel := workflowExecContext(ctx, server, workflowObjectListTimeout)
	defer cancel()
	result, err := server.executeCLICommandWithContext(execCtx, cliArgs)
	if err != nil {
		return nil, errfmt.Newf("failed to get current backlog item").Wrap(err)
	}

	// Extract the first item from the result
	backlogItem, found, err := extractWorkflowResultItem(result)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{objects.FieldKeyStatus: objects.ObjectStatusNotFound, "message": workflowErrNoCurrent}, nil
	}
	return backlogItem, nil
}

// HandleGetNextBacklogItem handles the get_next_backlog_item built-in tool.
// Uses a total timeout (getNextBacklogItemTotalTimeout) so the whole operation (planned + exploring)
// cannot hang; each CLI call is further limited by getNextBacklogItemPerCallTimeout.
func HandleGetNextBacklogItem(ctx context.Context, server *Server, args map[string]any) (any, error) {
	ctx = EnsureContext(ctx)
	// Get format (default to json)
	format := workflowFormatJSON
	if f, ok := args[objects.FieldKeyFormat].(string); ok && f != emptyValue {
		format = f
	}

	// Cap total time for this tool so we never run 20s+20s (planned then exploring).
	totalCtx, totalCancel := context.WithTimeout(ctx, getNextBacklogItemTotalTimeout)
	defer totalCancel()

	// First try to get planned items (sorted by priority_tier). Omit "kind" — already in path.
	cliArgs := map[string]any{
		workflowKeyCmdPath:     GetCommandPath(workflowCmdListBacklog),
		workflowKeyFilter:      []string{workflowStatusPlanned},
		workflowKeySortBy:      workflowSortPriority,
		workflowKeySortAsc:     true,
		workflowKeyLimit:       1,
		objects.FieldKeyFormat: format,
	}

	execCtx, cancel := workflowExecContext(totalCtx, server, getNextBacklogItemPerCallTimeout)
	defer cancel()
	result, err := server.executeCLICommandWithContext(execCtx, cliArgs)
	if err != nil {
		return nil, errfmt.Newf("failed to get next backlog item").Wrap(err)
	}

	// Extract the first item from the result
	backlogItem, found, err := extractWorkflowResultItem(result)
	if err != nil {
		return nil, err
	}
	if !found {
		// No planned items, try exploring status (reuse totalCtx so we don't add another full timeout)
		return handleGetNextBacklogItemExploring(totalCtx, server, format)
	}
	return backlogItem, nil
}

// handleGetNextBacklogItemExploring gets the next item from exploring status.
// Uses getNextBacklogItemPerCallTimeout; when ctx is totalCtx from HandleGetNextBacklogItem,
// the effective timeout is the remaining time on ctx (bounded by total 15s).
func handleGetNextBacklogItemExploring(ctx context.Context, server *Server, format string) (any, error) {
	cliArgs := map[string]any{
		workflowKeyCmdPath:     GetCommandPath(workflowCmdListBacklog),
		workflowKeyFilter:      []string{workflowStatusExplore},
		workflowKeySortBy:      workflowSortPriority,
		workflowKeySortAsc:     true,
		workflowKeyLimit:       1,
		objects.FieldKeyFormat: format,
	}

	timeout := getNextBacklogItemPerCallTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	execCtx, cancel := workflowExecContext(ctx, server, timeout)
	defer cancel()
	result, err := server.executeCLICommandWithContext(execCtx, cliArgs)
	if err != nil {
		return nil, errfmt.Newf("failed to get next backlog item (exploring)").Wrap(err)
	}

	// Extract the first item from the result
	backlogItem, found, err := extractWorkflowResultItem(result)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New(workflowErrNoNextItem)
	}
	return backlogItem, nil
}

func extractWhatsNextLeadPlan(result any) (any, bool, error) {
	switch v := result.(type) {
	case map[string]any:
		if plan, ok := v[workflowKeyPriorityPlan]; ok && plan != nil {
			return plan, true, nil
		}
		if data, ok := v["data"].(map[string]any); ok {
			if plan, ok := data[workflowKeyPriorityPlan]; ok && plan != nil {
				return plan, true, nil
			}
		}
		return nil, false, nil
	case string:
		var data map[string]any
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return nil, false, errfmt.Errorf(workflowErrBadFormat, result)
		}
		return extractWhatsNextLeadPlan(data)
	default:
		return nil, false, errfmt.Errorf(workflowErrBadFormat, result)
	}
}

func extractWorkflowResultItem(result any) (any, bool, error) {
	switch v := result.(type) {
	case map[string]any:
		if objs, ok := v[workflowKeyObjects].([]any); ok && len(objs) > 0 {
			return objs[0], true, nil
		} else if objs, ok := v[workflowKeyObjects].([]map[string]any); ok && len(objs) > 0 {
			return objs[0], true, nil
		}
		return nil, false, nil
	case []any:
		if len(v) > 0 {
			return v[0], true, nil
		}
		return nil, false, nil
	case []map[string]any:
		if len(v) > 0 {
			return v[0], true, nil
		}
		return nil, false, nil
	default:
		if str, ok := result.(string); ok {
			var data map[string]any
			if err := json.Unmarshal([]byte(str), &data); err == nil {
				if objs, ok := data[workflowKeyObjects].([]any); ok && len(objs) > 0 {
					return objs[0], true, nil
				}
				return nil, false, nil
			} else {
				return nil, false, errfmt.Errorf(workflowErrBadFormat, result)
			}
		}
		return nil, false, errfmt.Errorf(workflowErrBadFormat, result)
	}
}
