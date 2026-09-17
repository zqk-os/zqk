package swarm

import (
	"strings"
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

func TestTokenTracker_FitUserPrompt(t *testing.T) {
	tracker := NewTokenTracker(100, 0.9) // limit = 90 tokens ≈ 360 chars
	system := "sys"                      // ~1 token
	huge := strings.Repeat("abcd", 2000) // ~2000 tokens

	fitted, total, ok := tracker.FitUserPrompt(system, huge)
	if !ok {
		t.Fatalf("expected FitUserPrompt to succeed by truncating, total=%d limit=%d", total, tracker.BudgetLimit())
	}
	if len(fitted) >= len(huge) {
		t.Errorf("expected truncation, fitted len=%d original=%d", len(fitted), len(huge))
	}
	if !strings.Contains(fitted, "truncated for token budget") {
		t.Errorf("expected truncation marker in fitted prompt")
	}
	if !tracker.ValidateBudget(total) {
		t.Errorf("fitted total %d exceeds budget %d", total, tracker.BudgetLimit())
	}
}

func TestTokenTracker_FitUserPrompt_TooSmall(t *testing.T) {
	tracker := NewTokenTracker(8, 0.5) // limit = 4 tokens
	system := strings.Repeat("x", 40)  // 10 tokens alone
	_, total, ok := tracker.FitUserPrompt(system, "tiny")
	if ok {
		t.Fatalf("expected failure when system alone exceeds budget, total=%d", total)
	}
}
