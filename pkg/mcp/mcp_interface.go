package mcp

import (
	"context"
)

// MCPServer defines the core MCP protocol interface
// This interface explicitly implements the MCP specification methods
// All methods follow JSON-RPC 2.0 semantics
type MCPServer interface {
	// Lifecycle Methods

	// Initialize initializes the MCP server with client capabilities
	// Method: "initialize"
	// Request: InitializeParams
	// Response: InitializeResult
	Initialize(ctx context.Context, params *InitializeParams) (*InitializeResult, error)

	// NotifyInitialized notifies the server that the client is ready
	// Method: "notifications/initialized"
	// This is a notification (no response)
	NotifyInitialized(ctx context.Context, params *InitializedParams) error

	// Shutdown gracefully shuts down the server
	// Method: "shutdown"
	// This is a notification (no response)
	Shutdown(ctx context.Context) error

	// Tools Methods

	// ListTools returns all available tools
	// Method: "tools/list"
	// Response: ToolsListResult
	ListTools(ctx context.Context) (*ToolsListResult, error)

	// CallTool executes a tool with the given arguments
	// Method: "tools/call"
	// Request: ToolCallParams
	// Response: ToolCallResult
	CallTool(ctx context.Context, params *ToolCallParams) (*ToolCallResult, error)

	// Resources Methods

	// ListResources returns all available resources
	// Method: "resources/list"
	// Request: ResourcesListParams (optional)
	// Response: ResourcesListResult
	ListResources(ctx context.Context, params *ResourcesListParams) (*ResourcesListResult, error)

	// GetResource retrieves a specific resource by URI
	// Method: "resources/get"
	// Request: ResourceGetParams
	// Response: ResourceGetResult
	GetResource(ctx context.Context, params *ResourceGetParams) (*ResourceGetResult, error)

	// Prompts Methods

	// ListPrompts returns all available prompts
	// Method: "prompts/list"
	// Response: PromptsListResult
	ListPrompts(ctx context.Context) (*PromptsListResult, error)

	// GetPrompt retrieves a specific prompt template
	// Method: "prompts/get"
	// Request: PromptGetParams
	// Response: PromptGetResult
	GetPrompt(ctx context.Context, params *PromptGetParams) (*PromptGetResult, error)

	// Roots Methods

	// ListRoots returns all available root resource URIs
	// Method: "roots/list"
	// Response: RootsListResult
	ListRoots(ctx context.Context) (*RootsListResult, error)

	// Notifications Methods (Server → Client)

	// SendLogMessage sends a log message to the client
	// Method: "notifications/logMessage"
	// This is a notification sent by the server
	SendLogMessage(ctx context.Context, level LogLevel, message string, fields map[string]any) error

	// SendEvent sends an event to the client
	// Method: "notifications/event"
	// This is a notification sent by the server
	SendEvent(ctx context.Context, event *Event) error

	// SendMessage sends a message to the client
	// Method: "notifications/message"
	// This is a notification sent by the server
	SendMessage(ctx context.Context, message, messageType, priority string) error

	// Cancellation

	// NotifyCancelled notifies that a request was cancelled
	// Method: "notifications/cancelled"
	// This is a notification (no response)
	NotifyCancelled(ctx context.Context, params *CancelledParams) error
}

// MCPAsyncServer extends MCPServer with async operations
// These methods return channels/futures for non-blocking execution
// This is an extension to the base MCP protocol for operations that benefit from async
type MCPAsyncServer interface {
	MCPServer

	// Async Tool Operations

	// CallToolAsync executes a tool asynchronously
	// Returns a channel that will receive the result when complete
	CallToolAsync(ctx context.Context, params *ToolCallParams) <-chan AsyncToolResult

	// ListToolsAsync lists tools asynchronously (useful for large tool sets)
	ListToolsAsync(ctx context.Context) <-chan AsyncToolsListResult

	// Async Resource Operations

	// GetResourceAsync retrieves a resource asynchronously
	// Useful for large resources or slow I/O
	GetResourceAsync(ctx context.Context, params *ResourceGetParams) <-chan AsyncResourceResult

	// ListResourcesAsync lists resources asynchronously
	// Useful for large resource sets or slow discovery
	ListResourcesAsync(ctx context.Context, params *ResourcesListParams) <-chan AsyncResourcesListResult

	// Async Prompt Operations

	// GetPromptAsync retrieves a prompt asynchronously
	GetPromptAsync(ctx context.Context, params *PromptGetParams) <-chan AsyncPromptResult

	// ListPromptsAsync lists prompts asynchronously
	ListPromptsAsync(ctx context.Context) <-chan AsyncPromptsListResult

	// Batch Operations (Async Extension)

	// CallToolsBatch executes multiple tools in parallel
	// Returns results in the same order as requests
	CallToolsBatch(ctx context.Context, params []*ToolCallParams) <-chan AsyncBatchToolResult

	// GetResourcesBatch retrieves multiple resources in parallel
	GetResourcesBatch(ctx context.Context, params []*ResourceGetParams) <-chan AsyncBatchResourceResult

	// Metrics Operations

	// GetMetricsSnapshot returns a snapshot of all MCP protocol metrics
	GetMetricsSnapshot() MetricsSnapshot
}

// Async Result Types

// AsyncToolResult represents the result of an async tool call
type AsyncToolResult struct {
	Result *ToolCallResult
	Error  error
}

// AsyncToolsListResult represents the result of async tools listing
type AsyncToolsListResult struct {
	Result *ToolsListResult
	Error  error
}

// AsyncResourceResult represents the result of an async resource retrieval
type AsyncResourceResult struct {
	Result *ResourceGetResult
	Error  error
}

// AsyncResourcesListResult represents the result of async resources listing
type AsyncResourcesListResult struct {
	Result *ResourcesListResult
	Error  error
}

// AsyncPromptResult represents the result of an async prompt retrieval
type AsyncPromptResult struct {
	Result *PromptGetResult
	Error  error
}

// AsyncPromptsListResult represents the result of async prompts listing
type AsyncPromptsListResult struct {
	Result *PromptsListResult
	Error  error
}

// AsyncBatchToolResult represents a batch tool call result
type AsyncBatchToolResult struct {
	Index  int
	Result *ToolCallResult
	Error  error
}

// AsyncBatchResourceResult represents a batch resource retrieval result
type AsyncBatchResourceResult struct {
	Index  int
	Result *ResourceGetResult
	Error  error
}

// Parameter Types (explicitly defined for interface clarity)

// InitializedParams represents parameters for notifications/initialized
type InitializedParams struct {
	// No parameters required by spec
}

// ResourcesListParams represents parameters for resources/list
type ResourcesListParams struct {
	// Optional: filter by URI pattern
	URIPattern string `json:"uriPattern,omitempty"`
}

// ResourcesListResult represents the result of resources/list
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// ResourceGetResult represents the result of resources/get
type ResourceGetResult struct {
	Contents []Content `json:"contents"`
	MimeType string    `json:"mimeType,omitempty"`
}

// PromptsListResult represents the result of prompts/list
type PromptsListResult struct {
	Prompts []Prompt `json:"prompts"`
}

// Note: ResourceGetParams, PromptGetParams, PromptGetResult, and PromptMessage
// are already defined in server_handlers.go - we reference those types

// RootsListResult represents the result of roots/list
type RootsListResult struct {
	Roots []string `json:"roots"`
}
