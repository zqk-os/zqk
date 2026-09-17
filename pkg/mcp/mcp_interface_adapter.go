package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// EventCoordinator is an interface to avoid import cycles
// This matches coordination.EventCoordinator but defined here to avoid cycles
type EventCoordinator interface {
	Emit(ctx context.Context, eventCtx any) error
}

// MCPServerAdapter adapts Server to implement MCPServer and MCPAsyncServer interfaces
// This avoids method name conflicts with existing Server methods
type MCPServerAdapter struct {
	*Server
	metrics     *MCPMetrics
	coordinator EventCoordinator // Optional coordinator for event emission (avoids import cycle)
}

// NewMCPServerAdapter creates a new adapter that implements the MCP interfaces
func NewMCPServerAdapter(server *Server) *MCPServerAdapter {
	// Try to get coordinator via reflection/interface to avoid import cycle
	// Coordinator will be set via SetCoordinator if available
	adapter := &MCPServerAdapter{
		Server:  server,
		metrics: server.mcpMetrics,
		// Coordinator will be set via SetCoordinator if coordinator package is available
	}
	return adapter
}

// NewMCPServerAdapterWithCoordinator creates a new adapter with a specific coordinator
func NewMCPServerAdapterWithCoordinator(server *Server, coordinator EventCoordinator) *MCPServerAdapter {
	return &MCPServerAdapter{
		Server:      server,
		metrics:     server.mcpMetrics,
		coordinator: coordinator,
	}
}

// SetCoordinator sets the coordinator for event emission
func (a *MCPServerAdapter) SetCoordinator(coordinator EventCoordinator) {
	a.coordinator = coordinator
}

// Ensure MCPServerAdapter implements the interfaces
var _ MCPServer = (*MCPServerAdapter)(nil)
var _ MCPAsyncServer = (*MCPServerAdapter)(nil) // GetMetricsSnapshot is implemented below

// MCPServer Implementation

// Initialize implements MCPServer.Initialize
func (a *MCPServerAdapter) Initialize(ctx context.Context, params *InitializeParams) (*InitializeResult, error) {
	start := time.Now()
	operationID := fmt.Sprintf("mcp_initialize_%d", time.Now().UnixNano())

	defer func() {
		duration := time.Since(start)
		var err error
		// Error will be set below if any
		a.metrics.RecordInitialize(duration, err)

		// Emit via coordinator if available
		if a.coordinator != nil {
			eventCtx := BuildMCPEventContext(operationID, "mcp_initialize", "complete", duration, err)
			_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		}
	}()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		duration := time.Since(start)
		a.metrics.RecordInitialize(duration, err)
		if a.coordinator != nil {
			eventCtx := BuildMCPEventContext(operationID, "mcp_initialize", "error", duration, err)
			_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck
		}
		return nil, err
	}
	result, err := a.handleInitialize(ctx, "initialize", paramsJSON)
	if err != nil {
		duration := time.Since(start)
		a.metrics.RecordInitialize(duration, err)
		if a.coordinator != nil {
			eventCtx := BuildMCPEventContext(operationID, "mcp_initialize", "error", duration, err)
			_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck
		}
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	switch v := result.(type) {
	case *InitializeResult:
		return v, nil
	case map[string]any:
		return mapToStruct[InitializeResult](v)
	}

	return nil, errfmt.Errorf("unexpected result type: %T", result)
}

// NotifyInitialized implements MCPServer.NotifyInitialized
func (a *MCPServerAdapter) NotifyInitialized(ctx context.Context, params *InitializedParams) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	_, err = a.handleNotificationInitialized(ctx, "notifications/initialized", paramsJSON)
	return err
}

// Shutdown implements MCPServer.Shutdown
func (a *MCPServerAdapter) Shutdown(ctx context.Context) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordShutdown(duration)
	}()

	_, err := a.handleShutdown(ctx, "shutdown", nil)
	return err
}

// ListTools implements MCPServer.ListTools
func (a *MCPServerAdapter) ListTools(ctx context.Context) (*ToolsListResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordToolsList(duration, nil) // Error recorded below if any
	}()

	result, err := a.handleToolsList(ctx, "tools/list", nil)
	if err != nil {
		a.metrics.RecordToolsList(time.Since(start), err)
		return nil, err
	}
	switch v := result.(type) {
	case *ToolsListResult:
		return v, nil
	case map[string]any:
		return mapToStruct[ToolsListResult](v)
	}

	// Fallback: use internal method (different signature - no context)
	tools := a.Server.ListTools() // This returns []Tool, not (*ToolsListResult, error)
	return &ToolsListResult{Tools: tools}, nil
}

