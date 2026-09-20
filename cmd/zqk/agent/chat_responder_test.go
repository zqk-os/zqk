package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGetTranscriptContext(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "transcript.jsonl")

	// Test with non-existent file
	ctx := getTranscriptContext(logPath)
	if ctx != "" {
		t.Errorf("Expected empty context for non-existent file, got %q", ctx)
	}

	// Create a dummy transcript
	dummyTranscript := `{"step_index": 1, "type": "USER_INPUT", "content": "Hello"}
{"step_index": 2, "type": "PLANNER_RESPONSE", "content": "Hi there"}
{"step_index": 3, "type": "SYSTEM", "content": "ignoring this"}
{"step_index": 4, "type": "USER_INPUT", "content": ""}
`
	err := fileutil.WriteStandardFile(logPath, []byte(dummyTranscript))
	if err != nil {
		t.Fatalf("Failed to write transcript: %v", err)
	}

	ctx = getTranscriptContext(logPath)
	if !strings.Contains(ctx, "User: Hello") {
		t.Errorf("Expected context to contain 'User: Hello', got %q", ctx)
	}
	if !strings.Contains(ctx, "IDE Assistant: Hi there") {
		t.Errorf("Expected context to contain 'IDE Assistant: Hi there', got %q", ctx)
	}
	if strings.Contains(ctx, "ignoring this") {
		t.Errorf("Expected context NOT to contain system messages, got %q", ctx)
	}
}
