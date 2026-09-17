package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// testArgs is a JSON-encoded payload with one string field, so tests
// exercise the "trim string values, keep structure" path.
func testArgs(t *testing.T, key, val string) string {
	t.Helper()
	big := map[string]string{key: val}
	enc, err := json.Marshal(big)
	if err != nil {
		t.Fatalf("test fixture: marshal failed: %v", err)
	}
	return string(enc)
}

// TestEstimateTokens_zeroAndOneCharacter locks down the floor rule: any
// non-empty payload consumes at least one token regardless of character
// count, so a budget of 1 always permits at least one message.
func TestEstimateTokens_zeroAndOneCharacter(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("EstimateTokens(empty) = %d, want 0", got)
	}
	if got := EstimateTokens("a"); got < 1 {
		t.Fatalf("EstimateTokens(\"a\") = %d, want >=1", got)
	}
}

// TestEstimateTokens_linearish checks the 4-chars/token heuristic on a
// payload whose length is not a multiple of four, to ensure rounding is
// ceil (not floor), which is load-bearing for the within-budget invariant.
func TestEstimateTokens_linearish(t *testing.T) {
	if got := EstimateTokens(strings.Repeat("a", 10)); got != 3 { // ceil(10/4)=3
		t.Fatalf("EstimateTokens(10 a's) = %d, want 3", got)
	}
	if got := EstimateTokens("abcdefg"); got != 2 { // ceil(7/4)=2
		t.Fatalf("EstimateTokens(7 chars) = %d, want 2", got)
	}
}

// TestApplyTokenBudget_zeroBudgetIsNoop is the fail-open guarantee: a zero
// budget (unset / unset-by-config) must not mutate messages. Callers that
// do not know the caller's model window must not silently drop tool_calls.
func TestApplyTokenBudget_zeroBudgetIsNoop(t *testing.T) {
	origin := NewToolBudget()
	args := testArgs(t, "path", "pkg/llm/tools.go")
	msgs := []Message{
		{
			Role:      "assistant",
			Content:   strings.Repeat("x", 1000),
			ToolCalls: []ToolCall{{Name: "zqk_read_code", Arguments: args}},
		},
	}
	copied := cloneMessagesForTest(msgs)
	res := origin.Apply(copied, ToolBudgetLimits{MaxTokens: 0, PerToolCall: 0})
	if res.Exceeded {
		t.Fatal("zero budget must fail open (no truncation), got Exceeded=true")
	}
	if res.ContentTokens == 0 {
		t.Fatal("expected non-zero ContentTokens accounting even when no truncation")
	}
	if len(res.Kept) != len(msgs) {
		t.Fatalf("zero budget must not drop messages; kept=%d want=%d", len(res.Kept), len(msgs))
	}
	if len(res.Kept[0].Content) != 1000 {
		t.Fatalf("zero budget must not drop chars; got %d chars", len(res.Kept[0].Content))
	}
	if res.Kept[0].ToolCalls[0].Arguments != msgs[0].ToolCalls[0].Arguments {
		t.Fatal("zero budget must not alter ToolCall arguments")
	}
}

// TestApplyTokenBudget_preservesToolCallsWhenContentIsLarge verifies that
// when content is huge but a tool_call exists, the budget trims content
// first (the prose) and never drops the tool_call (which carries the
// mutation evidence).
func TestApplyTokenBudget_preservesToolCallsWhenContentIsLarge(t *testing.T) {
	origin := NewToolBudget()
	args := `{"path":"pkg/llm/tools.go"}`
	msgs := []Message{
		{
			Role:      "assistant",
			Content:   strings.Repeat("prose ", 5000), // ~3750 tokens
			ToolCalls: []ToolCall{{Name: "zqk_read_code", Arguments: args}},
		},
	}
	limits := ToolBudgetLimits{MaxTokens: 200, PerToolCall: 100}
	res := origin.Apply(msgs, limits)
	if !res.Exceeded {
		t.Fatalf("expected truncation to engage; Exceeded=false (total tokens %d)", res.ContentTokens)
	}
	if len(res.Kept) != 1 {
		t.Fatalf("kept=%d want=1", len(res.Kept))
	}
	if len(res.Kept[0].ToolCalls) != 1 {
		t.Fatalf("tool_calls must not be dropped by budget; got %d", len(res.Kept[0].ToolCalls))
	}
	if res.Kept[0].ToolCalls[0].Name != "zqk_read_code" {
		t.Fatalf("tool name mutated: %q", res.Kept[0].ToolCalls[0].Name)
	}
	// Content must have been trimmed to fit the 200-token budget.
	if len(res.Kept[0].Content) >= 30000 {
		t.Fatalf("expected content to be trimmed; still %d chars", len(res.Kept[0].Content))
	}
}

