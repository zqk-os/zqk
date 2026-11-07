package llmtranscript

import "time"

// TranscriptStep models a single JSONL event emitted in the agent transcript log.
type TranscriptStep struct {
	StepIndex       int        `json:"step_index"`
	Source          string     `json:"source"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	CreatedAt       string     `json:"created_at"`
	InputTokens     int64      `json:"input_tokens,omitempty"`
	CacheReadTokens int64      `json:"cache_read_tokens,omitempty"`
	OutputTokens    int64      `json:"output_tokens,omitempty"`
	Thinking        string     `json:"thinking,omitempty"`
	ToolCalls       []ToolCall `json:"tool_calls,omitempty"`
	Content         string     `json:"content,omitempty"`
	Error           string     `json:"error,omitempty"`
	TruncatedFields []string   `json:"truncated_fields,omitempty"`
}

// ToolCall models an individual tool call issued by the model planner.
type ToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// ToolExecution represents the evaluated outcome of a tool call.
type ToolExecution struct {
	ToolName     string         `json:"tool_name"`
	Args         map[string]any `json:"args,omitempty"`
	Success      bool           `json:"success"`
	ExitCode     int            `json:"exit_code"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StepIndex    int            `json:"step_index"`
	DurationMs   int64          `json:"duration_ms,omitempty"`
}

// Turn represents an interaction turn containing planner responses and subsequent tool outcomes.
type Turn struct {
	StepIndex         int             `json:"step_index"`
	Model             string          `json:"model"`
	InputTokens       int64           `json:"input_tokens"`
	OutputTokens      int64           `json:"output_tokens"`
	CacheReadTokens   int64           `json:"cache_read_tokens"`
	ToolCalls         []ToolCall      `json:"tool_calls"`
	Executions        []ToolExecution `json:"executions"`
	IsSuccess         bool            `json:"is_success"`
	IsSelfCorrection  bool            `json:"is_self_correction"`
	CorrectedPrevious bool            `json:"corrected_previous"`
}

// FailurePattern records a common error archetype for prompt tuning diagnostics.
type FailurePattern struct {
	Tool           string `json:"tool"`
	Pattern        string `json:"pattern"`
	Count          int    `json:"count"`
	Sample         string `json:"sample,omitempty"`
	Recommendation string `json:"recommendation"`
}

// ModelMetrics summarizes empirical performance, correctness, and token economy for a model.
type ModelMetrics struct {
	Model                   string           `json:"model"`
	TotalTurns              int              `json:"total_turns"`
	TotalToolCalls          int              `json:"total_tool_calls"`
	SuccessfulToolCalls     int              `json:"successful_tool_calls"`
	FailedToolCalls         int              `json:"failed_tool_calls"`
	ToolAccuracy            float64          `json:"tool_accuracy"`
	SelfCorrectionAttempts  int              `json:"self_correction_attempts"`
	SelfCorrectionSuccesses int              `json:"self_correction_successes"`
	SelfCorrectionRate      float64          `json:"self_correction_rate"`
	TotalInputTokens        int64            `json:"total_input_tokens"`
	TotalOutputTokens       int64            `json:"total_output_tokens"`
	TotalCacheReadTokens    int64            `json:"total_cache_read_tokens"`
	TokensPerSuccess        float64          `json:"tokens_per_success"`
	CompositeScore          float64          `json:"composite_score"`
	Rank                    int              `json:"rank"`
	TopFailurePatterns      []FailurePattern `json:"top_failure_patterns,omitempty"`
	PromptHoningTips        []string         `json:"prompt_honing_tips,omitempty"`
}

// EvaluationReport contains aggregated evaluations across multiple conversations or logs.
type EvaluationReport struct {
	GeneratedAt           time.Time      `json:"generated_at"`
	ConversationsAnalyzed int            `json:"conversations_analyzed"`
	TotalSteps            int            `json:"total_steps"`
	TotalTurns            int            `json:"total_turns"`
	ModelRankings         []ModelMetrics `json:"model_rankings"`
}
