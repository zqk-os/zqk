package swarm

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/llm"
)

// CodeDraftToolNames is the write-focused menu for models that cannot do
// OpenAI native tool_calls. Lookup/status tools are omitted on purpose:
// Coder-7B otherwise spends the run on object_get and parks with no write.
func CodeDraftToolNames(prefix string) []string {
	p := strings.TrimSpace(prefix)
	return []string{
		p + "write_code",
		p + "write_file",
		p + "read_code",
		p + "observer_search",
		p + "execute_bash",
	}
}

// CodeDraftSystemGuidance overrides the doer prompt's "object_get first"
// teaching. The host still recovers markdown / Hermes JSON.
func CodeDraftSystemGuidance() string {
	p := DefaultToolPrefix()
	return strings.TrimSpace(`
## Code-draft role (this model)
You are a file writer, not a planner or kernel researcher.
Do not call ` + p + `object_get, ` + p + `object_list, ` + p + `system_status, or mcp_list_tools.
First action: ` + p + `write_code with path + the full file contents.
Then ` + p + `execute_bash only for go test / go vet on the package you wrote.
If native tool_calls are unavailable, emit one JSON object with name and arguments (the host will run it).
`)
}

// ApplyCodeDraftHarness restricts the engine when the configured chat model
// is a coder-draft. Returns true when the harness was applied.
func ApplyCodeDraftHarness(e *Engine, model string) bool {
	if e == nil || !llm.IsCodeDraftModel(model) {
		return false
	}
	e.RestrictTools(CodeDraftToolNames(DefaultToolPrefix())...).
		WithExtraSystem(CodeDraftSystemGuidance())
	return true
}

func filterToolsByAllowlist(tools []llm.ToolDefinition, allow []string) []llm.ToolDefinition {
	if len(allow) == 0 {
		return tools
	}
	out := make([]llm.ToolDefinition, 0, len(allow))
	for _, t := range tools {
		if toolNameAllowed(t.Name, allow) {
			out = append(out, t)
		}
	}
	return out
}
