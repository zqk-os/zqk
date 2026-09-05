package ambient

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestSuspendAndWakeContext(t *testing.T) {
	projectRoot := t.TempDir()
	agentID := "test-agent-123"
	logger := logging.GetLoggerFromProfile("test")

	state := map[string]any{
		"intent":      "analyze codebase",
		"progress":    42,
		"active_file": "main.go",
	}

	filePath, err := SuspendContext(projectRoot, agentID, state, logger)
	if err != nil {
		t.Fatalf("SuspendContext failed: %v", err)
	}

	if filepath.Base(filePath) != "test-agent-123.csnap" {
		t.Errorf("unexpected filename: %s", filepath.Base(filePath))
	}

	if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
		t.Errorf("snapshot file was not created: %s", filePath)
	}

	awokenState, err := WakeContext(projectRoot, agentID, logger)
	if err != nil {
		t.Fatalf("WakeContext failed: %v", err)
	}

	if intent, ok := awokenState["intent"].(string); !ok || intent != "analyze codebase" {
		t.Errorf("unexpected intent: %v", awokenState["intent"])
	}

	progressAny, ok := awokenState["progress"]
	if !ok {
		t.Errorf("progress field missing")
	} else {
		switch v := progressAny.(type) {
		case int:
			if v != 42 {
				t.Errorf("unexpected progress: %v", v)
			}
		case float64:
			if v != 42.0 {
				t.Errorf("unexpected progress: %v", v)
			}
		case int64:
			if v != 42 {
				t.Errorf("unexpected progress: %v", v)
			}
		default:
			t.Errorf("unexpected progress type: %T", v)
		}
	}

	if file, ok := awokenState["active_file"].(string); !ok || file != "main.go" {
		t.Errorf("unexpected active_file: %v", awokenState["active_file"])
	}
}

func TestSuspendContext_EmptyInputs(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")

	_, err := SuspendContext("", "agent-1", map[string]any{}, logger)
	if err == nil {
		t.Errorf("expected error for empty project root")
	}

	_, err = SuspendContext("root", "", map[string]any{}, logger)
	if err == nil {
		t.Errorf("expected error for empty agent ID")
	}

	_, err = SuspendContext("root", "agent-1", nil, logger)
	if err == nil {
		t.Errorf("expected error for nil state")
	}
}

func TestWakeContext_EmptyInputs(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")

	_, err := WakeContext("", "agent-1", logger)
	if err == nil {
		t.Errorf("expected error for empty project root")
	}

	_, err = WakeContext("root", "", logger)
	if err == nil {
		t.Errorf("expected error for empty agent ID")
	}
}
