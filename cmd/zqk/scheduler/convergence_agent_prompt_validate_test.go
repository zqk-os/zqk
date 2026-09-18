package scheduler

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestBuildAppleScriptIDEPaste_embeddedScript(t *testing.T) {
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "")
	t.Setenv(zqkenv.IDEPasteApp().Name(), "")
	s, err := buildAppleScriptIDEPaste()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `keystroke "y" using command down`) {
		t.Fatal("missing ⌘Y (focus agent chat before paste)")
	}
	if !strings.Contains(s, `tell application "Cursor" to activate`) {
		t.Fatal("default paste target must be quoted Cursor (unquoted names osascript -2740; IDE is not a process)")
	}
	if strings.Contains(s, `tell application "IDE"`) || strings.Contains(s, `whose name is "IDE"`) {
		t.Fatal("must not target a process named IDE")
	}
	if strings.Contains(s, `keystroke "e" using {command down, shift down}`) {
		t.Fatal("embedded default must not send ⌘⇧E unless overridden via ZQK_CURSOR_PASTE_PREFIX_STEPS")
	}
	if strings.Contains(s, "key code 53") {
		t.Fatal("embedded default must not send Escape unless overridden")
	}
	if strings.Contains(s, `keystroke "e" using {command down, option down}`) {
		t.Fatal("embedded default must not send ⌥⌘E unless overridden")
	}
	if strings.Contains(s, `keystroke "l" using command down`) {
		t.Fatal("embedded default must not send ⌘L (toggles sidebar; can hide agent chat)")
	}
	if !strings.Contains(s, `keystroke "v" using command down`) {
		t.Fatal("missing cmd+v paste")
	}
	if !strings.Contains(s, "key code 36") {
		t.Fatal("missing return key code")
	}
	if strings.Contains(s, "keystroke \"i\" using command down") {
		t.Fatal("unexpected command+i keystroke")
	}
}

func TestLoadIDEPasteAutomation_customPrefixSteps(t *testing.T) {
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "escape, option_cmd_e , option_cmd_e")
	a, err := loadIDEPasteAutomation()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a.appleScript, `keystroke "l" using command down`) {
		t.Fatal("custom sequence should omit cmd+l when not requested")
	}
	if strings.Count(a.appleScript, `keystroke "e" using {command down, option down}`) != 2 {
		t.Fatalf("want 2 option_cmd_e, script: %s", truncateForTestLog(a.appleScript, 400))
	}
	if !strings.Contains(a.keystrokesSummary, "Escape(53)") || strings.Contains(a.keystrokesSummary, "Cmd+L") {
		t.Fatalf("keystrokesSummary: %q", a.keystrokesSummary)
	}
}

func TestLoadIDEPasteAutomation_invalidToken(t *testing.T) {
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "escape,cmd_shift_l")
	_, err := loadIDEPasteAutomation()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadIDEPasteAutomation_builtinAlias(t *testing.T) {
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "builtin")
	a, err := loadIDEPasteAutomation()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a.appleScript, `keystroke "l" using command down`) {
		t.Fatal("builtin embedded script should not include cmd+l")
	}
}

func TestBuildAppleScriptIDEPaste_appNameOverride(t *testing.T) {
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "")
	t.Setenv(zqkenv.IDEPasteApp().Name(), "Cursor Nightly")
	s, err := buildAppleScriptIDEPaste()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `tell application "Cursor Nightly" to activate`) {
		t.Fatalf("override app name missing, script: %s", truncateForTestLog(s, 400))
	}
}

