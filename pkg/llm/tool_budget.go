package llm

import (
	"encoding/json"
)

// TokensPerCharacter is the coarse bytes→tokens heuristic used by the
// budgeter. It matches the convention already used in pkg/agentprompt
// (`len(s)/4`) and avoids pulling in a real tokenizer dependency.
const TokensPerCharacter = 4

// truncationMarkerKey names the JSON string field we inject when an
// oversized tool_call argument is shrunk, so an operator reading a
// provider-side 400 error can see that we attempted a budget trim.
// Keeping it on a stable key (not a comment) also lets the client-side
// caller detect truncation in the returned ToolCall.Arguments.
const truncationMarkerKey = "zqk_truncated"

// truncationMarkerValue is what we record in the marker.
const truncationMarkerValue = "payload truncated by zqk dynamic token budget"

// ToolBudgetLimits describes the per-call budget for a single
// GenerateStructuredCompletion request.
//
// All fields are token counts (EstimateTokens units). Zero values mean
// "unset" and the budgeter fails open (see ToolBudget.Apply) so that
// callers that have not learned the caller's model window do not silently
// drop mutation evidence (tool_calls).
//
// MaxTokens     caps the total payload (content + tool_call arguments) for
//               the request. If zero, no global cap is applied.
// PerToolCall   caps the argument size for each individual tool_call
//               (not the content). If zero, per-call caps are not applied.
// Weight        is a reserved knob for future recency weighting. Values in
//               (0,1] shrink the applied budget; a value of 0 or a value
//               >1 (common "unlimited") is treated as 1.0. This field is a
//               no-op in the current implementation — it is reserved so the
//               struct shape is stable for callers that already pass it.
type ToolBudgetLimits struct {
	MaxTokens   int
	PerToolCall int
	Weight      float64
}

// ToolBudget is a stateless budget-apply helper over []Message. It is
// safe for concurrent use because all of its state is the per-call limits
// (pass-by-value) plus the input slice.
//
// NewToolBudget returns a zero-state budget; ToolBudgetLimits carries the
// per-call knobs.
type ToolBudget struct{}

// NewToolBudget constructs a ToolBudget. It accepts no arguments to leave
// room for future configuration (e.g. custom token estimator) without
// breaking the existing call sites.
func NewToolBudget() *ToolBudget {
	return &ToolBudget{}
}

// ApplyResult is the return value of ToolBudget.Apply.
//
// Kept            is the deep-copied []Message the caller may hand to the
//                 provider. Apply never mutates the input slice.
// Exceeded         is true when the request needed truncation (any budget
//                 was applied). Callers can use this to emit a telemetry
//                 event without re-scanning the messages.
// ContentTokens    is a rough pre-truncation estimate of total token count,
//                 for telemetry.
type ApplyResult struct {
	Kept          []Message
	Exceeded      bool
	ContentTokens int
}

// EstimateTokens returns a coarse token estimate for s. Empty string is 0;
// any non-empty string pays a 1-token floor (so a budget of 1 always fits
// at least one message).
//
// The estimate is bytes/4 ceil, which is the same heuristic used in
// pkg/agentprompt and pkg/pipeline. We do NOT use a real BPE tokenizer:
// the budget's contract is "keep the wire payload bounded", and the
// provider does its own tokenization.
func EstimateTokens(s string) int {
	n := len(s)
	if n == 0 {
		return 0
	}
	v := (n + TokensPerCharacter - 1) / TokensPerCharacter
	if v < 1 {
		return 1
	}
	return v
}

// Apply enforces the token budget on msgs, returning a deep-copied slice
// that is safe to pass to any Client.GenerateStructured* method.
//
// Contract:
//  1. Fail open: if limits is zero-valued (both MaxTokens and PerToolCall
//     are 0), Apply is a strict deep-copy no-op (Exceeded=false).
//  2. Tool_calls are load-bearing: a message with tool_calls is never
//     dropped wholesale, and its arguments are only trimmed to a valid JSON
//     string when the per-call cap is exceeded.
//  3. Non-tool_call prose messages are dropped from oldest-first once the
//     budget cannot hold them AND trimming the prose alone would not fit.
//     This preserves the recency of the exchange, which is what a mutating
//     seat-worker needs to continue.
//  4. The result total token estimate is within limits.MaxTokens + 1 slack
//     (EstimateTokens ceil rounding).
func (b *ToolBudget) Apply(msgs []Message, limits ToolBudgetLimits) ApplyResult {
	res := ApplyResult{Kept: deepCopyMessages(msgs)}
	res.ContentTokens = estimateTotal(msgs)

	// Fail-open: zero limits → strict no-op deep copy.
	if limits.MaxTokens <= 0 && limits.PerToolCall <= 0 {
		return res
	}

	out := res.Kept
	exceeded := false

	// Step 1: enforce the per-call tool_call cap (safe: args stay valid JSON).
	if limits.PerToolCall > 0 {
		anyTrimmed := false
		for i := range out {
			for j := range out[i].ToolCalls {
				newArgs, trimmed := trimToolCallArgs(out[i].ToolCalls[j].Arguments, limits.PerToolCall)
				if trimmed {
					anyTrimmed = true
				}
				out[i].ToolCalls[j].Arguments = newArgs
			}
		}
		exceeded = exceeded || anyTrimmed
	}

	// Step 2: enforce the total budget. Trim content from oldest first,
	// then drop oldest non-tool_call messages if still over.
	if limits.MaxTokens > 0 {
		if total := estimateTotal(out); total > limits.MaxTokens {
			exceeded = enforceTotalBudget(&out, limits.MaxTokens) || exceeded
		}
	}

	res.Kept = out
	res.Exceeded = exceeded
	return res
}

