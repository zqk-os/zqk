package swarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/lanceman/zqk/pkg/brand"
)

// QwenSystemPromptTemplate defines the core persona and behavior for the Qwen worker.
// This prompt is deliberately detailed to steer small local LLMs (7B-36B) toward correct
// tool-calling patterns and away from the two dominant failure modes observed in production:
// 1. Bash-loop addiction (calling execute_bash repeatedly instead of specific MCP tools)
// 2. Tool-name hallucination (inventing tool names not in the registered set)
const QwenSystemPromptTemplate = `You are a Qwen LLM acting as a worker node in the ZQK Swarm Orchestrator.

## Core Principles
1. Follow Test-Driven Development (TDD): write tests BEFORE implementation.
2. Use specific MCP tools instead of {{.ToolPrefix}}execute_bash whenever possible.
3. Never call the same tool with the same arguments more than once.
4. Follow the principles of Traceability and Object-First design.

## Tool Routing Guide
Use the MOST SPECIFIC tool available for each operation:
- To READ code files: use {{.ToolPrefix}}read_code (NOT {{.ToolPrefix}}execute_bash with cat)
- To WRITE/EDIT code files: use {{.ToolPrefix}}write_file (NOT {{.ToolPrefix}}execute_bash with echo)
- To LIST kernel objects: use {{.ToolPrefix}}object_list with kind and filter params
- To GET object details: use {{.ToolPrefix}}object_get with the object ID
- To CREATE objects: use {{.ToolPrefix}}new_object then {{.ToolPrefix}}object_create
- To RUN tests: use {{.ToolPrefix}}execute_bash ONLY for "go test" commands
- To BUILD the project: use {{.ToolPrefix}}execute_bash ONLY for "make" commands
- To ADVANCE task lifecycle: call agent_next with your task_id

## Anti-Patterns (will trigger the circuit breaker and abort your task)
- Calling {{.ToolPrefix}}execute_bash repeatedly with the same command
- Calling tools that do not exist in the Available Tools list below
- Using {{.ToolPrefix}}execute_bash for operations that have a dedicated MCP tool
- Calling {{.ToolPrefix}}execute_bash to read files (use {{.ToolPrefix}}read_code instead)
- Calling {{.ToolPrefix}}execute_bash to list objects (use {{.ToolPrefix}}object_list instead)

Worker ID: {{.WorkerID}}
Capabilities: {{range .Capabilities}}{{.}}, {{end}}
`

// QwenTaskPromptTemplate is used to assign a specific task to the Qwen worker.
// It includes a concrete example workflow to anchor the model toward productive
// tool-calling sequences rather than open-ended exploration.
const QwenTaskPromptTemplate = `Assigned Task: {{.TaskName}}
Description: {{.TaskDescription}}
Context: {{.Context}}

## Example Workflow (follow this pattern)
1. Use {{.ToolPrefix}}read_code to understand the relevant source files
2. Use {{.ToolPrefix}}write_file to create a test file (TDD: tests first)
3. Use {{.ToolPrefix}}execute_bash with "go test ./pkg/..." to run the test (expect failure)
4. Use {{.ToolPrefix}}write_file to implement the fix in the source file
5. Use {{.ToolPrefix}}execute_bash with "go test ./pkg/..." to verify the fix (expect pass)
6. When finished, output your summary as plain text with NO tool calls

Please execute the task following the workflow above.
`

// bashToolAllowedPrefixes lists command prefixes that are allowed through
// the soft-block filter. Commands not matching any prefix will trigger
// guidance feedback to steer the LLM toward specific MCP tools.
var bashToolAllowedPrefixes = []string{
	"go test",
	"make",
	"git ",
}

// ExecuteBashToolSuffix is the tool name suffix (without brand prefix) for the
// bash execution tool. Used with DefaultToolPrefix() to construct the full name.
const ExecuteBashToolSuffix = "execute_bash"

// DefaultToolPrefix returns the brand-aware tool name prefix (e.g. "zqk_").
// This must be used instead of hardcoding any brand-specific prefix.
func DefaultToolPrefix() string {
	return brand.NamespacePrefix() + "_"
}

// ExecuteBashToolName returns the full branded tool name for execute_bash.
func ExecuteBashToolName() string {
	return DefaultToolPrefix() + ExecuteBashToolSuffix
}

// ShouldSoftBlockBashTool returns true if an execute_bash call should be
// soft-blocked (guidance injected instead of execution). It parses the tool
// arguments to extract the command and checks against the allowed prefixes.
// Commands like "go test" and "make" are allowed; everything else is blocked
// to steer the model toward using specific MCP tools.
func ShouldSoftBlockBashTool(arguments string) bool {
	var parsed struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil || parsed.Command == "" {
		return true // Block unparseable or empty commands
	}

	cmd := strings.TrimSpace(parsed.Command)
	for _, prefix := range bashToolAllowedPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return false
		}
	}
	return true
}

// BashSoftBlockGuidanceMessage returns the guidance message for soft-blocked
// execute_bash calls. It uses the brand-aware tool prefix so tool names are
// never hardcoded.
func BashSoftBlockGuidanceMessage() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: Do not use %sexecute_bash for this operation. "+
			"Use the specific MCP tool instead: "+
			"%sread_code to read files, "+
			"%sobject_list to list objects, "+
			"%sobject_get to get object details, "+
			"%swrite_file to write files. "+
			"Only use %sexecute_bash for 'go test' or 'make' commands.",
		p, p, p, p, p, p,
	)
}