// CallTool implements MCPServer.CallTool
func (a *MCPServerAdapter) CallTool(ctx context.Context, params *ToolCallParams) (*ToolCallResult, error) {
	start := time.Now()
	toolName := params.Name
	operationID := fmt.Sprintf("mcp_tool_call_%s_%d", toolName, time.Now().UnixNano())

	defer func() {
		duration := time.Since(start)
		var err error
		// Error will be set below if any
		a.metrics.RecordToolCall(toolName, duration, err)

		// Emit via coordinator if available
		if a.coordinator != nil {
			eventCtx := BuildMCPToolCallEventContext(operationID, toolName, duration, err)
			_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		}
	}()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		duration := time.Since(start)
		a.metrics.RecordToolCall(toolName, duration, err)
		if a.coordinator != nil {
			eventCtx := BuildMCPToolCallEventContext(operationID, toolName, duration, err)
			_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck
		}
		return nil, err
	}
	result, err := a.handleToolsCall(ctx, "tools/call", paramsJSON)
	if err != nil {
		duration := time.Since(start)
		a.metrics.RecordToolCall(toolName, duration, err)
		if a.coordinator != nil {
			eventCtx := BuildMCPToolCallEventContext(operationID, toolName, duration, err)
			_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck
		}
		return nil, err
	}
	switch v := result.(type) {
	case *ToolCallResult:
		return v, nil
	case map[string]any:
		return mapToStruct[ToolCallResult](v)
	}

	return nil, errfmt.Errorf("unexpected result type: %T", result)
}

// ListResources implements MCPServer.ListResources
func (a *MCPServerAdapter) ListResources(ctx context.Context, params *ResourcesListParams) (*ResourcesListResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordResourcesList(duration, nil) // Error recorded below if any
	}()

	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			a.metrics.RecordResourcesList(time.Since(start), err)
			return nil, err
		}
	}
	result, err := a.handleResourcesList(ctx, "resources/list", paramsJSON)
	if err != nil {
		a.metrics.RecordResourcesList(time.Since(start), err)
		return nil, err
	}
	switch v := result.(type) {
	case *ResourcesListResult:
		return v, nil
	case map[string]any:
		return mapToStruct[ResourcesListResult](v)
	}

	return nil, errfmt.Errorf("unexpected result type: %T", result)
}

// GetResource implements MCPServer.GetResource
func (a *MCPServerAdapter) GetResource(ctx context.Context, params *ResourceGetParams) (*ResourceGetResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordResourceGet(duration, nil) // Error recorded below if any
	}()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		a.metrics.RecordResourceGet(time.Since(start), err)
		return nil, err
	}
	result, err := a.handleResourcesGet(ctx, "resources/get", paramsJSON)
	if err != nil {
		a.metrics.RecordResourceGet(time.Since(start), err)
		return nil, err
	}
	switch v := result.(type) {
	case *ResourceGetResult:
		return v, nil
	case map[string]any:
		return mapToStruct[ResourceGetResult](v)
	}

	return nil, errfmt.Errorf("unexpected result type: %T", result)
}

// ListPrompts implements MCPServer.ListPrompts
func (a *MCPServerAdapter) ListPrompts(ctx context.Context) (*PromptsListResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordPromptsList(duration, nil) // Error recorded below if any
	}()

	result, err := a.handlePromptsList(ctx, "prompts/list", nil)
	if err != nil {
		a.metrics.RecordPromptsList(time.Since(start), err)
		return nil, err
	}
	switch v := result.(type) {
	case *PromptsListResult:
		return v, nil
	case map[string]any:
		return mapToStruct[PromptsListResult](v)
	}

	// Fallback: use internal method (different signature - no context)
	prompts := a.Server.ListPrompts() // This returns []Prompt, not (*PromptsListResult, error)
	return &PromptsListResult{Prompts: prompts}, nil
}

// GetPrompt implements MCPServer.GetPrompt
func (a *MCPServerAdapter) GetPrompt(ctx context.Context, params *PromptGetParams) (*PromptGetResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordPromptGet(duration, nil) // Error recorded below if any
	}()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		a.metrics.RecordPromptGet(time.Since(start), err)
		return nil, err
	}
	result, err := a.handlePromptsGet(ctx, "prompts/get", paramsJSON)
	if err != nil {
		a.metrics.RecordPromptGet(time.Since(start), err)
		return nil, err
	}
	switch v := result.(type) {
	case *PromptGetResult:
		return v, nil
	case map[string]any:
		return mapToStruct[PromptGetResult](v)
	}

	return nil, errfmt.Errorf("unexpected result type: %T", result)
}