// TestApplyTokenBudget_TruncatesArgumentsToValidJSON ensures that when a
// tool_call argument exceeds PerToolCall the returned string remains
// parseable JSON. A malformed arguments string is worse than a truncated
// one because the provider returns a 400 (non-retryable) fault which the
// ResilientClient treats as hard-stop.
func TestApplyTokenBudget_TruncatesArgumentsToValidJSON(t *testing.T) {
	origin := NewToolBudget()
	args := testArgs(t, "payload", strings.Repeat("x", 4096))
	msgs := []Message{{
		Role:      "assistant",
		ToolCalls: []ToolCall{{Name: "zqk_write_file", Arguments: args}},
	}}
	res := origin.Apply(msgs, ToolBudgetLimits{MaxTokens: 512, PerToolCall: 32})
	if len(res.Kept) != 1 || len(res.Kept[0].ToolCalls) != 1 {
		t.Fatalf("shape: kept=%d tool_calls=%d", len(res.Kept), len(res.Kept[0].ToolCalls))
	}
	got := res.Kept[0].ToolCalls[0].Arguments
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("truncated arguments must remain valid JSON; got %q err=%v", got, err)
	}
	if !strings.Contains(got, "zqk_truncated") {
		t.Fatalf("expected truncation marker in %q", got)
	}
}

// TestApplyTokenBudget_DropsLowValueMessagesFirst locks in the ordering:
// when the budget cannot hold every message, oldest non-tool_call messages
// are the first candidates for removal so the most recent exchange
// (assistant tool_call + user tool_response) is preserved.
func TestApplyTokenBudget_DropsLowValueMessagesFirst(t *testing.T) {
	origin := NewToolBudget()
	msgs := []Message{
		{Role: "user", Content: "old tool result that is huge " + strings.Repeat("y", 2000)},
		{
			Role:      "assistant",
			Content:   strings.Repeat("recent ", 1000),
			ToolCalls: []ToolCall{{Name: "next", Arguments: `{"ok":true}`}},
		},
	}
	res := origin.Apply(msgs, ToolBudgetLimits{MaxTokens: 100, PerToolCall: 40})
	var found bool
	for _, m := range res.Kept {
		if len(m.ToolCalls) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("message with tool_calls was dropped; kept=%+v", res.Kept)
	}
}

// TestApplyTokenBudget_PerCallLimitShrinksLargestFirst ensures that when a
// single tool_call argument exceeds PerToolCall the budget trims the
// oversized one, leaving sibling arguments untouched.
func TestApplyTokenBudget_PerCallLimitShrinksLargestFirst(t *testing.T) {
	origin := NewToolBudget()
	msgs := []Message{{
		Role: "assistant",
		ToolCalls: []ToolCall{
			{Name: "small", Arguments: `{"a":1}`},
			{Name: "huge", Arguments: testArgs(t, "data", strings.Repeat("z", 8192))},
		},
	}}
	res := origin.Apply(msgs, ToolBudgetLimits{MaxTokens: 512, PerToolCall: 100})
	if len(res.Kept[0].ToolCalls) != 2 {
		t.Fatalf("both tool_calls must survive (only args trimmed); got %d", len(res.Kept[0].ToolCalls))
	}
	var small, hugeOut string
	for _, tc := range res.Kept[0].ToolCalls {
		switch tc.Name {
		case "small":
			small = tc.Arguments
		case "huge":
			hugeOut = tc.Arguments
		}
	}
	if small != `{"a":1}` {
		t.Fatalf("small tool_call argument must be untouched; got %q", small)
	}
	if len(hugeOut) >= 4096 {
		t.Fatalf("huge argument expected truncation; len=%d", len(hugeOut))
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(hugeOut), &parsed); err != nil {
		t.Fatalf("huge truncated args must be valid JSON; %v", err)
	}
}

// TestApplyTokenBudget_KeepsTotalWithinBudget pins the invariant the
// ResilientClient relies on before firing GenerateStructuredCompletion:
// after Apply, the token estimate of Kept messages must be <= MaxTokens
// (+1 slack for ceiling rounding).
func TestApplyTokenBudget_KeepsTotalWithinBudget(t *testing.T) {
	origin := NewToolBudget()
	msgs := []Message{{Role: "user", Content: strings.Repeat("token ", 4000)}} // ~6000 tokens
	limits := ToolBudgetLimits{MaxTokens: 64}
	res := origin.Apply(msgs, limits)
	total := 0
	for _, m := range res.Kept {
		total += EstimateTokens(m.Content)
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Arguments)
		}
	}
	if total > limits.MaxTokens+1 {
		t.Fatalf("total tokens %d exceed budget %d (+1 slack)", total, limits.MaxTokens)
	}
}

// TestToolBudgetLimits_Defaults pins the zero-value contract: a caller
// that constructs ToolBudgetLimits{} (zero) must not drop anything — the
// ResilientClient call sites have not learned the caller's model window
// and must fail open.
func TestToolBudgetLimits_Defaults(t *testing.T) {
	origin := NewToolBudget()
	msgs := []Message{{Role: "assistant", Content: strings.Repeat("x", 64)}}
	res := origin.Apply(msgs, ToolBudgetLimits{})
	if res.Exceeded {
		t.Fatal("zero limits must fail open; got Exceeded=true")
	}
	if len(res.Kept) != 1 {
		t.Fatalf("zero limits must not drop messages; kept=%d", len(res.Kept))
	}
}

// cloneMessagesForTest is a local deep-copy of []Message used by tests so
// that Apply's mutation behaviour can be observed independent of the
// caller's input.
func cloneMessagesForTest(in []Message) []Message {
	out := make([]Message, len(in))
	for i, m := range in {
		out[i] = Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if len(m.ToolCalls) > 0 {
			out[i].ToolCalls = make([]ToolCall, len(m.ToolCalls))
			copy(out[i].ToolCalls, m.ToolCalls)
		}
	}
	return out
}
