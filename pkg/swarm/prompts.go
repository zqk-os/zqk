package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Prompt template kernel object IDs.
const (
	PromptTemplateSwarmWorkerSystemCoding   = "PROMPT-SWARM-WORKER-SYSTEM-CODING"
	PromptTemplateSwarmWorkerSystemDocsEval = "PROMPT-SWARM-WORKER-SYSTEM-DOCS-EVAL"
	PromptTemplateSwarmWorkerTask           = "PROMPT-SWARM-WORKER-TASK"
)

// DefaultSwarmWorkerCodingSystemPromptTemplate defines the fallback persona and behavior for coding tasks.
// Small local LLMs and remote LLMs are steered toward correct tool-calling patterns and away from
// bash-loop addiction and tool-name hallucination.
const DefaultSwarmWorkerCodingSystemPromptTemplate = `You are an LLM acting as a worker node in the ZQK Swarm Orchestrator.

## How you call tools
The host already registered tools on this request. To invoke a tool you MUST use the native function-calling API (tool_calls field). Leave assistant text empty when calling a tool.
Forbidden in assistant text: markdown fences (triple-backtick / json fences), XML tool_call tags, or a JSON object with a name field plus arguments. Those are not tool calls.
When the task is done, write a short prose summary and make zero tool calls.

## Core Principles
1. Follow Test-Driven Development (TDD): write tests BEFORE implementation.
2. Use specific MCP tools instead of {{.ToolPrefix}}execute_bash whenever possible.
3. Never call the same tool with the same arguments more than once.
4. Follow the principles of Traceability and Object-First design.

## This repository
This is a Go kernel module (cmd/, pkg/, scripts/, docs/). Never invent src/main.go, helloworld.go, or Python files. Kernel ids (WFL-/ATK-/BLI-/PRI-…) are object_get targets, not files. Do not call mcp_list_tools or mcp_get_tool_schema.

## Tool Routing Guide
Use the MOST SPECIFIC tool available for each operation — and ONLY tools that appear in Available Tools:
- To LOCATE symbols / real files: use {{.ToolPrefix}}observer_search with name= and/or path= (pkg/ or cmd/). Do this BEFORE inventing a file.
- To READ code files: use {{.ToolPrefix}}read_code (NOT {{.ToolPrefix}}execute_bash with cat)
- To WRITE/EDIT code files: use {{.ToolPrefix}}write_file / {{.ToolPrefix}}write_code (NOT {{.ToolPrefix}}execute_bash with echo)
- To LIST kernel objects: use {{.ToolPrefix}}object_list with a required kind (e.g. agent_task) and limit<=20. Never list all kinds. Do not inspect or touch backlog items: your scope is strictly your assigned task.
- To GET object details: use {{.ToolPrefix}}object_get with the object ID
- To CREATE/UPDATE objects: use only create/update tools that exist in Available Tools (do not invent {{.ToolPrefix}}new_object)
- To RUN tests / vet: use {{.ToolPrefix}}execute_bash ONLY for "go test", "go vet", "go fmt", or "go mod tidy|download"
- To BUILD the project: use {{.ToolPrefix}}execute_bash ONLY for "make" commands
- To ADVANCE task lifecycle: do NOT call agent_next. Write files, then stop. The seat-worker promotes.

## Anti-Patterns (will trigger the circuit breaker and abort your task)
- Calling {{.ToolPrefix}}execute_bash repeatedly with the same command
- Calling tools that do not exist in the Available Tools list below
- Using {{.ToolPrefix}}execute_bash for operations that have a dedicated MCP tool
- Calling {{.ToolPrefix}}execute_bash to read files (use {{.ToolPrefix}}read_code instead)
- Calling {{.ToolPrefix}}execute_bash to list objects (use {{.ToolPrefix}}object_list instead)
- Inventing tool names from memory or docs that are not in Available Tools
- Writing a tool call as markdown, a fenced JSON block, or a name-plus-arguments JSON object in your message

Worker ID: {{.WorkerID}}
Capabilities: {{range .Capabilities}}{{.}}, {{end}}
Work class: coding
`