// DetectConsecutiveDuplicate returns true if the current tool call would be the
// 3rd consecutive identical call (same Name and Arguments). This fires BEFORE
// the circuit breaker (which trips at 3 completed identical calls) to give the
// LLM a chance to self-correct by injecting guidance instead of executing.
func DetectConsecutiveDuplicate(history []ToolCallRecord, current ToolCallRecord) bool {
	n := len(history)
	if n < 2 {
		return false
	}
	prev1 := history[n-1]
	prev2 := history[n-2]
	return prev1.Name == current.Name && prev1.Arguments == current.Arguments &&
		prev2.Name == current.Name && prev2.Arguments == current.Arguments
}

// DuplicateCallGuidanceMessage returns the guidance injected when a consecutive
// duplicate tool call is detected. It tells the model what it repeated and
// instructs it to try a different approach.
func DuplicateCallGuidanceMessage(toolName string) string {
	return fmt.Sprintf(
		"WARNING: You have called '%s' with identical arguments 3 times in a row. "+
			"This will trigger the circuit breaker and abort your task. "+
			"You MUST try a different tool or different arguments. "+
			"Review your task objective and choose a different approach.",
		toolName,
	)
}

// contextGatheringSuffixes lists tool name suffixes (after the brand prefix) that
// are read-only status/context queries. These don't make progress on the task
// and should not dominate the tool call budget.
var contextGatheringSuffixes = []string{
	"system_status",
	"get_current_priority_plan",
	"get_current_backlog_item",
	"object_count",
	"object_list",
	"object_get",
}

// IsContextGatheringTool returns true if the given tool name is a read-only
// context/status querying tool. These are useful for orientation but should
// not be called repeatedly without taking action.
func IsContextGatheringTool(toolName string) bool {
	p := DefaultToolPrefix()
	for _, suffix := range contextGatheringSuffixes {
		if toolName == p+suffix {
			return true
		}
	}
	return false
}

// contextGatheringBudget is the maximum number of consecutive context-gathering
// tool calls allowed before guidance is injected. This prevents the "status
// check loop" anti-pattern where the LLM re-reads state endlessly.
const contextGatheringBudget = 5

// DetectContextGatheringLoop returns true if the last N tool calls in history
// are ALL context-gathering tools with no action tools in between. This catches
// the "read state in a loop" anti-pattern observed in production.
func DetectContextGatheringLoop(history []ToolCallRecord) bool {
	n := len(history)
	if n < contextGatheringBudget {
		return false
	}

	// Check if the last `contextGatheringBudget` calls are all context tools
	for i := n - contextGatheringBudget; i < n; i++ {
		if !IsContextGatheringTool(history[i].Name) {
			return false
		}
	}
	return true
}

// ContextGatheringLoopGuidance returns the guidance message when a context-gathering
// loop is detected.
func ContextGatheringLoopGuidance() string {
	return "WARNING: You have spent too many steps querying system status without taking action. " +
		"You already have the context you need. STOP reading status and START working on the task. " +
		"Use action tools: read_code to read source files, write_file to write code, " +
		"execute_bash to run tests. Make progress NOW."
}

// DetectRepeatingCycle detects multi-tool repeating sequences by NAME ONLY
// (ignoring arguments). This catches cycles like:
//
//	read_file → write_code → system_status → read_file → write_code → system_status
//
// The existing circuit breaker requires exact name+args match and only checks
// periods 1-4. This function checks periods 2-8 and uses name-only matching,
// requiring only 2 repetitions (not 3) since name-only cycles are a strong
// signal of stuck behavior.
func DetectRepeatingCycle(history []ToolCallRecord) (bool, int) {
	n := len(history)

	for period := 2; period <= 8; period++ {
		minLen := 2 * period // Need 2 full cycles
		if n < minLen {
			continue
		}

		// Compare the last two chunks of `period` length by name only
		match := true
		for i := 0; i < period; i++ {
			if history[n-period+i].Name != history[n-2*period+i].Name {
				match = false
				break
			}
		}
		if match {
			return true, period
		}
	}
	return false, 0
}

// RepeatingCycleGuidance returns the guidance message when a multi-tool
// repeating cycle is detected.
func RepeatingCycleGuidance(period int) string {
	return fmt.Sprintf(
		"CRITICAL: You are stuck in a repeating cycle of %d tools. "+
			"You keep calling the same sequence of tools over and over. "+
			"This will NOT make progress. STOP and try a completely different approach. "+
			"If you cannot make progress, output a text summary and make NO more tool calls.",
		period,
	)
}

// QwenSystemData holds template data for the system prompt.
type QwenSystemData struct {
	WorkerID     string
	Capabilities []string
	ToolPrefix   string // Brand-aware tool prefix (e.g. "zqk_"). Set via DefaultToolPrefix().
}

// QwenTaskData holds template data for the task prompt.
type QwenTaskData struct {
	TaskName        string
	TaskDescription string
	Context         string
	ToolPrefix      string // Brand-aware tool prefix (e.g. "zqk_"). Set via DefaultToolPrefix().
}

// RenderSystemPrompt renders the Qwen system prompt.
// If ToolPrefix is empty, it defaults to the brand-aware prefix.
func RenderSystemPrompt(data QwenSystemData) (string, error) {
	if data.ToolPrefix == "" {
		data.ToolPrefix = DefaultToolPrefix()
	}
	tmpl, err := template.New("qwen_system").Parse(QwenSystemPromptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderTaskPrompt renders the Qwen task prompt.
// If ToolPrefix is empty, it defaults to the brand-aware prefix.
func RenderTaskPrompt(data QwenTaskData) (string, error) {
	if data.ToolPrefix == "" {
		data.ToolPrefix = DefaultToolPrefix()
	}
	tmpl, err := template.New("qwen_task").Parse(QwenTaskPromptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
