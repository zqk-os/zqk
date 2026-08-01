package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// JSONRPCRequest is the standard JSON-RPC 2.0 request structure
// This is the common shape all MCP requests unmarshal to
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`          // Always "2.0"
	ID      any             `json:"id,omitempty"`     // Request ID (number, string, or null)
	Method  string          `json:"method"`           // Method name (e.g., "initialize", "tools/call")
	Params  json.RawMessage `json:"params,omitempty"` // Method parameters (raw JSON for flexible unmarshaling)
}

// JSONRPCResponse is the standard JSON-RPC 2.0 response structure
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`          // Always "2.0"
	ID      any           `json:"id,omitempty"`     // Request ID (matches request)
	Result  any           `json:"result,omitempty"` // Success result
	Error   *JSONRPCError `json:"error,omitempty"`  // Error (if any)
}

// JSONRPCError is the standard JSON-RPC 2.0 error structure
type JSONRPCError struct {
	Code    int    `json:"code"`           // Error code (standard or custom)
	Message string `json:"message"`        // Error message
	Data    any    `json:"data,omitempty"` // Additional error data
}

// Error implements the error interface
func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// JSON-RPC 2.0 constants
const (
	JSONRPCVersion = "2.0" // JSON-RPC protocol version
)

// JSON-RPC field name constants
const (
	jsonrpcFieldJSONRPC = "jsonrpc"
	jsonrpcFieldMethod  = "method"
	jsonrpcFieldParams  = "params"
)

// MCP notification method constants
const (
	notificationMethodLogMessage = "notifications/logMessage"
	notificationMethodEvent      = "notifications/event"
	notificationMethodMessage    = "notifications/message"
)

// Error codes are now defined in error_codes.go
// This file maintains JSON-RPC protocol structures and utilities
// Import error_codes.go to access error code constants

// UnmarshalRequest unmarshals raw message bytes into a JSONRPCRequest
// This provides a common entry point for all MCP requests
func UnmarshalRequest(data []byte) (*JSONRPCRequest, error) {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, errfmt.Newf("failed to unmarshal JSON-RPC request").Wrap(err)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != JSONRPCVersion {
		return nil, errfmt.Errorf("unsupported JSON-RPC version: %s", req.JSONRPC)
	}

	return &req, nil
}

// NewResponse creates a new JSON-RPC response
func NewResponse(id any) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
	}
}

// NewErrorResponse creates a new JSON-RPC error response
// Uses ErrorResponseBuilder for consistency with the rest of the codebase
func NewErrorResponse(id any, code int, message string, data any) *JSONRPCResponse {
	builder := NewErrorResponseBuilder(code, message)

	// Convert data to map structure for consistent error format
	if data != nil {
		if dataMap, ok := data.(map[string]any); ok {
			builder.WithDataMap(dataMap)
		} else {
			// Non-map data - wrap in standard "data" field
			builder.WithData("data", data)
		}
	}

	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   builder.Build(),
	}
}

// Marshal marshals the response to JSON
func (r *JSONRPCResponse) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// IsNotification returns true if the request is a notification (no ID)
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil
}

// UnmarshalParams unmarshals the params into the target type
func (r *JSONRPCRequest) UnmarshalParams(target any) error {
	if len(r.Params) == 0 {
		return nil // No params to unmarshal
	}
	return json.Unmarshal(r.Params, target)
}
