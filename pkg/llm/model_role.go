package llm

import "strings"

// ModelRole is how a chat model should be used in AgentX / swarm.
type ModelRole string

const (
	// ModelRoleAgentDoer can populate native tool_calls (or is assumed to).
	ModelRoleAgentDoer ModelRole = "agent_doer"
	// ModelRoleCodeDraft is strong at writing file bodies and weak at native
	// function-calling. Small "coder" tags typically return content JSON instead
	// of tool_calls.
	ModelRoleCodeDraft ModelRole = "code_draft"
)

// ClassifyChatModel decides the harness for a configured chat model id.
// Vendor-specific ids (Qwen2.5-Coder, …) are classified by registered adapters.
func ClassifyChatModel(model string) ModelRole {
	n := strings.ToLower(strings.TrimSpace(model))
	if n == "" {
		return ModelRoleAgentDoer
	}
	for _, a := range snapshotProviderAdapters() {
		c, ok := a.(ModelClassifier)
		if !ok {
			continue
		}
		if role, hit := c.ClassifyModel(model); hit {
			return role
		}
	}
	if strings.Contains(n, "coder") && (strings.Contains(n, "7b") || strings.Contains(n, "8b") || strings.Contains(n, "6.7b")) {
		return ModelRoleCodeDraft
	}
	return ModelRoleAgentDoer
}

// IsCodeDraftModel reports whether the model should use the write-only harness.
func IsCodeDraftModel(model string) bool {
	return ClassifyChatModel(model) == ModelRoleCodeDraft
}