// ListRoots implements MCPServer.ListRoots
func (a *MCPServerAdapter) ListRoots(ctx context.Context) (*RootsListResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		a.metrics.RecordRootsList(duration, nil) // Error recorded below if any
	}()

	result, err := a.handleRootsList(ctx, "roots/list", nil)
	if err != nil {
		a.metrics.RecordRootsList(time.Since(start), err)
		return nil, err
	}
	switch v := result.(type) {
	case *RootsListResult:
		return v, nil
	case map[string]any:
		return mapToStruct[RootsListResult](v)
	}

	return nil, errfmt.Errorf("unexpected result type: %T", result)
}

// SendLogMessage implements MCPServer.SendLogMessage
func (a *MCPServerAdapter) SendLogMessage(ctx context.Context, level LogLevel, message string, fields map[string]any) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Delegate to existing method (which doesn't take context)
		err := a.Server.SendLogMessage(level, message, fields)
		// Record metrics (dropped status would need to be tracked in SendLogMessage)
		a.metrics.RecordLogMessage(false) // TODO: Track dropped status
		return err
	}
}

// SendEvent implements MCPServer.SendEvent
func (a *MCPServerAdapter) SendEvent(ctx context.Context, event *Event) error {
	if a.eventEmitter != nil {
		a.eventEmitter.Emit(event)
	}
	a.metrics.RecordEvent()
	return nil
}

// SendMessage implements MCPServer.SendMessage
func (a *MCPServerAdapter) SendMessage(ctx context.Context, message, messageType, priority string) error {
	err := a.SendMessageToClient(message, messageType, priority)
	a.metrics.RecordMessage()
	return err
}

// NotifyCancelled implements MCPServer.NotifyCancelled
func (a *MCPServerAdapter) NotifyCancelled(ctx context.Context, params *CancelledParams) error {
	// This is typically handled by the client
	return nil
}

// MCPAsyncServer Implementation

// CallToolAsync implements MCPAsyncServer.CallToolAsync
func (a *MCPServerAdapter) CallToolAsync(ctx context.Context, params *ToolCallParams) <-chan AsyncToolResult {
	resultChan := make(chan AsyncToolResult, 1)
	a.metrics.RecordConcurrentOperation(1)
	goroutinelabels.NewGoroutine("mcp_tool_caller", fmt.Sprintf("calling tool %s asynchronously", params.Name)).
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			defer a.metrics.RecordConcurrentOperation(-1)
			result, err := a.CallTool(ctx, params)
			a.metrics.RecordAsyncToolCall(err)
			resultChan <- AsyncToolResult{
				Result: result,
				Error:  err,
			}
		})
	return resultChan
}

// ListToolsAsync implements MCPAsyncServer.ListToolsAsync
func (a *MCPServerAdapter) ListToolsAsync(ctx context.Context) <-chan AsyncToolsListResult {
	resultChan := make(chan AsyncToolsListResult, 1)
	goroutinelabels.NewGoroutine("mcp_tools_lister", "listing tools asynchronously").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			result, err := a.ListTools(ctx)
			resultChan <- AsyncToolsListResult{
				Result: result,
				Error:  err,
			}
		})
	return resultChan
}

// GetResourceAsync implements MCPAsyncServer.GetResourceAsync
func (a *MCPServerAdapter) GetResourceAsync(ctx context.Context, params *ResourceGetParams) <-chan AsyncResourceResult {
	resultChan := make(chan AsyncResourceResult, 1)
	goroutinelabels.NewGoroutine("mcp_resource_getter", fmt.Sprintf("getting resource %s asynchronously", params.URI)).
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			result, err := a.GetResource(ctx, params)
			resultChan <- AsyncResourceResult{
				Result: result,
				Error:  err,
			}
		})
	return resultChan
}

// ListResourcesAsync implements MCPAsyncServer.ListResourcesAsync
func (a *MCPServerAdapter) ListResourcesAsync(ctx context.Context, params *ResourcesListParams) <-chan AsyncResourcesListResult {
	resultChan := make(chan AsyncResourcesListResult, 1)
	goroutinelabels.NewGoroutine("mcp_resources_lister", "listing resources asynchronously").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			result, err := a.ListResources(ctx, params)
			resultChan <- AsyncResourcesListResult{
				Result: result,
				Error:  err,
			}
		})
	return resultChan
}

