package system_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/system"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const mockTranscript = `
{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-06T18:00:00Z","content":"Build parser"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-06T18:00:01Z","input_tokens":500,"output_tokens":40,"tool_calls":[{"name":"run_command","args":{"CommandLine":"echo hello"}}]}
{"step_index":2,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-06T18:00:02Z","content":"Command exited with code 0.\nhello\n"}
`

func TestNewLLMMetricsCmd_FlagsAndStructure(t *testing.T) {
	cmd := system.NewLLMMetricsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "llm", cmd.Name())

	assert.NotNil(t, cmd.Flags().Lookup("dir"))
	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("model"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))
}

func TestRunLLMMetrics_WithFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "transcript.jsonl")
	require.NoError(t, fileutil.WriteFile(logFile, []byte(mockTranscript), 0644))

	cmd := system.NewLLMMetricsCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--file", logFile, "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestRunLLMMetrics_WithDirFlag(t *testing.T) {
	tmpDir := t.TempDir()
	convDir := filepath.Join(tmpDir, "session-1", ".system_generated", "logs")
	require.NoError(t, fileutil.MkdirAll(convDir, 0755))
	logFile := filepath.Join(convDir, "transcript.jsonl")
	require.NoError(t, fileutil.WriteFile(logFile, []byte(mockTranscript), 0644))

	cmd := system.NewLLMMetricsCmd()
	cmd.SetArgs([]string{"--dir", tmpDir, "--format", "table"})

	err := cmd.Execute()
	require.NoError(t, err)
}

func TestRunLLMMetrics_NonExistentFileReturnsError(t *testing.T) {
	cmd := system.NewLLMMetricsCmd()
	cmd.SetArgs([]string{"--file", "/does/not/exist/transcript.jsonl"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse transcript file")
}

func TestRunLLMMetrics_RequiresFileOrDir(t *testing.T) {
	cmd := system.NewLLMMetricsCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provide --file")
}
