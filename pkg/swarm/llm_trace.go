package swarm

import (
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// llmTraceEnabled is on by default in a real process so operators can read
// the prompt the model was handed next to the raw completion. go test stays
// quiet unless LLM_TRACE=1 or a trace/diagnostics dir is set.
func llmTraceEnabled() bool {
	if zqkenv.LLMTrace().Get() == "1" || config.LLMTrace().OrDefault(false) {
		return true
	}
	if strings.TrimSpace(zqkenv.LLMTraceDir().Get()) != "" || config.PathsLLMTraceDir().OrDefault("") != "" || config.PathsDiagnosticsDir().OrDefault("") != "" {
		return true
	}
	return flag.Lookup("test.v") == nil
}

func llmTraceDir() string {
	if dir := strings.TrimSpace(zqkenv.LLMTraceDir().Get()); dir != "" {
		return dir
	}
	if dir := strings.TrimSpace(config.PathsLLMTraceDir().OrDefault("")); dir != "" {
		return dir
	}
	if dir := strings.TrimSpace(config.PathsDiagnosticsDir().OrDefault("")); dir != "" {
		return dir
	}
	root := strings.TrimSpace(zqkenv.ProjectRoot().Get())
	if root == "" {
		wd, err := fileutil.Getwd()
		if err != nil {
			return ""
		}
		root = wd
	}
	return filepath.Join(paths.LogsDirPath(root), paths.LLMLogsSubdir)
}

type llmTraceRecord struct {
	EngineID           string
	Step               int
	ToolChoice         string
	ToolChoiceFunction string
	ToolNames          []string
	Prompt             []llm.Message
	RawContent         string
	NativeToolCalls    []llm.ToolCall
	RecoveredMarkdown  bool
	ExecutedToolCalls  []llm.ToolCall
	DroppedToolNames   []string
}

func writeLLMTrace(rec llmTraceRecord) {
	if !llmTraceEnabled() {
		return
	}
	dir := llmTraceDir()
	if dir == "" {
		return
	}
	if err := fileutil.EnsureDir(dir); err != nil {
		return
	}
	id := sanitizeTraceID(rec.EngineID)
	logPath := filepath.Join(dir, fmt.Sprintf("llm-trace-%s.log", id))
	f, err := fileutil.OpenAppend(logPath)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(formatLLMTrace(rec))
}

func sanitizeTraceID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func formatLLMTrace(rec llmTraceRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n========== [Session: %s] LLM STEP %d ==========\n", rec.EngineID, rec.Step)
	fmt.Fprintf(&b, "tool_choice: %s\n", emptyDash(rec.ToolChoice))
	fmt.Fprintf(&b, "tool_choice_function: %s\n", emptyDash(rec.ToolChoiceFunction))
	fmt.Fprintf(&b, "tools: %s\n", strings.Join(rec.ToolNames, ", "))
	fmt.Fprintf(&b, "channel: %s\n", traceChannel(rec))
	if hint := toolChannelHint(rec.RawContent, len(rec.NativeToolCalls)); hint != "" {
		fmt.Fprintf(&b, "server_hint: %s\n", hint)
	}
	if len(rec.DroppedToolNames) > 0 {
		fmt.Fprintf(&b, "pin_filter_dropped: %s\n", strings.Join(rec.DroppedToolNames, ", "))
	}
	b.WriteString("\n----- PROMPT -----\n")
	for i, msg := range rec.Prompt {
		fmt.Fprintf(&b, "[%d %s]", i, msg.Role)
		if msg.Name != "" {
			fmt.Fprintf(&b, " name=%s", msg.Name)
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&b, " tool_call_id=%s", msg.ToolCallID)
		}
		b.WriteByte('\n')
		if strings.TrimSpace(msg.Content) != "" {
			b.WriteString(msg.Content)
			if !strings.HasSuffix(msg.Content, "\n") {
				b.WriteByte('\n')
			}
		}
		if len(msg.ToolCalls) > 0 {
			if raw, err := json.MarshalIndent(msg.ToolCalls, "", "  "); err == nil {
				b.WriteString(string(raw))
				b.WriteByte('\n')
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("----- RESPONSE -----\n")
	if strings.TrimSpace(rec.RawContent) != "" {
		b.WriteString(rec.RawContent)
		if !strings.HasSuffix(rec.RawContent, "\n") {
			b.WriteByte('\n')
		}
	} else {
		b.WriteString("(empty content)\n")
	}
	if len(rec.NativeToolCalls) > 0 {
		if raw, err := json.MarshalIndent(rec.NativeToolCalls, "", "  "); err == nil {
			b.WriteString("[native tool_calls]\n")
			b.WriteString(string(raw))
			b.WriteByte('\n')
		}
	}
	if rec.RecoveredMarkdown && len(rec.ExecutedToolCalls) > 0 {
		if raw, err := json.MarshalIndent(rec.ExecutedToolCalls, "", "  "); err == nil {
			b.WriteString("[recovered tool_calls]\n")
			b.WriteString(string(raw))
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	return b.String()
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func traceChannel(rec llmTraceRecord) string {
	switch {
	case len(rec.NativeToolCalls) > 0:
		return "native"
	case rec.RecoveredMarkdown && len(rec.ExecutedToolCalls) > 0:
		return "markdown-recovered"
	case rec.RecoveredMarkdown:
		return "markdown-recovered-dropped"
	case strings.TrimSpace(rec.RawContent) == "":
		return "empty"
	default:
		return "text"
	}
}

func toolChannelHint(content string, nativeCount int) string {
	if nativeCount > 0 {
		return ""
	}
	switch {
	case strings.Contains(content, "<tool_call>"):
		return "native tool_calls empty; content is Hermes <tool_call> XML. vLLM needs --enable-auto-tool-choice --tool-call-parser hermes; llama.cpp needs --jinja with a compatible tool-call chat template."
	case strings.Contains(content, "```"):
		return "native tool_calls empty; model wrote markdown JSON. The host recovered it. Local servers that advertise OpenAI tools still need a tool-call parser to populate the tool_calls field."
	default:
		return ""
	}
}

func toolDefinitionNames(tools []llm.ToolDefinition) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if n := strings.TrimSpace(t.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}