// DefaultSwarmWorkerDocsEvalSystemPromptTemplate is for docs / evaluation ATKs.
// It must not push TDD, go test loops, or source edits.
const DefaultSwarmWorkerDocsEvalSystemPromptTemplate = `You are an LLM acting as a docs/evaluation worker in the Swarm Orchestrator.

## How you call tools
The host already registered tools on this request. To invoke a tool you MUST use the native function-calling API (tool_calls field). Leave assistant text empty when calling a tool.
Forbidden in assistant text: markdown fences (triple-backtick / json fences), XML tool_call tags, or a JSON object with a name field plus arguments. Those are not tool calls.
When the task is done, write a short prose summary and make zero tool calls.

## Core Principles
1. Obey FORBID / SUCCESS_GATE in the task description exactly.
2. Use specific MCP tools instead of {{.ToolPrefix}}execute_bash whenever possible.
3. Never call the same tool with the same arguments more than once.
4. Cite real paths under documentation and the codebase; do not invent modules.

## This assignment class (docs_eval)
- Read rubrics and evaluation documents named in the task.
- Write findings JSONL and run logs under the paths named in the task.
- Do NOT edit application source unless the task explicitly requires it.
- Do NOT mint process objects. Do NOT call agent_next — summarize and stop; the seat-worker promotes.

## Tool Routing Guide
- To READ docs or code: use {{.ToolPrefix}}read_file or {{.ToolPrefix}}read_code
- To WRITE findings / run_log / diagrams: use {{.ToolPrefix}}write_file
- To GET task/object details: use {{.ToolPrefix}}object_get

## Anti-Patterns
- Rewriting application source to "prove" architecture findings
- Calling {{.ToolPrefix}}execute_bash for cat/ls when read_file exists
- Claiming SUCCESS_GATE without a successful write_file of findings JSONL

Worker ID: {{.WorkerID}}
Capabilities: {{range .Capabilities}}{{.}}, {{end}}
Work class: docs_eval
`

// DefaultSwarmWorkerTaskPromptTemplate is used to assign a specific task to the worker.
// It includes a concrete example workflow to anchor the model toward productive
// tool-calling sequences rather than open-ended exploration.
const DefaultSwarmWorkerTaskPromptTemplate = `Assigned Task: {{.TaskName}}
Description: {{.TaskDescription}}
Context: {{.Context}}

## Example Workflow (follow this pattern)
1. Use {{.ToolPrefix}}observer_search to locate the real symbol or pkg/ path, then {{.ToolPrefix}}read_code that file
2. Use {{.ToolPrefix}}write_file to create a test file (TDD: tests first)
3. Use {{.ToolPrefix}}execute_bash with "go test ./pkg/..." to run the test (expect failure)
4. Use {{.ToolPrefix}}write_file to implement the fix in the source file
5. Use {{.ToolPrefix}}execute_bash with "go test ./pkg/..." to verify the fix (expect pass)
6. When finished, output your summary as plain text with NO tool calls and NO JSON

Please execute the task following the workflow above.
`

