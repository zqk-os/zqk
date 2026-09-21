package swarm

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRenderSystemPrompt(t *testing.T) {
	data := QwenSystemData{
		WorkerID:     "worker-123",
		Capabilities: []string{"coding", "review", "docs"},
	}

	result, err := RenderSystemPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Worker ID: worker-123") {
		t.Errorf("expected Worker ID in prompt, got: %s", result)
	}
	if !strings.Contains(result, "Capabilities: coding, review, docs, ") {
		t.Errorf("expected Capabilities in prompt, got: %s", result)
	}
}

func TestRenderSystemPrompt_ContainsTacticalGuidance(t *testing.T) {
	data := QwenSystemData{
		WorkerID:     "worker-tactical",
		Capabilities: []string{"coding"},
	}

	result, err := RenderSystemPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use the dynamic prefix for assertions (never hardcode brand)
	p := DefaultToolPrefix()

	requiredSections := []struct {
		name    string
		keyword string
	}{
		{"TDD mandate", "Test-Driven Development"},
		{"tool routing guide", "Tool Routing"},
		{"anti-patterns section", "Anti-Pattern"},
		{"observer search guidance", p + "observer_search"},
		{"read code guidance", p + "read_code"},
		{"write file guidance", p + "write_file"},
		{"object list guidance", p + "object_list"},
		{"object get guidance", p + "object_get"},
		{"bash restriction", p + "execute_bash"},
		{"circuit breaker warning", "circuit breaker"},
		{"no hourglass tool", "do NOT call agent_next"},
		{"repo layout", "Go kernel module"},
		{"native tool API", "native function-calling API"},
		{"no markdown fences", "markdown fences"},
	}

	for _, s := range requiredSections {
		if !strings.Contains(result, s.keyword) {
			t.Errorf("system prompt missing %s (keyword %q not found)", s.name, s.keyword)
		}
	}
}

func TestRenderSystemPrompt_UsesCustomToolPrefix(t *testing.T) {
	data := QwenSystemData{
		WorkerID:     "worker-custom",
		Capabilities: []string{"coding"},
		ToolPrefix:   "acme_",
	}

	result, err := RenderSystemPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "acme_read_code") {
		t.Error("system prompt did not use custom tool prefix for read_code")
	}
	if !strings.Contains(result, "acme_execute_bash") {
		t.Error("system prompt did not use custom tool prefix for execute_bash")
	}
}

func TestRenderTaskPrompt(t *testing.T) {
	data := QwenTaskData{
		TaskName:        "Test Task",
		TaskDescription: "Do something testable",
		Context:         "Some context here",
	}

	result, err := RenderTaskPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Assigned Task: Test Task") {
		t.Errorf("expected Task Name in prompt, got: %s", result)
	}
	if !strings.Contains(result, "Description: Do something testable") {
		t.Errorf("expected Description in prompt, got: %s", result)
	}
	if !strings.Contains(result, "Context: Some context here") {
		t.Errorf("expected Context in prompt, got: %s", result)
	}
}

func TestRenderTaskPrompt_ContainsExampleWorkflow(t *testing.T) {
	data := QwenTaskData{
		TaskName:        "Fix Bug",
		TaskDescription: "Fix a bug in the system",
		Context:         "Bug context",
	}

	result, err := RenderTaskPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Example Workflow") {
		t.Error("task prompt missing Example Workflow section")
	}
	if !strings.Contains(result, "go test") {
		t.Error("task prompt example workflow missing go test step")
	}
}

func TestRenderTaskPrompt_UsesCustomToolPrefix(t *testing.T) {
	data := QwenTaskData{
		TaskName:        "Fix Bug",
		TaskDescription: "Fix it",
		Context:         "ctx",
		ToolPrefix:      "mybrand_",
	}

	result, err := RenderTaskPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "mybrand_read_code") {
		t.Error("task prompt did not use custom tool prefix for read_code")
	}
	if !strings.Contains(result, "mybrand_write_file") {
		t.Error("task prompt did not use custom tool prefix for write_file")
	}
}

func TestShouldSoftBlockBashTool(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		wantBlock bool
	}{
		{
			name:      "go test should be allowed",
			arguments: `{"command": "go test ./pkg/swarm/..."}`,
			wantBlock: false,
		},
		{
			name:      "go test -run should be allowed",
			arguments: `{"command": "go test -run TestFoo ./pkg/..."}`,
			wantBlock: false,
		},
		{
			name:      "go build should be allowed",
			arguments: `{"command": "go build ./pkg/reqharness/..."}`,
			wantBlock: false,
		},
		{
			name:      "agent validate should be blocked",
			arguments: `{"command": "./bin/zqk agent validate"}`,
			wantBlock: true,
		},
		{
			name:      "cat file should be blocked",
			arguments: `{"command": "cat pkg/swarm/engine.go"}`,
			wantBlock: true,
		},
		{
			name:      "make build should be allowed",
			arguments: `{"command": "make build-all"}`,
			wantBlock: false,
		},
		{
			name:      "empty arguments should be blocked",
			arguments: `{}`,
			wantBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldSoftBlockBashTool(tt.arguments)
			if got != tt.wantBlock {
				t.Errorf("ShouldSoftBlockBashTool(%q) = %v, want %v", tt.arguments, got, tt.wantBlock)
			}
		})
	}
}

