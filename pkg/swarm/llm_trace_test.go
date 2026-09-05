package swarm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/llm"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestFormatLLMTrace_includesPromptAndResponse(t *testing.T) {
	t.Parallel()
	got := formatLLMTrace(llmTraceRecord{
		EngineID:           "eng-1",
		Step:               0,
		ToolChoice:         llm.ToolChoiceRequired,
		ToolChoiceFunction: "zqk_write_code",
		ToolNames:          []string{"zqk_write_code", "zqk_object_get"},
		Prompt: []llm.Message{
			{Role: "system", Content: "SYSTEM PROMPT BODY"},
			{Role: "user", Content: "USER TASK BODY"},
		},
		RawContent:        "```json\n{\"name\":\"zqk_object_get\"}\n```",
		RecoveredMarkdown: true,
		DroppedToolNames:  []string{"zqk_object_get"},
	})
	for _, want := range []string{
		"SYSTEM PROMPT BODY",
		"USER TASK BODY",
		"----- PROMPT -----",
		"----- RESPONSE -----",
		"tool_choice_function: zqk_write_code",
		"pin_filter_dropped: zqk_object_get",
		"server_hint:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace missing %q:\n%s", want, got)
		}
	}
}

func TestWriteLLMTrace_optInWritesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(zqkenv.LLMTrace(), "1")
	t.Setenv(zqkenv.LLMTraceDir(), dir)
	writeLLMTrace(llmTraceRecord{
		EngineID:   "trace-test",
		Step:       2,
		Prompt:     []llm.Message{{Role: "user", Content: "handed this prompt"}},
		RawContent: "model said this",
	})
	b, err := fileutil.ReadFile(filepath.Join(dir, "llm-trace-trace-test.log"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(b), "handed this prompt") || !strings.Contains(string(b), "model said this") {
		t.Fatalf("file missing prompt/response:\n%s", b)
	}
}

func TestToolChannelHint_hermes(t *testing.T) {
	t.Parallel()
	hint := toolChannelHint("<tool_call>{\"name\":\"x\"}</tool_call>", 0)
	if !strings.Contains(hint, "hermes") {
		t.Fatalf("hint = %q", hint)
	}
	if toolChannelHint("plain", 1) != "" {
		t.Fatal("native calls should not hint")
	}
}