// bashToolAllowedPrefixes lists command prefixes that are allowed through
// the soft-block filter. Commands not matching any prefix will trigger
// guidance feedback to steer the LLM toward specific MCP tools.
var bashToolAllowedPrefixes = []string{
	"go test",
	"go build",
	"go vet",
	"go fmt",
	"go mod tidy",
	"go mod download",
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
			"%sobserver_search to locate symbols, "+
			"%sread_code to read files, "+
			"%sobject_list to list objects, "+
			"%sobject_get to get object details, "+
			"%swrite_file to write files. "+
			"Only use %sexecute_bash for 'go test', 'go build', 'go vet', 'go fmt', 'go mod tidy|download', or 'make' commands.",
		p, p, p, p, p, p, p,
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
var kernelLookupSuffixes = []string{
	"system_status",
	"get_current_priority_plan",
	"get_current_backlog_item",
	"object_count",
	"object_list",
	"object_get",
	"mcp_list_tools",
	"mcp_get_tool_schema",
}

var contextGatheringSuffixes = append([]string{
	"read_code",
	"read_file",
	"observer_search",
}, kernelLookupSuffixes...)

func toolNameHasSuffix(toolName string, suffixes []string) bool {
	n := strings.ToLower(strings.TrimSpace(toolName))
	if n == "" {
		return false
	}
	for _, suffix := range suffixes {
		if n == suffix || strings.HasSuffix(n, "_"+suffix) {
			return true
		}
	}
	return false
}

// IsKernelLookupTool is object_get/list/status (and MCP schema probes).
// After the unpaid lookup budget these leave the wire; read_code stays
// until the force-write threshold.
func IsKernelLookupTool(toolName string) bool {
	return toolNameHasSuffix(toolName, kernelLookupSuffixes)
}

// IsContextGatheringTool returns true if the given tool name is a read-only
// context/status querying tool. These are useful for orientation but should
// not be called repeatedly without taking action. read_code/read_file count:
// mixed lookup+read never paid a write pin and reset the old detector.
func IsContextGatheringTool(toolName string) bool {
	return toolNameHasSuffix(toolName, contextGatheringSuffixes)
}

// contextGatheringBudget is the maximum number of consecutive context-gathering
// tool turns allowed before guidance is injected. This prevents the "status
// check loop" anti-pattern where the LLM re-reads state endlessly while allowing
// legitimate orientation and code reading before writing changes.
const contextGatheringBudget = 10

// DetectContextGatheringLoop returns true if the last N tool calls in history
// are ALL context-gathering tools with no action tools in between. This catches
// the "read state in a loop" anti-pattern observed in production.
func groupTurns(history []ToolCallRecord) [][]ToolCallRecord {
	var turns [][]ToolCallRecord
	var currentTurn []ToolCallRecord
	var lastTurn int = -1
	for _, rec := range history {
		if rec.Turn != lastTurn {
			if len(currentTurn) > 0 {
				turns = append(turns, currentTurn)
			}
			currentTurn = nil
			lastTurn = rec.Turn
		}
		currentTurn = append(currentTurn, rec)
	}
	if len(currentTurn) > 0 {
		turns = append(turns, currentTurn)
	}
	return turns
}

// DetectContextGatheringLoop detects if the model is stuck in a loop of reading context.
func DetectContextGatheringLoop(history []ToolCallRecord) bool {
	turns := groupTurns(history)
	n := len(turns)
	if n < contextGatheringBudget {
		return false
	}

	for i := n - contextGatheringBudget; i < n; i++ {
		turn := turns[i]
		for _, call := range turn {
			if !IsContextGatheringTool(call.Name) {
				return false
			}
		}
	}
	return true
}

// ContextGatheringLoopGuidance returns the guidance message when a context-gathering
// loop is detected.
func ContextGatheringLoopGuidance() string {
	return "WARNING: You have spent too many steps reading status or source without taking action. " +
		"You already have the context you need. STOP object_get, object_list, and read_code loops. " +
		"Call write_file or write_code on a path named in the task. Make progress NOW."
}

// DetectRepeatingCycle detects multi-tool repeating sequences over periods 2-8.
//
// Repeats with identical arguments are stuck behavior and trip after 2 cycles.
// Name-only repeats are weaker evidence: a capable model legitimately calls the
// same tool with new arguments while working through a task (e.g. reading a
// different object each pass), so those require 3 cycles before tripping.
func DetectRepeatingCycle(history []ToolCallRecord) (bool, int) {
	turns := groupTurns(history)
	n := len(turns)

	for period := 1; period <= 8; period++ {
		if n >= 2*period && turnChunksEqual(turns, n-2*period, n-period, period, true) {
			return true, period
		}
		if period >= 2 && n >= 3*period &&
			turnChunksEqual(turns, n-2*period, n-period, period, false) &&
			turnChunksEqual(turns, n-3*period, n-2*period, period, false) {
			return true, period
		}
	}
	return false, 0
}

func turnChunksEqual(turns [][]ToolCallRecord, aStart, bStart, period int, matchArgs bool) bool {
	for i := 0; i < period; i++ {
		a, b := turns[aStart+i], turns[bStart+i]
		if len(a) != len(b) {
			return false
		}
		for j := 0; j < len(a); j++ {
			if a[j].Name != b[j].Name {
				return false
			}
			if matchArgs && a[j].Arguments != b[j].Arguments {
				return false
			}
		}
	}
	return true
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

// SwarmWorkerSystemData holds template data for the system prompt.
type SwarmWorkerSystemData struct {
	WorkerID     string
	Capabilities []string
	ToolPrefix   string // Brand-aware tool prefix (e.g. "zqk_"). Set via DefaultToolPrefix().
	WorkClass    string // coding | docs_eval — selects system template.
}

// SwarmWorkerTaskData holds template data for the task prompt.
type SwarmWorkerTaskData struct {
	TaskName        string
	TaskDescription string
	Context         string
	ToolPrefix      string // Brand-aware tool prefix (e.g. "zqk_"). Set via DefaultToolPrefix().
}

// FormatAvailableToolsSuffix lists tool *names* only. Full schemas travel on
// the native tools API; repeating descriptions here puts 7B models into
// document mode and they start wrapping calls in markdown.
func FormatAvailableToolsSuffix(toolNames []string, toolPrefix string) string {
	if toolPrefix == "" {
		toolPrefix = DefaultToolPrefix()
	}
	var b strings.Builder
	b.WriteString("\n\n## Available Tools\n")
	b.WriteString("Use these exact names via the native function-calling API (underscores, not spaces; e.g. ")
	b.WriteString(toolPrefix)
	b.WriteString("object_get, not \"")
	b.WriteString(strings.TrimSuffix(toolPrefix, "_"))
	b.WriteString(" object get\"). Do not copy this list into your message.\n")
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteByte('\n')
	}
	b.WriteString("\nRULES:\n")
	b.WriteString("1. Use ONLY the native function-calling API. Never write tool calls as JSON, markdown fences, or XML in your response.\n")
	b.WriteString("2. Wait for tool results before proceeding.\n")
	b.WriteString("3. Use ONLY tools listed under Available Tools above. Do not invent names (including agent_next / new_object / object_create unless they appear in that list).\n")
	b.WriteString("4. Prefer specific MCP tools over ")
	b.WriteString(toolPrefix)
	b.WriteString("execute_bash. When finished, output a plain-text summary and make NO further tool calls.\n")
	return b.String()
}

// MarkdownToolCallConventionGuidance is injected after we recover a prose/JSON
// tool call so the next step uses the native API.
func MarkdownToolCallConventionGuidance() string {
	return "CONVENTION: You wrote a tool call in assistant text (markdown/JSON). " +
		"The host executed it this once. Next step: empty text + native function-calling API. " +
		"Do not wrap the next call in a json fence or a name-plus-arguments object."
}

// ResolvePromptTemplate retrieves a prompt template body by ID from kernel storage,
// returning fallback if the storage provider is nil, the object is missing,
// or the template body is empty.
func ResolvePromptTemplate(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, templateID string, fallback string) string {
	if sp == nil || templateID == "" {
		return fallback
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSecurityContext(authcred.DefaultSwarmWorkerAccount, []string{"swarm_worker"}, []string{"*"})
	}
	obj, err := sp.Read(ctx, secCtx, templateID)
	if err != nil || obj == nil {
		return fallback
	}
	if body, ok := obj[objects.FieldKeyPromptBody].(string); ok && strings.TrimSpace(body) != "" {
		return strings.TrimSpace(body)
	}
	return fallback
}

// RenderSystemPromptWithStorage renders the swarm worker system prompt, resolving the template
// dynamically from the kernel prompt_template object in storage if available, falling back to default.
func RenderSystemPromptWithStorage(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, data SwarmWorkerSystemData) (string, error) {
	if data.ToolPrefix == "" {
		data.ToolPrefix = DefaultToolPrefix()
	}
	var templateID, fallback string
	if strings.EqualFold(strings.TrimSpace(data.WorkClass), "docs_eval") {
		templateID = PromptTemplateSwarmWorkerSystemDocsEval
		fallback = DefaultSwarmWorkerDocsEvalSystemPromptTemplate
	} else {
		templateID = PromptTemplateSwarmWorkerSystemCoding
		fallback = DefaultSwarmWorkerCodingSystemPromptTemplate
	}
	tmplSrc := ResolvePromptTemplate(ctx, sp, secCtx, templateID, fallback)
	tmpl, err := template.New("swarm_worker_system").Parse(tmplSrc)
	if err != nil && tmplSrc != fallback {
		// Fallback to built-in template if custom template has syntax error
		tmpl, err = template.New("swarm_worker_system").Parse(fallback)
	}
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderTaskPromptWithStorage renders the swarm worker task prompt, resolving the template
// dynamically from the kernel prompt_template object in storage if available, falling back to default.
func RenderTaskPromptWithStorage(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, data SwarmWorkerTaskData) (string, error) {
	if data.ToolPrefix == "" {
		data.ToolPrefix = DefaultToolPrefix()
	}
	tmplSrc := ResolvePromptTemplate(ctx, sp, secCtx, PromptTemplateSwarmWorkerTask, DefaultSwarmWorkerTaskPromptTemplate)
	tmpl, err := template.New("swarm_worker_task").Parse(tmplSrc)
	if err != nil && tmplSrc != DefaultSwarmWorkerTaskPromptTemplate {
		// Fallback to built-in template if custom template has syntax error
		tmpl, err = template.New("swarm_worker_task").Parse(DefaultSwarmWorkerTaskPromptTemplate)
	}
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderSystemPrompt renders the swarm worker system prompt using default fallback templates.
// If ToolPrefix is empty, it defaults to the brand-aware prefix.
func RenderSystemPrompt(data SwarmWorkerSystemData) (string, error) {
	return RenderSystemPromptWithStorage(context.Background(), nil, nil, data)
}

// RenderTaskPrompt renders the swarm worker task prompt using default fallback templates.
// If ToolPrefix is empty, it defaults to the brand-aware prefix.
func RenderTaskPrompt(data SwarmWorkerTaskData) (string, error) {
	return RenderTaskPromptWithStorage(context.Background(), nil, nil, data)
}