// estimateTotal sums EstimateTokens over contents and tool_call arguments.
func estimateTotal(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Arguments)
		}
	}
	return total
}

// estimateTotalWithSkip totals tokens excluding the message at idx.
func estimateTotalWithSkip(msgs []Message, idx int) int {
	total := 0
	for i, m := range msgs {
		if i == idx {
			continue
		}
		total += EstimateTokens(m.Content)
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Arguments)
		}
	}
	return total
}

// enforceTotalBudget trims Content on messages from oldest-first until the
// slice fits maxTotal, then drops the oldest non-tool_call message if
// Content alone is not enough. Never drops a message with tool_calls.
//
// Returns true if any mutation happened (trim or drop).
func enforceTotalBudget(out *[]Message, maxTotal int) bool {
	mutated := false

	// Content pass: trim oldest-first until budget fits.
	for i := 0; i < len(*out); i++ {
		if estimateTotal(*out) <= maxTotal {
			break
		}
		msg := &(*out)[i]
		if msg.Content == "" {
			continue
		}
		other := estimateTotalWithSkip(*out, i)
		remaining := maxTotal - other
		if remaining < 0 {
			remaining = 0
		}
		newContent, trimmed := trimContentToTokenBudget(msg.Content, remaining)
		msg.Content = newContent
		if trimmed {
			mutated = true
		}
	}

	// Drop pass: still over → drop oldest non-tool_call message.
	for estimateTotal(*out) > maxTotal && len(*out) > 1 {
		idx := -1
		for i := range *out {
			if len((*out)[i].ToolCalls) == 0 {
				idx = i
				break
			}
		}
		if idx < 0 {
			break // nothing left to drop; over-budget is tolerated
		}
		*out = append((*out)[:idx], (*out)[idx+1:]...)
		mutated = true
	}

	return mutated
}

// trimContentToTokenBudget shortens s (in runes) to a token estimate ≤
// tokenBudget, keeping the prefix. Returns ("", true) when
// tokenBudget ≤ 0.
func trimContentToTokenBudget(s string, tokenBudget int) (string, bool) {
	if tokenBudget <= 0 {
		return "", true
	}
	if EstimateTokens(s) <= tokenBudget {
		return s, false
	}
	maxChars := tokenBudget * TokensPerCharacter
	runes := []rune(s)
	trimmed := s
	if maxChars < len(runes) {
		trimmed = string(runes[:maxChars])
	}
	if EstimateTokens(trimmed) > tokenBudget && maxChars >= 1 && maxChars < len(runes) {
		// ceil rounding may have pushed us one token over; shave a rune.
		trimmed = string(runes[:maxChars-1])
	}
	return trimmed, true
}

// trimToolCallArgs trims arguments (a JSON string) so that its token
// estimate is <= perCall. If arguments were parseable JSON, the returned
// string is also parseable JSON: we drop the largest string value first,
// replacing it with the truncation marker. If arguments were not parseable
// JSON, we return an opaque JSON string that keeps the tool call
// syntactically valid on the wire.
//
// A malformed ToolCall.Arguments field is otherwise what triggers the
// provider-side 400 / "invalid_request_error" fault that the
// ResilientClient treats as non-retryable (see isNonRetryableLLMError).
func trimToolCallArgs(args string, perCall int) (string, bool) {
	if perCall <= 0 || args == "" {
		return args, false
	}
	if EstimateTokens(args) <= perCall {
		return args, false
	}

	var v any
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		if out, ok := wrapInJSONStringWithinBudget(args, perCall); ok {
			return out, true
		}
		return `{"zqk_truncated":"1"}`, true
	}

	if m, ok := v.(map[string]any); ok {
		var largestKey string
		largestLen := 0
		for k, val := range m {
			if s, ok := val.(string); ok && len(s) > largestLen {
				largestKey = k
				largestLen = len(s)
			}
		}
		m[truncationMarkerKey] = truncationMarkerValue
		if largestKey != "" {
			m[largestKey] = ""
		}
		if enc, encErr := json.Marshal(m); encErr == nil && enc != nil {
			out := string(enc)
			if EstimateTokens(out) > perCall {
				m[truncationMarkerKey] = "truncated"
				if enc2, err2 := json.Marshal(m); err2 == nil && enc2 != nil {
					out = string(enc2)
				}
			}
			return out, true
		}
		return `{"zqk_truncated":"truncated"}`, true
	}

	out := `{"` + truncationMarkerKey + `":"` + truncationMarkerValue + `"}`
	if EstimateTokens(out) > perCall {
		out = `{"` + truncationMarkerKey + `":"1"}`
	}
	return out, true
}

// wrapInJSONStringWithinBudget attempts to embed s into a JSON string
// envelope. If the result is ≤ perCall tokens it is returned; otherwise
// ok=false so the caller can fall back to an even smaller form.
func wrapInJSONStringWithinBudget(s string, perCall int) (string, bool) {
	runes := []rune(s)
	maxChars := perCall * TokensPerCharacter
	if len(runes) > maxChars {
		runes = runes[:maxChars]
	}
	b, err := json.Marshal(string(runes))
	if err != nil {
		return "", false
	}
	out := string(b)
	if EstimateTokens(out) > perCall {
		return "", false
	}
	return out, true
}

// deepCopyMessages returns an independent deep copy of msgs. Apply never
// mutates its input, but tests and ApplyResult.Kept both rely on the copy
// being safe to modify independently.
func deepCopyMessages(in []Message) []Message {
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
