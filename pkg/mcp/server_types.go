package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"time"
)

// ClientConnection represents a connected client's transport information
// This allows routing messages to specific clients by ID
type ClientConnection struct {
	ID       string
	Writer   *bufio.Writer
	Format   *MessageFormat
	LastSeen time.Time
	// Message queue for backpressure relief
	Queue *MessageQueue
}

// Tool represents an MCP tool that can be called
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema any         `json:"inputSchema"`
	Handler     ToolHandler `json:"-"` // Handler function (not serialized)
}

// Resource represents an MCP resource that can be accessed
type Resource struct {
	URI         string            `json:"uri"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	MimeType    string            `json:"mimeType,omitempty"`
	Category    string            `json:"category,omitempty"` // e.g., "guide", "architecture", "policy", "lifecycle"
	Priority    string            `json:"priority,omitempty"` // e.g., "critical", "reference", "optional"
	Tags        []string          `json:"tags,omitempty"`     // Additional tags for categorization
	Metadata    map[string]string `json:"metadata,omitempty"` // Additional metadata
}

// Prompt represents an MCP prompt template
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents an argument for a prompt
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

// Request represents an MCP JSON-RPC request
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents an MCP JSON-RPC response
// Deprecated: Use JSONRPCResponse from protocol.go instead
// This type is kept for reference but should not be used in new code
type Response struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// InitializeParams represents initialization parameters
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// InitializeResult represents initialization result
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	ClientID string `json:"clientId,omitempty"` // Client ID assigned by server
}

// ToolsListResult represents the result of listing tools
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams represents parameters for calling a tool
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolCallResult represents the result of calling a tool
type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents content in a tool result (ITEM-958: MimeType for content-type handling)
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	MimeType string `json:"mimeType,omitempty"`
}

// CancelledParams represents the payload for notifications/cancelled
type CancelledParams struct {
	RequestID any    `json:"requestId"`
	Reason    string `json:"reason"`
}

// ToolHandler is a function that handles tool calls
type ToolHandler func(ctx context.Context, args map[string]any) (any, error)
