package eval_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/eval"
)

// Criterion 1: Static Floor Invariant - Parsing transcript JSONL and swarm traces into LLMInteraction.
func TestLLMScrutinyParser_ParseTranscriptJSONL(t *testing.T) {
	logContent := `
{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"Please check the status"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-11-07T12:00:00Z","input_tokens":1200,"output_tokens":80,"tool_calls":[{"name":"run_command","args":{"CommandLine":"zqk status"}}]}
{"step_index":2,"source":"MODEL","type":"GENERIC","content":"Command exited with code 0.\nAll systems operational."}
{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-11-07T12:01:00Z","input_tokens":1500,"output_tokens":90,"tool_calls":[{"name":"run_command","args":{"CommandLine":"zqk nonexistent-command"}}]}
{"step_index":4,"source":"MODEL","type":"GENERIC","content":"Command exited with code 1.\nError: unknown command"}
`
	parser := eval.NewLLMScrutinyParser()
	interactions, err := parser.ParseTranscriptJSONL(strings.NewReader(logContent), "test_transcript.jsonl")
	require.NoError(t, err)
	require.Len(t, interactions, 2)

	assert.Equal(t, "test_transcript.jsonl-step-1", interactions[0].ID)
	assert.Equal(t, eval.OutcomeSuccess, interactions[0].Outcome)
	assert.Equal(t, 1200, interactions[0].InputTokens)
	require.Len(t, interactions[0].ToolCalls, 1)
	assert.True(t, interactions[0].ToolCalls[0].Successful)

	assert.Equal(t, "test_transcript.jsonl-step-3", interactions[1].ID)
	assert.Equal(t, eval.OutcomeFailure, interactions[1].Outcome)
	assert.Contains(t, interactions[1].Errors, "nonzero_command_exit")
	require.Len(t, interactions[1].ToolCalls, 1)
	assert.False(t, interactions[1].ToolCalls[0].Successful)
}

func TestLLMScrutinyParser_ParseLLMTraceLog(t *testing.T) {
	traceContent := `
========== [Session: sess-alpha] LLM STEP 1 ==========
tool_choice: auto
tools: read_file, run_command
channel: native
----- PROMPT -----
[0 system]
System prompt
----- RESPONSE -----
[native tool_calls]
[{"name": "read_file"}]

========== [Session: sess-alpha] LLM STEP 2 ==========
tool_choice: auto
tools: edit_file
channel: markdown-recovered
server_hint: native tool_calls empty; model wrote markdown JSON.
----- PROMPT -----
[0 user]
Edit file
----- RESPONSE -----
{"name": "edit_file"}
`
	parser := eval.NewLLMScrutinyParser()
	interactions, err := parser.ParseLLMTraceLog(strings.NewReader(traceContent), "llm-trace-sess-alpha.log")
	require.NoError(t, err)
	require.Len(t, interactions, 2)

	assert.Equal(t, "sess-alpha-1", interactions[0].ID)
	assert.Equal(t, eval.OutcomeSuccess, interactions[0].Outcome)

	assert.Equal(t, "sess-alpha-2", interactions[1].ID)
	assert.Contains(t, interactions[1].Violations, "markdown_recovered_tool_call")
}

