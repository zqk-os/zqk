package llmtranscript_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/telemetry/llmtranscript"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const sampleTranscriptJSONL = `
{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-06T18:00:00Z","content":"Build the parser"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-06T18:00:01Z","input_tokens":1000,"output_tokens":50,"tool_calls":[{"name":"run_command","args":{"CommandLine":"which zqk"}}]}
{"step_index":2,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-06T18:00:02Z","content":"Command exited with code 0.\n/usr/local/bin/zqk\n"}
{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-06T18:00:03Z","input_tokens":1200,"output_tokens":80,"tool_calls":[{"name":"run_command","args":{"CommandLine":"cat non_existent_file.txt"}}]}
{"step_index":4,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-06T18:00:04Z","content":"Command exited with code 1.\ncat: non_existent_file.txt: No such file or directory\n"}
{"step_index":5,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-06T18:00:05Z","input_tokens":1400,"output_tokens":60,"tool_calls":[{"name":"run_command","args":{"CommandLine":"ls -la"}}]}
{"step_index":6,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-06T18:00:06Z","content":"Command exited with code 0.\ntotal 0\n"}
{"step_index":7,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-06T18:00:07Z","input_tokens":1500,"output_tokens":120,"tool_calls":[{"name":"edit_file","args":{"Path":"/tmp/test.go","Target":"func foo()","Replacement":"func bar()"}}]}
{"step_index":8,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-06T18:00:08Z","content":"Error: Target not found in file: func foo()\n"}
`

func TestParseTranscript_Success(t *testing.T) {
	steps, err := llmtranscript.ParseTranscript(strings.NewReader(sampleTranscriptJSONL))
	require.NoError(t, err)
	assert.Len(t, steps, 9)

	assert.Equal(t, 0, steps[0].StepIndex)
	assert.Equal(t, "USER_EXPLICIT", steps[0].Source)
	assert.Equal(t, "USER_INPUT", steps[0].Type)

	assert.Equal(t, 1, steps[1].StepIndex)
	assert.Equal(t, "PLANNER_RESPONSE", steps[1].Type)
	assert.Equal(t, int64(1000), steps[1].InputTokens)
	require.Len(t, steps[1].ToolCalls, 1)
	assert.Equal(t, "run_command", steps[1].ToolCalls[0].Name)
}

func TestExtractTurns_EvaluatesTurnOutcomesAndSelfCorrection(t *testing.T) {
	steps, err := llmtranscript.ParseTranscript(strings.NewReader(sampleTranscriptJSONL))
	require.NoError(t, err)

	turns := llmtranscript.ExtractTurns(steps, "Gemini 3.8 Flash")
	require.Len(t, turns, 4)

	// Turn 0: step 1 (which zqk -> code 0)
	assert.Equal(t, 1, turns[0].StepIndex)
	assert.True(t, turns[0].IsSuccess)
	assert.False(t, turns[0].IsSelfCorrection)
	require.Len(t, turns[0].Executions, 1)
	assert.Equal(t, 0, turns[0].Executions[0].ExitCode)

	// Turn 1: step 3 (cat non_existent_file.txt -> code 1)
	assert.Equal(t, 3, turns[1].StepIndex)
	assert.False(t, turns[1].IsSuccess)
	assert.False(t, turns[1].IsSelfCorrection)
	require.Len(t, turns[1].Executions, 1)
	assert.Equal(t, 1, turns[1].Executions[0].ExitCode)

	// Turn 2: step 5 (ls -la -> code 0) -> this was a self correction!
	assert.Equal(t, 5, turns[2].StepIndex)
	assert.True(t, turns[2].IsSuccess)
	assert.True(t, turns[2].IsSelfCorrection)
	assert.True(t, turns[2].CorrectedPrevious)

	// Turn 3: step 7 (edit_file -> target not found error)
	assert.Equal(t, 7, turns[3].StepIndex)
	assert.False(t, turns[3].IsSuccess)
	assert.False(t, turns[3].IsSelfCorrection)
	require.Len(t, turns[3].Executions, 1)
	assert.False(t, turns[3].Executions[0].Success)
	assert.Contains(t, turns[3].Executions[0].ErrorMessage, "Target not found")
}

func TestEvaluateTurns_GeneratesRankingsAndPromptHoning(t *testing.T) {
	steps, err := llmtranscript.ParseTranscript(strings.NewReader(sampleTranscriptJSONL))
	require.NoError(t, err)

	turns := llmtranscript.ExtractTurns(steps, "Gemini 3.8 Flash")
	rankings := llmtranscript.EvaluateTurns(turns)

	require.Len(t, rankings, 1)
	metric := rankings[0]

	assert.Equal(t, "Gemini 3.8 Flash", metric.Model)
	assert.Equal(t, 4, metric.TotalTurns)
	assert.Equal(t, 4, metric.TotalToolCalls)
	assert.Equal(t, 2, metric.SuccessfulToolCalls)
	assert.Equal(t, 2, metric.FailedToolCalls)
	assert.Equal(t, 0.5, metric.ToolAccuracy)

	assert.Equal(t, 1, metric.SelfCorrectionAttempts)
	assert.Equal(t, 1, metric.SelfCorrectionSuccesses)
	assert.Equal(t, 1.0, metric.SelfCorrectionRate)

	assert.Greater(t, metric.CompositeScore, 0.0)
	assert.Equal(t, 1, metric.Rank)

	// Check failure categorization
	require.NotEmpty(t, metric.TopFailurePatterns)
	// Check prompt honing tips
	require.NotEmpty(t, metric.PromptHoningTips)
	foundEditTip := false
	for _, tip := range metric.PromptHoningTips {
		if strings.Contains(tip, "edit_file") {
			foundEditTip = true
			break
		}
	}
	assert.True(t, foundEditTip, "expected prompt honing tip for edit_file failure")
}

func TestAnalyzeDirectory_MultiModelComparison(t *testing.T) {
	tmpDir := t.TempDir()

	conv1Dir := filepath.Join(tmpDir, "conv1", ".system_generated", "logs")
	require.NoError(t, fileutil.MkdirAll(conv1Dir, 0755))
	conv1Log := filepath.Join(conv1Dir, "transcript.jsonl")
	require.NoError(t, fileutil.WriteFile(conv1Log, []byte(sampleTranscriptJSONL), 0644))

	report, err := llmtranscript.AnalyzeDirectory(tmpDir, "Gemini 3.8 Flash")
	require.NoError(t, err)
	assert.Equal(t, 1, report.ConversationsAnalyzed)
	assert.Equal(t, 9, report.TotalSteps)
	assert.Equal(t, 4, report.TotalTurns)
	require.Len(t, report.ModelRankings, 1)
	assert.Equal(t, "Gemini 3.8 Flash", report.ModelRankings[0].Model)
}

func TestParseTranscript_EmptyAndMalformed(t *testing.T) {
	// Empty reader
	steps, err := llmtranscript.ParseTranscript(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, steps)

	// Malformed JSON line
	malformed := `{"step_index":0, "status": "DONE"}
{"invalid json line
{"step_index":2, "status": "DONE"}
`
	_, err = llmtranscript.ParseTranscript(strings.NewReader(malformed))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "line 2: malformed json")
}

func TestExtractTurns_EmptySteps(t *testing.T) {
	turns := llmtranscript.ExtractTurns(nil, "TestModel")
	assert.Empty(t, turns)

	// Only user input steps
	userOnly := []llmtranscript.TranscriptStep{
		{StepIndex: 0, Source: "USER_EXPLICIT", Type: "USER_INPUT", Content: "Hello"},
	}
	turns = llmtranscript.ExtractTurns(userOnly, "TestModel")
	assert.Empty(t, turns)
}

func TestEvaluateTurns_ZeroToolCalls(t *testing.T) {
	turns := []llmtranscript.Turn{
		{
			StepIndex:   1,
			Model:       "ModelZero",
			InputTokens: 500,
			IsSuccess:   true,
		},
	}
	metrics := llmtranscript.EvaluateTurns(turns)
	require.Len(t, metrics, 1)
	assert.Equal(t, 1.0, metrics[0].ToolAccuracy)
	assert.Equal(t, 1.0, metrics[0].SelfCorrectionRate)
	assert.Greater(t, metrics[0].CompositeScore, 0.0)
}

func TestAnalyzeDirectory_NonExistentDir(t *testing.T) {
	report, err := llmtranscript.AnalyzeDirectory("/path/that/does/not/exist/anywhere", "ModelX")
	require.NoError(t, err) // filepath.Walk on non-existent directory returns nil or handled gracefully
	assert.Equal(t, 0, report.ConversationsAnalyzed)
}
