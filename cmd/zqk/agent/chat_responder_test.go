package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/adapters/antigravity"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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

func TestNewChatResponderCmd_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No active Antigravity transcript found") {
		t.Logf("chat-responder output: %s", out)
	}
}

func TestChatResponder_WithTranscript_NoNewMessages(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	transcriptDir := filepath.Join(tempDir, ".system_generated", "logs")
	_ = fileutil.EnsureDir(transcriptDir)
	logPath := filepath.Join(transcriptDir, "transcript.jsonl")
	_ = fileutil.WriteStandardFile(logPath, []byte(""))

	t.Setenv(zqkenv.AGTranscriptPath().Name(), logPath)

	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No new messages found") {
		t.Errorf("expected 'No new messages found' in output, got: %s", out)
	}
}

func TestChatResponder_WithTranscript_NewMessages(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	transcriptDir := filepath.Join(tempDir, ".system_generated", "logs")
	_ = fileutil.EnsureDir(transcriptDir)
	logPath := filepath.Join(transcriptDir, "transcript.jsonl")

	entry := `{"step_index": 1, "type": "USER_INPUT", "content": "What is next?", "timestamp": "2026-10-07T12:00:00Z"}` + "\n"
	_ = fileutil.WriteStandardFile(logPath, []byte(entry))

	t.Setenv(zqkenv.AGTranscriptPath().Name(), logPath)

	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Found 1 new messages") {
		t.Errorf("expected 'Found 1 new messages' in output, got: %s", out)
	}
}

func TestChatResponder_WithExistingSessionState(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	transcriptDir := filepath.Join(tempDir, ".system_generated", "logs")
	_ = fileutil.EnsureDir(transcriptDir)
	logPath := filepath.Join(transcriptDir, "transcript.jsonl")

	entry := `{"step_index": 1, "type": "USER_INPUT", "content": "What is next?", "timestamp": "2026-10-07T12:00:00Z"}` + "\n"
	_ = fileutil.WriteStandardFile(logPath, []byte(entry))

	t.Setenv(zqkenv.AGTranscriptPath().Name(), logPath)

	// Pre-create session object with last_step = 1 so the message is considered already processed
	convID := "DEFAULT"
	if root, err := antigravity.ConversationRootFromTranscript(logPath); err == nil {
		if id := antigravity.ConversationID(root); id != "" {
			convID = id
		}
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(convID))
	sessionID := fmt.Sprintf("ZQK-%d", hash.Sum32())

	statePayload, _ := json.Marshal(map[string]any{
		"last_step":       float64(1),
		"last_nudge_time": time.Now().UTC().Format(time.RFC3339),
	})
	sessionObj := map[string]any{
		objects.FieldKeyID:            sessionID,
		objects.FieldKeyKind:          objects.KindZqkSession,
		objects.FieldKeyTitle:         string(statePayload),
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	require.NoError(t, provider.Create(ctx, secCtx, sessionObj))

	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No new messages found") {
		t.Errorf("expected 'No new messages found', got: %s", out)
	}
}

func TestChatResponder_IdleDetection(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	transcriptDir := filepath.Join(tempDir, ".system_generated", "logs")
	_ = fileutil.EnsureDir(transcriptDir)
	logPath := filepath.Join(transcriptDir, "transcript.jsonl")

	oldTime := time.Now().Add(-20 * time.Minute)
	entry := fmt.Sprintf(`{"step_index": 1, "type": "USER_INPUT", "content": "hello", "timestamp": %q}`+"\n", oldTime.Format(time.RFC3339))
	_ = fileutil.WriteStandardFile(logPath, []byte(entry))

	t.Setenv(zqkenv.AGTranscriptPath().Name(), logPath)

	convID := "DEFAULT"
	if root, err := antigravity.ConversationRootFromTranscript(logPath); err == nil {
		if id := antigravity.ConversationID(root); id != "" {
			convID = id
		}
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(convID))
	sessionID := fmt.Sprintf("ZQK-%d", hash.Sum32())

	// When last_nudge_time is after oldTime -> "already nudged"
	statePayload, _ := json.Marshal(map[string]any{
		"last_step":       float64(1),
		"last_nudge_time": time.Now().Format(time.RFC3339),
	})
	sessionObj := map[string]any{
		objects.FieldKeyID:            sessionID,
		objects.FieldKeyKind:          objects.KindZqkSession,
		objects.FieldKeyTitle:         string(statePayload),
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	require.NoError(t, provider.Create(ctx, secCtx, sessionObj))

	out, err := executeAgentCommand(t, tempDir, provider, "chat-responder")
	require.NoError(t, err)
	if !strings.Contains(out, "already nudged") {
		t.Errorf("expected 'already nudged' in output, got: %s", out)
	}
}
