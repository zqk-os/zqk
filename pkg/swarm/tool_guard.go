package swarm

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/objects"
)

// hourglassOnlyTools are IDE/seat-worker verbs that qwen invents as MCP calls.
// The seat-worker runs agent next after write evidence; swarm MCP does not.
var hourglassOnlyTools = map[string]struct{}{
	"agent_next":                    {},
	"zqk_agent_next":                {},
	"chat_send":                     {},
	"zqk_chat_send":                 {},
	"get_current_backlog_item":      {},
	"zqk_get_current_backlog_item":  {},
	"get_current_priority_plan":     {},
	"zqk_get_current_priority_plan": {},
	objects.FieldKeyNextAction:      {},
	"zqk_next_action":               {},
}

// inventedMutationTools are write-adjacent names qwen invents instead of
// write_code / write_file. Steering here keeps them off the "tool not found"
// loop that then parks for missing mutation evidence.
var inventedMutationTools = map[string]struct{}{
	"replace_code":     {},
	"zqk_replace_code": {},
	"commit":           {},
	"zqk_commit":       {},
}

var inventedObjectListKinds = map[string]struct{}{
	objects.FieldKeySourceFile: {},
	"code_file":                {},
	"file":                     {},
	objects.FieldKeySource:     {},
	objects.FieldKeyCode:       {},
}

// guardSwarmToolCall steers invented or lifecycle-illegal calls into guidance
// so they do not become coordinator ERROR or "tool not found" loops.
// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001 — remove when: seats stop inventing
// hourglass verbs and status= updates after qwen prompt + MCP surface converge.
func guardSwarmToolCall(call llm.ToolCall) (result string, handled bool) {
	name := strings.TrimSpace(call.Name)
	if isHourglassOnlyTool(name) {
		return HourglassOnlyToolGuidance(), true
	}
	if isInventedMutationTool(name) {
		return InventedMutationToolGuidance(), true
	}
	switch {
	case isObjectUpdateTool(name) && objectUpdateSetsStatus(call.Arguments):
		return StatusFieldUpdateGuidance(), true
	case isObjectGetTool(name) && placeholderObjectID(call.Arguments):
		return PlaceholderObjectIDGuidance(), true
	case isObjectListTool(name) && unscopedObjectList(call.Arguments):
		return UnscopedObjectListGuidance(), true
	case isObjectListTool(name) && inventedObjectListKind(call.Arguments):
		return InventedObjectListKindGuidance(), true
	case isObjectListTool(name) && oversizedObjectList(call.Arguments):
		return OversizedObjectListGuidance(), true
	case name == "zqk_mcp_call_tool" && mcpCallInnerIsNotATool(call.Arguments):
		return MCPCallInnerNotAToolGuidance(mcpCallInnerName(call.Arguments)), true
	case isMCPCatalogTool(name):
		return MCPCatalogToolGuidance(), true
	case isRepoFileTool(name) && inventedRepoPath(toolPathArg(call.Arguments)):
		return InventedRepoPathGuidance(toolPathArg(call.Arguments)), true
	}
	return "", false
}

func isRepoFileTool(name string) bool {
	return toolSuffixIs(name, "read_code") || toolSuffixIs(name, "read_file") ||
		toolSuffixIs(name, "write_code") || toolSuffixIs(name, "write_file")
}

func toolPathArg(arguments string) string {
	args := parseToolArgs(arguments)
	if args == nil {
		return ""
	}
	return strings.TrimSpace(stringifyArg(args[objects.FieldKeyPath]))
}

func inventedRepoPath(p string) bool {
	p = filepath.ToSlash(strings.TrimSpace(p))
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	base := strings.ToLower(filepath.Base(p))
	if strings.Contains(lower, "yourrepo") || strings.Contains(lower, "helloworld") {
		return true
	}
	if base == "main.go" && (p == "main.go" || strings.HasPrefix(lower, "src/")) {
		return true
	}
	if strings.HasSuffix(lower, ".py") && !strings.HasPrefix(lower, "scripts/") {
		return true
	}
	if kernelObjectIDPath(filepath.Base(p)) {
		return true
	}
	return false
}