// Criterion 2: Operational Dynamic Proof - Multi-model correctness evaluation, ranking, and feedback.
func TestScrutinyEvaluator_MultiModelRanking(t *testing.T) {
	interactions := []eval.LLMInteraction{
		// Model A (Claude): 3 interactions, 3 successes -> 100%
		{ID: "1", Model: "claude-3-5-sonnet", Outcome: eval.OutcomeSuccess},
		{ID: "2", Model: "claude-3-5-sonnet", Outcome: eval.OutcomeSuccess},
		{ID: "3", Model: "claude-3-5-sonnet", Outcome: eval.OutcomeSuccess},
		// Model B (GPT-4o): 2 interactions, 1 success, 1 failure -> 50%
		{ID: "4", Model: "gpt-4o", Outcome: eval.OutcomeSuccess},
		{ID: "5", Model: "gpt-4o", Outcome: eval.OutcomeFailure, Errors: []string{"nonzero_command_exit"}},
		// Model C (Small Coder): 2 interactions, 1 syntax error, 1 failure -> ~15%
		{ID: "6", Model: "qwen2.5-coder-7b", Outcome: eval.OutcomeSyntaxErr, Errors: []string{"malformed_tool_call_json"}},
		{ID: "7", Model: "qwen2.5-coder-7b", Outcome: eval.OutcomeFailure, Errors: []string{"markdown_tool_call_dropped"}},
	}

	evaluator := eval.NewScrutinyEvaluator()
	report := evaluator.Evaluate(interactions)

	assert.Equal(t, 7, report.TotalInteractions)
	assert.Equal(t, 3, report.ModelsScrutinized)
	require.Len(t, report.Rankings, 3)

	// Model A should be Rank 1
	assert.Equal(t, 1, report.Rankings[0].Rank)
	assert.Equal(t, "claude-3-5-sonnet", report.Rankings[0].Model)
	assert.Equal(t, 100.0, report.Rankings[0].CompositeScore)
	assert.Equal(t, 1.0, report.Rankings[0].SuccessRate)

	// Model B should be Rank 2
	assert.Equal(t, 2, report.Rankings[1].Rank)
	assert.Equal(t, "gpt-4o", report.Rankings[1].Model)

	// Model C should be Rank 3
	assert.Equal(t, 3, report.Rankings[2].Rank)
	assert.Equal(t, "qwen2.5-coder-7b", report.Rankings[2].Model)
	assert.NotEmpty(t, report.Rankings[2].TuningFeedback)

	// Output formatters check
	tableOut := eval.FormatTable(report)
	assert.Contains(t, tableOut, "LLM INTERACTION SCRUTINY")
	assert.Contains(t, tableOut, "claude-3-5-sonnet")

	mdOut := eval.FormatMarkdown(report)
	assert.Contains(t, mdOut, "# LLM Interaction Scrutiny & Model Correctness Ranking")
	assert.Contains(t, mdOut, "| Rank | Model |")
}

// Criterion 3: Negative Adversarial Boundary - Corrupted JSON lines, empty files, missing fields.
func TestLLMScrutinyParser_CorruptedAndEmptyFailSoft(t *testing.T) {
	// Empty content
	parser := eval.NewLLMScrutinyParser()
	emptyInteractions, err := parser.ParseTranscriptJSONL(strings.NewReader(""), "empty.jsonl")
	require.NoError(t, err)
	assert.Empty(t, emptyInteractions)

	// Corrupted JSON and truncated steps
	corrupted := `
{not valid json at all}
{"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "tool_calls": "malformed_json_not_array"}
{"step_index": 2, "source": "MODEL", "type": "GENERIC", "content": "Command exited with code 0"}
`
	interactions, err := parser.ParseTranscriptJSONL(strings.NewReader(corrupted), "corrupted.jsonl")
	require.NoError(t, err)
	require.Len(t, interactions, 1)
	assert.Equal(t, eval.OutcomeSyntaxErr, interactions[0].Outcome)

	// Evaluator should not crash on empty interactions
	evaluator := eval.NewScrutinyEvaluator()
	report := evaluator.Evaluate(nil)
	assert.Equal(t, 0, report.TotalInteractions)
	assert.Equal(t, 0, report.ModelsScrutinized)
	assert.Empty(t, report.Rankings)

	tableOut := eval.FormatTable(report)
	assert.Contains(t, tableOut, "Total Interactions : 0")
}

func TestDiscoverLogs_FilterSince(t *testing.T) {
	tempDir := t.TempDir()
	logs, err := eval.DiscoverLogs(tempDir, 24*time.Hour, false)
	require.NoError(t, err)
	assert.Empty(t, logs)
}
