package swarm

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
)

func TestCodeDraftToolNames_excludesLookup(t *testing.T) {
	t.Parallel()
	names := CodeDraftToolNames(DefaultToolPrefix())
	joined := strings.Join(names, " ")
	if !strings.Contains(joined, "write_code") || !strings.Contains(joined, "read_code") {
		t.Fatalf("draft menu missing write/read: %v", names)
	}
	for _, n := range names {
		if strings.Contains(n, "object_get") || strings.Contains(n, "object_list") {
			t.Fatalf("draft menu must not include lookup tools: %v", names)
		}
	}
}

func TestFilterToolsByAllowlist(t *testing.T) {
	t.Parallel()
	tools := []llm.ToolDefinition{
		{Name: "zqk_object_get"},
		{Name: "zqk_write_code"},
		{Name: "zqk_mcp_list_tools"},
	}
	got := filterToolsByAllowlist(tools, CodeDraftToolNames("zqk_"))
	if len(got) != 1 || got[0].Name != "zqk_write_code" {
		t.Fatalf("filtered = %#v", got)
	}
}

func TestApplyCodeDraftHarness(t *testing.T) {
	t.Parallel()
	e := NewEngine(&mockClient{}, &mockExecutor{}, 0, "harness")
	if ApplyCodeDraftHarness(e, "qwen3.6:latest") {
		t.Fatal("doer model must not get the draft harness")
	}
	if !ApplyCodeDraftHarness(e, "qwen2.5-coder:7b") {
		t.Fatal("coder-7b must get the draft harness")
	}
	if len(e.toolAllowlist) == 0 || e.extraSystem == "" {
		t.Fatal("harness must set allowlist and extra system")
	}
}

func TestCodeDraftSystemGuidance_forbidsLookup(t *testing.T) {
	t.Parallel()
	g := CodeDraftSystemGuidance()
	if !strings.Contains(g, "write_code") {
		t.Fatal("guidance must name write_code")
	}
	if !strings.Contains(g, "object_get") {
		t.Fatal("guidance must forbid object_get")
	}
}
