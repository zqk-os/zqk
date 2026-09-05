package swarm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestGuardSwarmToolCall_hourglassAndStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, tool, args string
		want             bool
		contains         string
	}{
		{"agent_next", "agent_next", `{}`, true, "not a swarm MCP tool"},
		{"prefixed agent_next", "zqk_agent_next", `{}`, true, "not a swarm MCP tool"},
		{"chat_send", "zqk_chat_send", `{}`, true, "not a swarm MCP tool"},
		{"status field", "zqk_object_update", `{"id":"ATK-1","status":"complete"}`, true, "promote"},
		{"status assign", "zqk_object_update", `{"id":"ATK-1","fields":["status=complete"]}`, true, "promote"},
		{"plain update", "zqk_object_update", `{"id":"ATK-1","title":"x"}`, false, ""},
		{"placeholder id", "zqk_object_get", `{"id":"<ID>"}`, true, "placeholder"},
		{"real id", "zqk_object_get", `{"id":"REDACTED"}`, false, ""},
		{"unscoped list", "zqk_object_list", `{}`, true, "requires kind"},
		{"priority plan verb", "zqk_get_current_priority_plan", `{}`, true, "not a swarm MCP tool"},
		{"invented kind", "zqk_object_list", fmt.Sprintf("{%q:%q}", objects.FieldKeyKind, "source_file"), true, "not kernel object kinds"},
		{"scoped list", "zqk_object_list", fmt.Sprintf("{%q:%q,%q:%d}", objects.FieldKeyKind, "agent_task", objectListArgLimit, maxSwarmObjectListLimit), false, ""},
		{"list without limit", "zqk_object_list", fmt.Sprintf("{%q:%q}", objects.FieldKeyKind, "agent_task"), true, "limit<="},
		{"list over limit", "zqk_object_list", fmt.Sprintf("{%q:%q,%q:%d}", objects.FieldKeyKind, "backlog_item", objectListArgLimit, 500), true, "limit<="},
		{"shell inner", "zqk_mcp_call_tool", `{"tool_name":"go mod tidy","arguments":{}}`, true, "not an MCP tool"},
		{"real write", "zqk_write_file", fmt.Sprintf("{%q:%q,%q:%q}", objects.FieldKeyPath, "pkg/swarm/tool_guard.go", objects.FieldKeyContent, "pkg"), false, ""},
		{"tutorial main", "zqk_read_code", fmt.Sprintf("{%q:%q}", objects.FieldKeyPath, "src/main.go"), true, "not a path"},
		{"vendor py", "zqk_write_file", fmt.Sprintf("{%q:%q,%q:%q}", objects.FieldKeyPath, "antigravity-1.py", objects.FieldKeyContent, "print"), true, "not a path"},
		{"workflow id as file", "zqk_read_file", fmt.Sprintf("{%q:%q}", objects.FieldKeyPath, "WFL-SUBAGENT-DISPATCH"), true, "not a path"},
		{"catalog schema", "zqk_mcp_get_tool_schema", `{}`, true, "Do not browse the MCP catalog"},
		{"catalog list", "zqk_mcp_list_tools", `{}`, true, "Do not browse the MCP catalog"},
		{"next_action", "zqk_next_action", `{}`, true, "not a swarm MCP tool"},
		{"replace_code", "zqk_replace_code", `{}`, true, "no replace_code"},
		{"stable replace", "zqk-stable_replace_code", `{}`, true, "no replace_code"},
		{"commit", "zqk_commit", `{}`, true, "no replace_code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, handled := guardSwarmToolCall(llm.ToolCall{Name: tc.tool, Arguments: tc.args})
			if handled != tc.want {
				t.Fatalf("handled=%v want %v result=%q", handled, tc.want, got)
			}
			if tc.want && !strings.Contains(got, tc.contains) {
				t.Fatalf("result %q missing %q", got, tc.contains)
			}
		})
	}
}

func TestShouldSoftBlockBashTool_allowsVetAndMod(t *testing.T) {
	t.Parallel()
	allow := []string{
		`{"command":"go vet ./cmd/zqk/agent"}`,
		`{"command":"go fmt cmd/zqk/agent/orchestrate.go"}`,
		`{"command":"go mod tidy"}`,
		`{"command":"go mod download github.com/fatih/color"}`,
		`{"command":"go test ./pkg/swarm -timeout 30s"}`,
	}
	for _, args := range allow {
		if ShouldSoftBlockBashTool(args) {
			t.Fatalf("blocked compile-adjacent command: %s", args)
		}
	}
	if !ShouldSoftBlockBashTool(`{"command":"go get github.com/fatih/color"}`) {
		t.Fatal("go get must stay blocked")
	}
}
