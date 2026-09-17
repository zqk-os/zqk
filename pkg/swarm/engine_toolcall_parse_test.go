package swarm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestExtractHallucinatedToolCallsFromProseAndPrettyJSON(t *testing.T) {
	t.Parallel()

	content := fmt.Sprintf(`I will inspect the backlog first.

{
  "%s": "zqk_object_list",
  "arguments": {
    "%s": "backlog_item",
    "%s": "json"
  }
}`, objects.FieldKeyName, objects.FieldKeyKind, objects.FieldKeyFormat)

	calls, err := extractHallucinatedToolCalls(content)
	if err != nil {
		t.Fatalf("extractHallucinatedToolCalls returned error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "zqk_object_list" {
		t.Fatalf("tool call name = %q, want zqk_object_list", calls[0].Name)
	}
}

func TestRecoverMarkdownToolCalls_clearsContent(t *testing.T) {
	t.Parallel()

	resp := llm.StructuredCompletionResponse{
		Content: fmt.Sprintf("```json\n{\n  %q: %q,\n  \"arguments\": {%q: %q}\n}\n```",
			objects.FieldKeyName, "zqk_read_code", objects.FieldKeyPath, "pkg/swarm/engine.go"),
	}
	if !recoverMarkdownToolCalls(&resp) {
		t.Fatal("expected recovery")
	}
	if resp.Content != "" {
		t.Fatalf("content should be cleared, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "zqk_read_code" {
		t.Fatalf("tool calls = %#v", resp.ToolCalls)
	}
	native := llm.StructuredCompletionResponse{
		Content:   "I will call a tool",
		ToolCalls: []llm.ToolCall{{Name: "zqk_write_code"}},
	}
	if recoverMarkdownToolCalls(&native) {
		t.Fatal("must not overwrite native tool_calls")
	}
}

func TestExtractHermesToolCalls(t *testing.T) {
	t.Parallel()
	content := fmt.Sprintf(`<tool_call>
{"%s": "zqk_write_code", "arguments": {"%s": "pkg/x.go", "%s": "package x\n"}}
</tool_call>`, objects.FieldKeyName, objects.FieldKeyPath, objects.FieldKeyContent)
	calls, err := extractHallucinatedToolCalls(content)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "zqk_write_code" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestFilterRecoveredToolCalls_dropsLookupWhileWritePinned(t *testing.T) {
	t.Parallel()
	calls := []llm.ToolCall{{Name: "zqk_object_get"}, {Name: "zqk_write_code"}}
	kept, dropped := filterRecoveredToolCalls(true, []string{"zqk_write_code"}, calls)
	if len(kept) != 1 || kept[0].Name != "zqk_write_code" {
		t.Fatalf("kept = %#v", kept)
	}
	if len(dropped) != 1 || dropped[0] != "zqk_object_get" {
		t.Fatalf("dropped = %#v", dropped)
	}
	native, none := filterRecoveredToolCalls(false, []string{"zqk_write_code"}, calls)
	if len(native) != 2 || none != nil {
		t.Fatalf("native must stay intact: kept=%#v dropped=%#v", native, none)
	}
}

func TestUnpaidLookupStreak(t *testing.T) {
	t.Parallel()
	p := DefaultToolPrefix()
	if n := UnpaidLookupStreak(nil); n != 0 {
		t.Fatalf("empty = %d", n)
	}
	history := []ToolCallRecord{
		{Name: p + "object_get"},
		{Name: p + "read_code"},
		{Name: p + "write_code"},
		{Name: p + "object_list"},
		{Name: p + "read_file"},
	}
	if n := UnpaidLookupStreak(history); n != 2 {
		t.Fatalf("after write = %d, want 2 trailing lookups", n)
	}
	if n := UnpaidLookupStreak(history[:3]); n != 0 {
		t.Fatalf("ending on write = %d", n)
	}
}

func TestFilterUnpaidLookupCalls(t *testing.T) {
	t.Parallel()
	calls := []llm.ToolCall{{Name: "zqk_object_get"}, {Name: "zqk_write_code"}, {Name: "zqk_read_code"}}
	budgetGets := make([]ToolCallRecord, unpaidLookupBudget)
	for i := range budgetGets {
		budgetGets[i] = ToolCallRecord{Name: "zqk_object_get"}
	}
	kept, dropped := filterUnpaidLookupCalls(true, budgetGets, calls)
	if len(kept) != 2 || kept[0].Name != "zqk_write_code" || kept[1].Name != "zqk_read_code" {
		t.Fatalf("after budget kept=%#v", kept)
	}
	if len(dropped) != 1 || dropped[0] != "zqk_object_get" {
		t.Fatalf("after budget dropped=%#v", dropped)
	}
	forcedGets := make([]ToolCallRecord, unpaidLookupForceWrite)
	for i := range forcedGets {
		forcedGets[i] = ToolCallRecord{Name: "zqk_read_code"}
	}
	forced, forcedDrop := filterUnpaidLookupCalls(true, forcedGets, calls)
	if len(forced) != 1 || forced[0].Name != "zqk_write_code" {
		t.Fatalf("force-write kept=%#v", forced)
	}
	if len(forcedDrop) != 2 {
		t.Fatalf("force-write dropped=%#v", forcedDrop)
	}
	early, none := filterUnpaidLookupCalls(true, []ToolCallRecord{{Name: "zqk_object_get"}}, calls)
	if len(early) != 3 || none != nil {
		t.Fatalf("before budget must keep native lookups: kept=%#v dropped=%#v", early, none)
	}
}

func TestFormatAvailableToolsSuffix_namesOnly(t *testing.T) {
	t.Parallel()
	got := FormatAvailableToolsSuffix([]string{"zqk_read_code", "zqk_write_code"}, "zqk_")
	if !strings.Contains(got, "- zqk_read_code\n") {
		t.Fatalf("missing name: %s", got)
	}
	if strings.Contains(got, "Reads a source") {
		t.Fatalf("suffix must not repeat tool descriptions: %s", got)
	}
	if !strings.Contains(got, "native function-calling API") {
		t.Fatalf("missing convention: %s", got)
	}
}

func TestExtractHallucinatedToolCalls_bashCodeBlock(t *testing.T) {
	t.Parallel()
	content := "I will run the tests:\n```bash\ngo test ./pkg/swarm/...\n```"
	calls, err := extractHallucinatedToolCalls(content)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(calls) != 1 || !strings.HasSuffix(calls[0].Name, "execute_bash") {
		t.Fatalf("calls = %#v, want execute_bash", calls)
	}
	if !strings.Contains(calls[0].Arguments, "go test ./pkg/swarm/...") {
		t.Fatalf("args = %s", calls[0].Arguments)
	}
}