func kernelObjectIDPath(base string) bool {
	base = strings.ToUpper(strings.TrimSpace(base))
	for _, pref := range kernelIDPathPrefixes {
		if strings.HasPrefix(base, pref) {
			return true
		}
	}
	return false
}

// kernelIDPathPrefixes are object ids qwen opens as files (WFL-SUBAGENT-DISPATCH).
var kernelIDPathPrefixes = []string{
	"WFL-", "ATK-", "BLI-", "PRI-", "CRIT-", "POL-", "GLS-", "AFE-",
	"SCH-", "CAP-", "REQ-", "TDE-", "CVS-", "ACC-", "PER-",
}

func InventedRepoPathGuidance(p string) string {
	pref := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: %q is not a path in this zqk Go module. Real trees are cmd/, pkg/, scripts/, docs/. "+
			"Kernel ids (WFL-/ATK-/BLI-/PRI-…) are %sobject_get targets, not files. "+
			"Use %sread_code on an existing file (start with cmd/zqk or pkg/).",
		p, pref, pref,
	)
}

func isHourglassOnlyTool(name string) bool {
	return namedToolInSet(name, hourglassOnlyTools)
}

func isInventedMutationTool(name string) bool {
	if namedToolInSet(name, inventedMutationTools) {
		return true
	}
	return toolSuffixIs(name, "replace_code") || toolSuffixIs(name, "commit")
}

func namedToolInSet(name string, set map[string]struct{}) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if _, ok := set[n]; ok {
		return true
	}
	stripped := strings.TrimPrefix(n, DefaultToolPrefix())
	_, ok := set[stripped]
	return ok
}

func isObjectUpdateTool(name string) bool {
	return toolSuffixIs(name, "object_update")
}

func isObjectGetTool(name string) bool {
	return toolSuffixIs(name, "object_get")
}

func isObjectListTool(name string) bool {
	return toolSuffixIs(name, "object_list")
}

func isMCPCatalogTool(name string) bool {
	return toolSuffixIs(name, "mcp_list_tools") || toolSuffixIs(name, "mcp_get_tool_schema") ||
		toolSuffixIs(name, "list_tools") || toolSuffixIs(name, "get_tool_schema")
}

func MCPCatalogToolGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: Do not browse the MCP catalog. Available Tools is already in your prompt. "+
			"Call %sread_code / %swrite_code on cmd/ or pkg/, or %sobject_get on the ATK/BLI id from the prompt.",
		p, p, p,
	)
}

func toolSuffixIs(name, suffix string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == suffix || strings.HasSuffix(n, "_"+suffix)
}

func InventedMutationToolGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: There is no replace_code or commit MCP tool. Edit a real repo path with %swrite_code or %swrite_file (full file content). "+
			"The seat-worker commits after write evidence. Do not invent git/commit tools.",
		p, p,
	)
}

func HourglassOnlyToolGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: That verb is not a swarm MCP tool. The seat-worker advances the ATK after write evidence. "+
			"Call %swrite_code or %swrite_file on a real repo path now. Do not call agent_next, chat_send, or get_current_* again.",
		p, p,
	)
}

func StatusFieldUpdateGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: Do not set status via %sobject_update. Lifecycle moves use object promote/demote/park. "+
			"The seat-worker does that after write evidence. Use %swrite_code or %swrite_file now.",
		p, p, p,
	)
}

func PlaceholderObjectIDGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: object_get needs a real object id from the task (ATK-/BLI-/PRI-…), not a placeholder like <ID>. "+
			"Read the assigned ATK id from your prompt, or skip get and call %sread_code on a real repo path.",
		p,
	)
}

func OversizedObjectListGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: %sobject_list needs limit<=%d (and a kind). A dump of the kernel is not task evidence. "+
			"Use %sobject_get on the ATK/BLI id from your prompt, or %swrite_code on a real cmd/ or pkg/ path.",
		p, maxSwarmObjectListLimit, p, p,
	)
}

func UnscopedObjectListGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: %sobject_list requires kind (backlog_item, agent_task, …) and a small limit. "+
			"Do not list every kind. Use %sobject_get on the ATK/BLI id from your prompt, or %sread_code on cmd/ or pkg/.",
		p, p, p,
	)
}

func InventedObjectListKindGuidance() string {
	p := DefaultToolPrefix()
	return fmt.Sprintf(
		"GUIDANCE: source files are not kernel object kinds. Use %sread_code / %sread_file on a real repo path. "+
			"%sobject_list is only for registered kinds (backlog_item, agent_task, …).",
		p, p, p,
	)
}

func MCPCallInnerNotAToolGuidance(inner string) string {
	return fmt.Sprintf(
		"GUIDANCE: %q is not an MCP tool name. Do not wrap shell commands or invented names in mcp_call_tool. "+
			"Use an exact name from Available Tools.",
		inner,
	)
}

func objectUpdateSetsStatus(arguments string) bool {
	args := parseToolArgs(arguments)
	if args == nil {
		return false
	}
	if _, ok := args[objects.FieldKeyStatus]; ok {
		return true
	}
	if fields, ok := args["fields"].([]any); ok {
		for _, f := range fields {
			s, _ := f.(string)
			if statusFieldAssign(s) {
				return true
			}
		}
	}
	if upd, ok := args["updates"].(map[string]any); ok {
		if _, ok := upd[objects.FieldKeyStatus]; ok {
			return true
		}
	}
	return false
}

func statusFieldAssign(s string) bool {
	s = strings.TrimSpace(s)
	key, _, ok := strings.Cut(s, "=")
	return ok && strings.EqualFold(strings.TrimSpace(key), objects.FieldKeyStatus)
}

func placeholderObjectID(arguments string) bool {
	args := parseToolArgs(arguments)
	if args == nil {
		return false
	}
	id := strings.TrimSpace(stringifyArg(args[objects.FieldKeyID]))
	if id == "" {
		id = strings.TrimSpace(stringifyArg(args["object_id"]))
	}
	if id == "" {
		return false
	}
	if strings.Contains(id, "<") || strings.Contains(id, ">") {
		return true
	}
	switch strings.ToUpper(id) {
	case "ID", "OBJECT_ID", "OBJECT-ID":
		return true
	}
	return false
}

func unscopedObjectList(arguments string) bool {
	args := parseToolArgs(arguments)
	if args == nil {
		return true
	}
	return strings.TrimSpace(stringifyArg(args[objects.FieldKeyKind])) == ""
}

// objectListArgLimit is the MCP list page-size argument (not an object FieldKey).
const objectListArgLimit = "limit"

// maxSwarmObjectListLimit is the largest page a seat-worker may request.
// Prompt already says <=20; without this guard, kind=backlog_item dumped 679KB
// into the 7B context and the run parked for missing writes.
const maxSwarmObjectListLimit = 20

func oversizedObjectList(arguments string) bool {
	args := parseToolArgs(arguments)
	if args == nil {
		return true
	}
	n, ok := objectListLimit(args)
	return !ok || n < 1 || n > maxSwarmObjectListLimit
}

func objectListLimit(args map[string]any) (int, bool) {
	if args == nil {
		return 0, false
	}
	v, ok := args[objectListArgLimit]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	default:
		return 0, false
	}
}

func inventedObjectListKind(arguments string) bool {
	args := parseToolArgs(arguments)
	if args == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(stringifyArg(args[objects.FieldKeyKind])))
	_, ok := inventedObjectListKinds[kind]
	return ok
}

func mcpCallInnerName(arguments string) string {
	args := parseToolArgs(arguments)
	if args == nil {
		return ""
	}
	return strings.TrimSpace(stringifyArg(args["tool_name"]))
}

func mcpCallInnerIsNotATool(arguments string) bool {
	inner := mcpCallInnerName(arguments)
	return inner == "" || strings.ContainsAny(inner, " \t")
}

func parseToolArgs(arguments string) map[string]any {
	if strings.TrimSpace(arguments) == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil
	}
	return args
}

func stringifyArg(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}
