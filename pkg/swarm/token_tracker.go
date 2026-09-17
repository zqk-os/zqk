package swarm

import (
	"encoding/json"

	"github.com/lanceman/zqk/pkg/llm"
)

// TokenTracker tracks and validates the token budget per swarm step.
// TRACK: BLI-1783631892661332000-b2cd615e
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

// BudgetLimit is the max allowed tokens (context window × threshold).
func (t *TokenTracker) BudgetLimit() int {
	return int(float64(t.ContextWindow) * t.Threshold)
}

// EstimateText estimates tokens for a free-form string (~4 chars/token).
func (t *TokenTracker) EstimateText(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / 4
	if n == 0 && len(s) > 0 {
		return 1
	}
	return n
}

// FitUserPrompt truncates user content until system+user fit the budget.
// Returns fitted user prompt, estimated total tokens, and whether it fit.
func (t *TokenTracker) FitUserPrompt(system, user string) (fittedUser string, total int, ok bool) {
	fittedUser = user
	for {
		total = t.EstimateText(system) + t.EstimateText(fittedUser)
		if t.ValidateBudget(total) {
			return fittedUser, total, true
		}
		if len(fittedUser) < 256 {
			return fittedUser, total, false
		}
		keep := len(fittedUser) / 2
		fittedUser = fittedUser[:keep] + "\n…[truncated for token budget]"
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
	return totalTokens <= t.BudgetLimit()
}

// CompactHistory dynamically removes the oldest history messages until they fit within the budget.
func (t *TokenTracker) CompactHistory(messages []llm.Message, tools []llm.ToolDefinition) ([]llm.Message, bool) {
	if len(messages) <= 2 {
		return messages, false
	}

	for len(messages) > 2 {
		_, _, total := t.EstimateTokens(messages, tools)
		if t.ValidateBudget(total) {
			return messages, true
		}

		if len(messages) > 3 {
			messages = append(messages[:2], messages[4:]...)
		} else {
			messages = messages[:2]
		}
	}

	_, _, total := t.EstimateTokens(messages, tools)
	return messages, t.ValidateBudget(total)
}