// GetPromptAsync implements MCPAsyncServer.GetPromptAsync
func (a *MCPServerAdapter) GetPromptAsync(ctx context.Context, params *PromptGetParams) <-chan AsyncPromptResult {
	resultChan := make(chan AsyncPromptResult, 1)
	goroutinelabels.NewGoroutine("mcp_prompt_getter", fmt.Sprintf("getting prompt %s asynchronously", params.Name)).
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			result, err := a.GetPrompt(ctx, params)
			resultChan <- AsyncPromptResult{
				Result: result,
				Error:  err,
			}
		})
	return resultChan
}

// ListPromptsAsync implements MCPAsyncServer.ListPromptsAsync
func (a *MCPServerAdapter) ListPromptsAsync(ctx context.Context) <-chan AsyncPromptsListResult {
	resultChan := make(chan AsyncPromptsListResult, 1)
	goroutinelabels.NewGoroutine("mcp_prompts_lister", "listing prompts asynchronously").
		WithCleanup(func() {
			close(resultChan)
		}).
		StartSimple(func() {
			result, err := a.ListPrompts(ctx)
			resultChan <- AsyncPromptsListResult{
				Result: result,
				Error:  err,
			}
		})
	return resultChan
}

// CallToolsBatch implements MCPAsyncServer.CallToolsBatch
func (a *MCPServerAdapter) CallToolsBatch(ctx context.Context, params []*ToolCallParams) <-chan AsyncBatchToolResult {
	resultChan := make(chan AsyncBatchToolResult, len(params))
	operationID := fmt.Sprintf("mcp_batch_tool_call_%d", time.Now().UnixNano())
	start := time.Now()

	a.metrics.RecordConcurrentOperation(int64(len(params)))
	goroutinelabels.NewGoroutine("mcp_batch_tool_caller", fmt.Sprintf("calling batch of %d tools (operation %s)", len(params), operationID)).
		WithCleanup(func() {
			duration := time.Since(start)
			a.metrics.RecordConcurrentOperation(-int64(len(params)))

			// Emit batch completion via coordinator
			if a.coordinator != nil {
				eventCtx := BuildMCPBatchToolCallEventContext(operationID, len(params), 0, duration)
				// Update error count from executor
				executor := NewParallelExecutor()
				errorCount := 0
				for _, param := range params {
					paramCopy := param
					executor.Execute(func() error {
						_, err := a.CallTool(ctx, paramCopy)
						if err != nil {
							errorCount++
						}
						return err
					})
				}
				_ = executor.Wait() //nolint:errcheck // Best effort - executor cleanup
				eventCtx.EventData.MetricsData["error_count"] = errorCount
				_ = a.coordinator.Emit(ctx, eventCtx) //nolint:errcheck
			}

			close(resultChan)
		}).
		StartSimple(func() {
			executor := NewParallelExecutor()
			errorCount := 0

			for i, param := range params {
				index := i
				paramCopy := param
				executor.Execute(func() error {
					result, err := a.CallTool(ctx, paramCopy)
					if err != nil {
						errorCount++
					}
					resultChan <- AsyncBatchToolResult{
						Index:  index,
						Result: result,
						Error:  err,
					}
					return err
				})
			}

			_ = executor.Wait() //nolint:errcheck // Best effort - executor cleanup
			a.metrics.RecordBatchToolCall(len(params), errorCount)
		})

	return resultChan
}

// GetResourcesBatch implements MCPAsyncServer.GetResourcesBatch
func (a *MCPServerAdapter) GetResourcesBatch(ctx context.Context, params []*ResourceGetParams) <-chan AsyncBatchResourceResult {
	resultChan := make(chan AsyncBatchResourceResult, len(params))

	a.metrics.RecordConcurrentOperation(int64(len(params)))
	goroutinelabels.NewGoroutine("mcp_batch_resource_getter", fmt.Sprintf("getting batch of %d resources", len(params))).
		WithCleanup(func() {
			a.metrics.RecordConcurrentOperation(-int64(len(params)))
			close(resultChan)
		}).
		StartSimple(func() {

			executor := NewParallelExecutor()

			for i, param := range params {
				index := i
				paramCopy := param
				executor.Execute(func() error {
					result, err := a.GetResource(ctx, paramCopy)
					resultChan <- AsyncBatchResourceResult{
						Index:  index,
						Result: result,
						Error:  err,
					}
					return err
				})
			}

			_ = executor.Wait() //nolint:errcheck // Best effort - executor cleanup
		})

	return resultChan
}

// GetMetricsSnapshot implements MCPAsyncServer.GetMetricsSnapshot
func (a *MCPServerAdapter) GetMetricsSnapshot() MetricsSnapshot {
	return a.metrics.GetSnapshot()
}

func mapToStruct[T any](m map[string]any) (*T, error) {
	resultJSON, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
