package llm

import "strings"

// ModelRole is how a chat model should be used in AgentX / swarm.
// TRACK: BLI-SWM-002 — Coder-7B is a writer, not an OpenAI tools doer.
type ModelRole string

const (
	// ModelRoleAgentDoer can populate native OpenAI tool_calls (or is assumed to).
	ModelRoleAgentDoer ModelRole = "agent_doer"
	// ModelRoleCodeDraft is strong at writing file bodies and weak at native
	// function-calling. Qwen2.5-Coder was trained on the Qwen2/Qwen-Agent
	// template, not Hermes, so /v1/chat/completions returns content JSON
	// instead of tool_calls.
	ModelRoleCodeDraft ModelRole = "code_draft"
)

// ClassifyChatModel decides the harness for a configured chat model id
// (Ollama tag, Hugging Face id, or provider name).
func ClassifyChatModel(model string) ModelRole {
	n := strings.ToLower(strings.TrimSpace(model))
	if n == "" {
		return ModelRoleAgentDoer
	}
	if strings.Contains(n, "qwen2.5-coder") || strings.Contains(n, "qwen2.5coder") {
		return ModelRoleCodeDraft
	}
	if strings.Contains(n, "coder") && (strings.Contains(n, "7b") || strings.Contains(n, "8b")) {
		return ModelRoleCodeDraft
	}
	return ModelRoleAgentDoer
}

// IsCodeDraftModel reports whether the model should use the write-only harness.
func IsCodeDraftModel(model string) bool {
	return ClassifyChatModel(model) == ModelRoleCodeDraft
}
