package swarm

import (
	"encoding/json"

	"github.com/lanceman/zqk/pkg/llm"
)

// TokenTracker tracks and validates the token budget per swarm step.
type TokenTracker struct {
	ContextWindow int
	Threshold     float64
}

// NewTokenTracker creates a new TokenTracker with specified context window and threshold.
func NewTokenTracker(contextWindow int, threshold float64) *TokenTracker {
	if contextWindow <= 0 {
		contextWindow = 32768
	}
	if threshold <= 0 || threshold > 1.0 {
		threshold = 0.9
	}
	return &TokenTracker{
		ContextWindow: contextWindow,
		Threshold:     threshold,
	}
}

// EstimateTokens calculates estimated tokens for messages and tools.
func (t *TokenTracker) EstimateTokens(messages []llm.Message, tools []llm.ToolDefinition) (int, int, int) {
	promptTokens := 0
	historyTokens := 0

	for i, m := range messages {
		tokens := t.EstimateMessageTokens(m)
		if i < 2 {
			promptTokens += tokens
		} else {
			historyTokens += tokens
		}
	}

	if len(tools) > 0 {
		tb, _ := json.Marshal(tools)
		promptTokens += len(tb) / 4
	}

	total := promptTokens + historyTokens
	return promptTokens, historyTokens, total
}

// EstimateMessageTokens estimates the token count for a single message.
func (t *TokenTracker) EstimateMessageTokens(m llm.Message) int {
	tokens := len(m.Content) / 4
	if len(m.ToolCalls) > 0 {
		tb, _ := json.Marshal(m.ToolCalls)
		tokens += len(tb) / 4
	}
	return tokens
}

// ValidateBudget checks if the total token count is within the budget threshold.
func (t *TokenTracker) ValidateBudget(totalTokens int) bool {
	limit := int(float64(t.ContextWindow) * t.Threshold)
	return totalTokens <= limit
}

// CompactHistory dynamically removes the oldest history messages until they fit within the budget.
func (t *TokenTracker) CompactHistory(messages []llm.Message, tools []llm.ToolDefinition) ([]llm.Message, bool) {
	// If messages contain only system (0) and user (1) prompts, we cannot compact further
	if len(messages) <= 2 {
		return messages, false
	}

	for len(messages) > 2 {
		_, _, total := t.EstimateTokens(messages, tools)
		if t.ValidateBudget(total) {
			return messages, true
		}

		// Remove the oldest turn (index 2 and 3).
		// If there is only one message left in history (odd count), just remove it.
		if len(messages) > 3 {
			messages = append(messages[:2], messages[4:]...)
		} else {
			messages = messages[:2]
		}
	}

	// Final check on just system + user prompts
	_, _, total := t.EstimateTokens(messages, tools)
	return messages, t.ValidateBudget(total)
}