func truncateForTestLog(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func TestRunConvergenceAgentPromptPipeline_requiresProjectRoot(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	_, _, err := runConvergenceAgentPromptPipeline(cmd, "", nil, "CVS-x", "", "", nil, nil, false, nil, false, "", false, false, false, "", nil, nil, nil)
	if !errors.Is(err, errAgentPromptProjectRootMissing) {
		t.Fatalf("want errAgentPromptProjectRootMissing, got %v", err)
	}
}

func TestAgentPromptValidateIngestConstraints(t *testing.T) {
	t.Parallel()
	err := agentPromptValidateIngestConstraints(&agentPromptPipelineInput{
		sessionID:      "CVS-1",
		skipSessionCtx: false,
		pasteIDE:       false,
		copyClip:       false,
	})
	if err != nil {
		t.Fatalf("valid input: %v", err)
	}

	err = agentPromptValidateIngestConstraints(&agentPromptPipelineInput{
		sessionID:      "",
		skipSessionCtx: false,
	})
	if !errors.Is(err, errAgentPromptMissingSession) {
		t.Fatalf("missing session: want errAgentPromptMissingSession, got %v", err)
	}

	err = agentPromptValidateIngestConstraints(&agentPromptPipelineInput{
		sessionID:      "CVS-1",
		skipSessionCtx: true,
	})
	if !errors.Is(err, errAgentPromptSkipSessionContext) {
		t.Fatalf("skip context: want errAgentPromptSkipSessionContext, got %v", err)
	}

	if runtime.GOOS != "darwin" {
		err = agentPromptValidateIngestConstraints(&agentPromptPipelineInput{
			sessionID: "CVS-1", pasteIDE: true,
		})
		if !errors.Is(err, errAgentPromptPasteRequiresDarwin) {
			t.Fatalf("paste non-darwin: %v", err)
		}
		err = agentPromptValidateIngestConstraints(&agentPromptPipelineInput{
			sessionID: "CVS-1", copyClip: true,
		})
		if !errors.Is(err, errAgentPromptCopyRequiresDarwin) {
			t.Fatalf("copy non-darwin: %v", err)
		}
	}
}

func TestAgentPromptIngestBlockedReasonCode(t *testing.T) {
	t.Parallel()
	if c := agentPromptIngestBlockedReasonCode(errAgentPromptMissingSession); c != "missing_session_id" {
		t.Fatalf("got %q", c)
	}
}

func TestNormalizeAgentPromptAttentionMode(t *testing.T) {
	t.Parallel()
	got, err := normalizeAgentPromptAttentionMode("")
	if err != nil || got != "" {
		t.Fatalf("empty: got %q err %v", got, err)
	}
	got, err = normalizeAgentPromptAttentionMode("test")
	if err != nil || got != agentprompt.AttentionTestNonDirective {
		t.Fatalf("test: got %q err %v", got, err)
	}
	got, err = normalizeAgentPromptAttentionMode("interrupt_plumbing")
	if err != nil || got != agentprompt.AttentionInterruptPlumbing {
		t.Fatalf("interrupt: got %q err %v", got, err)
	}
	_, err = normalizeAgentPromptAttentionMode("bogus")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFormatAgentMarkdown_AttentionMode(t *testing.T) {
	t.Parallel()
	md := formatAgentMarkdown("CVS-x", nil, nil, nil, agentprompt.AttentionTestNonDirective, nil, nil, "", nil, "")
	if !strings.Contains(md, "<!-- zqk:agent_prompt") || !strings.Contains(md, "test / non-directive") {
		t.Fatalf("expected preamble: %s", md)
	}
}

func TestAgentPromptMetricsBucketing(t *testing.T) {
	t.Parallel()
	var b agentPromptMetricsBucketing
	pctx := &pipeline.Context{Outcome: map[string]any{agentprompt.KeyAttentionMode: agentprompt.AttentionTestNonDirective}}
	m := b.Buckets(pctx, pipelineKindConvergenceAgentPrompt, pipeline.StageIngest)
	if m[agentprompt.KeyAttentionMode] != agentprompt.AttentionTestNonDirective {
		t.Fatalf("attention_mode: got %#v", m)
	}
	pctx2 := &pipeline.Context{Outcome: map[string]any{agentprompt.KeyAttentionMode: agentprompt.MetricAttentionDefault}}
	m2 := b.Buckets(pctx2, pipelineKindConvergenceAgentPrompt, pipeline.StageCommit)
	if m2[agentprompt.KeyAttentionMode] != agentprompt.MetricAttentionDefault {
		t.Fatalf("default: got %#v", m2)
	}
}

func TestAppendAgentPromptRunJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outcome := map[string]any{
		outcomeAgentPromptSessionID:     "CVS-test",
		outcomeAgentPromptMarkdownBytes: 42,
		outcomeAgentPromptDeliverMode:   agentPromptDeliverNone,
		agentprompt.KeyAttentionMode:    agentprompt.AttentionTestNonDirective,
		outcomeAgentPromptComplete:      true,
	}
	appendAgentPromptRunJSONL(root, outcome)
	path := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, agentPromptRunsJSONLFile)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("json: %v", err)
	}
	if row[schedpkg.KeyTimestamp] == nil || row[schedpkg.KeyTimestamp] == "" {
		t.Fatalf("missing timestamp: %#v", row[schedpkg.KeyTimestamp])
	}
	if row[agentPromptRunJSONKeyEventType] != agentPromptRunEventType {
		t.Fatalf("event_type: got %v", row[agentPromptRunJSONKeyEventType])
	}
	if row[agentPromptRunJSONKeyPipelineKind] != pipelineKindConvergenceAgentPrompt {
		t.Fatalf("pipeline_kind: got %v", row[agentPromptRunJSONKeyPipelineKind])
	}
	if row[outcomeAgentPromptSessionID] != "CVS-test" {
		t.Fatalf("session: got %v", row[outcomeAgentPromptSessionID])
	}
}
