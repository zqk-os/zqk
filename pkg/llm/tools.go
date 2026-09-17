package llm

// ToolDefinition represents a tool that the LLM can call.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema for parameters
}

// ToolCall represents a tool call requested by the LLM.
type ToolCall struct {
	ID        string `json:"id,omitempty"` // Used by OpenAI
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // Raw JSON string of arguments
}

// Message represents a conversation message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // Used for tool responses
	Name       string     `json:"name,omitempty"`         // Used for tool responses
}

// StructuredCompletionResponse is the result of a structured completion call.
type StructuredCompletionResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
