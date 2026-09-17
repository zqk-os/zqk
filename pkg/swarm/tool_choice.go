package swarm

import (
	"strings"

	"github.com/lanceman/zqk/pkg/llm"
)

type toolChoiceState struct {
	ToolsPresent          bool
	NeedExecutionEvidence bool
	MarkdownRecoveries    int
	NoToolCorrections     int
	PinTool               string
}

// nextToolChoice pins native function-calling while a 7B still owes work or
// after it slipped into markdown. Empty omits the field (no tools registered).
// Local servers that reject tool_choice are handled by a one-shot retry in
// the OpenAI client — not an env toggle. TRACK: BLI-SWM-002
func nextToolChoice(state toolChoiceState) string {
	if !state.ToolsPresent {
		return ""
	}
	if state.NeedExecutionEvidence || state.MarkdownRecoveries > 0 {
		return llm.ToolChoiceRequired
	}
	return llm.ToolChoiceAuto
}

// nextToolChoiceFunction names the owed mutation tool while evidence is
// still unpaid. Live 7B accepts the string "required" on every turn and
// still returns toolCalls=0, so waiting for a slip just burns step 0.
func nextToolChoiceFunction(state toolChoiceState) string {
	if !state.NeedExecutionEvidence || strings.TrimSpace(state.PinTool) == "" {
		return ""
	}
	return strings.TrimSpace(state.PinTool)
}

// unpaidLookupBudget is how many consecutive non-write tools may execute
// while a mutation pin is still unpaid. After this, the host hides
// object_get/list/status on the wire, narrowing to code-drafting tools.
// TRACK: BLI-SWM-002
const unpaidLookupBudget = 4

// unpaidLookupForceWrite hides read_code/observer_search too if a model
// runs an extensive streak without writing changes, forcing mutation evidence.
const unpaidLookupForceWrite = 16

// UnpaidLookupStreak counts trailing tool calls that are not a write.
// object_get, read_code, invented list tools, and bash all count:
// any of them reset the old context-gathering detector and never
// pay the mutation pin. A successful write_code/write_file resets.
func UnpaidLookupStreak(history []ToolCallRecord) int {
	n := 0
	for i := len(history) - 1; i >= 0; i-- {
		if IsMutationEvidenceTool(history[i].Name) {
			break
		}
		n++
	}
	return n
}

// ShouldNarrowToWriteMenu reports that lookup tools should leave the wire.
func ShouldNarrowToWriteMenu(needEvidence bool, history []ToolCallRecord) bool {
	return needEvidence && UnpaidLookupStreak(history) >= unpaidLookupBudget
}

// ShouldForceWriteOnly reports that only write_code/write_file stay on the wire.
func ShouldForceWriteOnly(needEvidence bool, history []ToolCallRecord) bool {
	return needEvidence && UnpaidLookupStreak(history) >= unpaidLookupForceWrite
}

func unpaidLookupNudge() string {
	return "Kernel lookup budget spent. Stop object_get, object_list, and system_status. " +
		"Use read_code or observer_search to inspect code, and call write_code or write_file to implement the changes."
}

func filterToolCalls(calls []llm.ToolCall, keep func(llm.ToolCall) bool) (kept []llm.ToolCall, dropped []string) {
	for _, call := range calls {
		if keep(call) {
			kept = append(kept, call)
			continue
		}
		dropped = append(dropped, call.Name)
	}
	return kept, dropped
}

// filterRecoveredToolCalls drops lookup/status tools recovered from prose
// while a mutation pin is still unpaid. 7B (and other small local models)
// emit markdown JSON for zqk_object_get first; executing that "succeeds"
// the interceptor and never reaches write_code.
func filterRecoveredToolCalls(recovered bool, allowed []string, calls []llm.ToolCall) (kept []llm.ToolCall, dropped []string) {
	if !recovered || len(allowed) == 0 || len(calls) == 0 {
		return calls, nil
	}
	return filterToolCalls(calls, func(call llm.ToolCall) bool {
		return toolNameAllowed(call.Name, allowed)
	})
}

// filterUnpaidLookupCalls drops native lookup/status (and, when forceWrite,
// every non-write) after the unpaid streak. Phase 0 3.6 emits native
// tool_calls but ignores the named write pin and looks up until timeout.
func filterUnpaidLookupCalls(needEvidence bool, history []ToolCallRecord, calls []llm.ToolCall) (kept []llm.ToolCall, dropped []string) {
	if !needEvidence || len(calls) == 0 {
		return calls, nil
	}
	switch {
	case ShouldForceWriteOnly(needEvidence, history):
		return filterToolCalls(calls, func(call llm.ToolCall) bool {
			return IsMutationEvidenceTool(call.Name)
		})
	case ShouldNarrowToWriteMenu(needEvidence, history):
		return filterToolCalls(calls, func(call llm.ToolCall) bool {
			return !IsKernelLookupTool(call.Name)
		})
	default:
		return calls, nil
	}
}
