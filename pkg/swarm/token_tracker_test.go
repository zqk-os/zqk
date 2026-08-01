package swarm

import (
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
)

func TestTokenTracker_EstimateTokens(t *testing.T) {
	tracker := NewTokenTracker(100, 0.9)

	messages := []llm.Message{
		{Role: "system", Content: "12345678"}, // 2 tokens
		{Role: "user", Content: "1234"},       // 1 token
		{Role: "assistant", Content: "12"},    // 0 tokens (approx)
	}

	tools := []llm.ToolDefinition{
		{Name: "t1", Description: "d1"},
	}

	prompt, history, total := tracker.EstimateTokens(messages, tools)

	if prompt == 0 {
		t.Errorf("Expected prompt tokens > 0, got %d", prompt)
	}
	if total != prompt+history {
		t.Errorf("Expected total to equal prompt + history, got %d vs %d", total, prompt+history)
	}
}

func TestTokenTracker_CompactHistory(t *testing.T) {
	// Setup tracker with small window to force compaction
	tracker := NewTokenTracker(10, 0.9) // Budget limit = 9 tokens

	messages := []llm.Message{
		{Role: "system", Content: "1234"},                        // 1 token
		{Role: "user", Content: "1234"},                          // 1 token
		{Role: "assistant", Content: "123456781234567812345678"}, // 6 tokens
		{Role: "tool", Content: "123456781234567812345678"},      // 6 tokens
		{Role: "assistant", Content: "12"},                       // 0 tokens
	}

	compacted, ok := tracker.CompactHistory(messages, nil)
	if !ok {
		t.Errorf("Expected compaction to succeed within budget")
	}

	// The large historical messages should have been removed, leaving only system, user, and the latest short assistant message
	if len(compacted) > 3 {
		t.Errorf("Expected compacted messages length to be at most 3, got %d", len(compacted))
	}
}