func TestBashSoftBlockGuidanceMessage_UsesBrandPrefix(t *testing.T) {
	msg := BashSoftBlockGuidanceMessage()
	p := DefaultToolPrefix()

	if !strings.Contains(msg, p+"execute_bash") {
		t.Errorf("guidance message missing branded execute_bash, got: %s", msg)
	}
	if !strings.Contains(msg, p+"read_code") {
		t.Errorf("guidance message missing branded read_code, got: %s", msg)
	}
	if !strings.Contains(msg, p+"object_list") {
		t.Errorf("guidance message missing branded object_list, got: %s", msg)
	}
}

func TestExecuteBashToolName(t *testing.T) {
	name := ExecuteBashToolName()
	p := DefaultToolPrefix()
	expected := p + "execute_bash"
	if name != expected {
		t.Errorf("ExecuteBashToolName() = %q, want %q", name, expected)
	}
}

func TestDetectConsecutiveDuplicate(t *testing.T) {
	tests := []struct {
		name    string
		history []ToolCallRecord
		current ToolCallRecord
		want    bool
	}{
		{
			name:    "empty history - no duplicate",
			history: nil,
			current: ToolCallRecord{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "foo.go"})},
			want:    false,
		},
		{
			name:    "single different call - no duplicate",
			history: []ToolCallRecord{{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "foo.go"})}},
			current: ToolCallRecord{Name: "write_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "bar.go"})},
			want:    false,
		},
		{
			name:    "single identical call - no duplicate yet",
			history: []ToolCallRecord{{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "foo.go"})}},
			current: ToolCallRecord{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "foo.go"})},
			want:    false,
		},
		{
			name: "two identical calls then same again - duplicate detected",
			history: []ToolCallRecord{
				{Name: "zqk_log", Arguments: `{"message":"stuck"}`},
				{Name: "zqk_log", Arguments: `{"message":"stuck"}`},
			},
			current: ToolCallRecord{Name: "zqk_log", Arguments: `{"message":"stuck"}`},
			want:    true,
		},
		{
			name: "two identical then different - no duplicate",
			history: []ToolCallRecord{
				{Name: "zqk_log", Arguments: `{"message":"stuck"}`},
				{Name: "zqk_log", Arguments: `{"message":"stuck"}`},
			},
			current: ToolCallRecord{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "foo.go"})},
			want:    false,
		},
		{
			name: "same name but different arguments - no duplicate",
			history: []ToolCallRecord{
				{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})},
				{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "b.go"})},
			},
			current: ToolCallRecord{Name: "read_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "c.go"})},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectConsecutiveDuplicate(tt.history, tt.current)
			if got != tt.want {
				t.Errorf("DetectConsecutiveDuplicate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuplicateCallGuidanceMessage(t *testing.T) {
	msg := DuplicateCallGuidanceMessage("zqk_log")
	if !strings.Contains(msg, "zqk_log") {
		t.Error("guidance message should mention the repeated tool name")
	}
	if !strings.Contains(msg, "different") {
		t.Error("guidance message should tell the model to try something different")
	}
}

func TestDetectContextGatheringLoop(t *testing.T) {
	p := DefaultToolPrefix()
	tests := []struct {
		name    string
		history []ToolCallRecord
		want    bool
	}{
		{
			name:    "empty history - no loop",
			history: nil,
			want:    false,
		},
		{
			name: "mixed action and context tools - no loop",
			history: []ToolCallRecord{
				{Name: p + "read_code"},
				{Name: p + "system_status"},
				{Name: p + "write_file"},
				{Name: p + "execute_bash"},
			},
			want: false,
		},
		{
			name: "15 consecutive context-only tools - loop detected",
			history: []ToolCallRecord{
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "system_status"},
				{Name: p + "get_current_priority_plan"},
				{Name: p + "get_current_backlog_item"},
				{Name: p + "system_status"},
				{Name: p + "get_current_priority_plan"},
			},
			want: true,
		},
		{
			name: "context tools broken by action - no loop",
			history: []ToolCallRecord{
				{Name: p + "system_status"},
				{Name: p + "get_current_priority_plan"},
				{Name: p + "write_file"},
				{Name: p + "system_status"},
				{Name: p + "get_current_priority_plan"},
			},
			want: false,
		},
		{
			name: "15 consecutive read_code - loop detected",
			history: []ToolCallRecord{
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_code"},
				{Name: p + "read_file"},
				{Name: p + "read_code"},
				{Name: p + "object_get"},
				{Name: p + "read_code"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.history {
				tt.history[i].Turn = i
			}
			got := DetectContextGatheringLoop(tt.history)
			if got != tt.want {
				t.Errorf("DetectContextGatheringLoop() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectRepeatingCycle(t *testing.T) {
	tests := []struct {
		name    string
		history []ToolCallRecord
		want    bool
	}{
		{
			name:    "empty history",
			history: nil,
			want:    false,
		},
		{
			name: "period-5 cycle repeating 3 times (name-only) - detected",
			history: []ToolCallRecord{
				// First cycle
				{Name: "read_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})},
				{Name: "write_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go", objects.FieldKeyContent: "v1"})},
				{Name: "system_status", Arguments: `{}`},
				{Name: "get_plan", Arguments: `{}`},
				{Name: "get_backlog", Arguments: `{}`},
				// Second cycle (same names, different write content)
				{Name: "read_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})},
				{Name: "write_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go", objects.FieldKeyContent: "v2"})},
				{Name: "system_status", Arguments: `{}`},
				{Name: "get_plan", Arguments: `{}`},
				{Name: "get_backlog", Arguments: `{}`},
				// Third cycle
				{Name: "read_file", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go"})},
				{Name: "write_code", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyPath: "a.go", objects.FieldKeyContent: "v3"})},
				{Name: "system_status", Arguments: `{}`},
				{Name: "get_plan", Arguments: `{}`},
				{Name: "get_backlog", Arguments: `{}`},
			},
			want: true,
		},
		{
			name: "same tool with new arguments each call - progress, not a cycle",
			history: []ToolCallRecord{
				{Name: "object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "ATK-1"})},
				{Name: "object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "BLI-2"})},
				{Name: "object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "REQ-3"})},
				{Name: "object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "CRIT-4"})},
			},
			want: false,
		},
		{
			name: "identical name and arguments repeated - stuck after 2 cycles",
			history: []ToolCallRecord{
				{Name: "object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "ATK-1"})},
				{Name: "system_status", Arguments: `{}`},
				{Name: "object_get", Arguments: toolArgsJSON(map[string]string{objects.FieldKeyID: "ATK-1"})},
				{Name: "system_status", Arguments: `{}`},
			},
			want: true,
		},
		{
			name: "period-3 cycle repeating 2 times - detected",
			history: []ToolCallRecord{
				{Name: "a"}, {Name: "b"}, {Name: "c"},
				{Name: "a"}, {Name: "b"}, {Name: "c"},
			},
			want: true,
		},
		{
			name: "no cycle - all different",
			history: []ToolCallRecord{
				{Name: "a"}, {Name: "b"}, {Name: "c"},
				{Name: "d"}, {Name: "e"}, {Name: "f"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.history {
				tt.history[i].Turn = i
			}
			got, _ := DetectRepeatingCycle(tt.history)
			if got != tt.want {
				t.Errorf("DetectRepeatingCycle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsKernelLookupTool(t *testing.T) {
	p := DefaultToolPrefix()
	if !IsKernelLookupTool(p + "object_get") {
		t.Fatal("object_get")
	}
	if IsKernelLookupTool(p + "read_code") {
		t.Fatal("read_code stays until force-write, not the first menu shrink")
	}
	if IsKernelLookupTool(p + "write_code") {
		t.Fatal("write_code")
	}
}

func TestIsContextGatheringTool(t *testing.T) {
	p := DefaultToolPrefix()
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{p + "system_status", p + "system_status", true},
		{p + "get_current_priority_plan", p + "get_current_priority_plan", true},
		{p + "get_current_backlog_item", p + "get_current_backlog_item", true},
		{p + "object_count", p + "object_count", true},
		{p + "write_file (action)", p + "write_file", false},
		{p + "execute_bash (action)", p + "execute_bash", false},
		{p + "read_code (lookup)", p + "read_code", true},
		{p + "read_file (lookup)", p + "read_file", true},
		{p + "observer_search (lookup)", p + "observer_search", true},
		{p + "stable object_get suffix", "zqk-stable_object_get", true},
		{"agent_next (action)", "agent_next", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsContextGatheringTool(tt.toolName)
			if got != tt.want {
				t.Errorf("IsContextGatheringTool(%q) = %v, want %v", tt.toolName, got, tt.want)
			}
		})
	}
}
