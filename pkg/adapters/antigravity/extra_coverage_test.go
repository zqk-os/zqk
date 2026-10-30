package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestConversationID_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{".", ""},
		{string(filepath.Separator), ""},
		{"/a/b/my-conversation-id", "my-conversation-id"},
	}

	for _, tc := range tests {
		got := ConversationID(tc.input)
		if got != tc.want {
			t.Errorf("ConversationID(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestInboxAdapter_DeliverErrors(t *testing.T) {
	// 1. Invalid transcript path
	adapter := &InboxAdapter{TranscriptPath: "invalid-transcript"}
	err := adapter.Deliver(context.Background(), "msg", "id-1", nil)
	if err == nil {
		t.Fatal("expected error for invalid transcript path")
	}

	// 2. EnsureDir failure (convRoot is blocked by a file)
	tmp := t.TempDir()
	convRoot := filepath.Join(tmp, "blocked-conv")
	transcript := filepath.Join(convRoot, systemGeneratedDir, "logs", "transcript.jsonl")
	// Make a file where systemGeneratedDir should be a directory
	if err := os.MkdirAll(convRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	blockedFile := filepath.Join(convRoot, systemGeneratedDir)
	if err := os.WriteFile(blockedFile, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}

	adapterBlocked := &InboxAdapter{TranscriptPath: transcript}
	err = adapterBlocked.Deliver(context.Background(), "msg", "id-2", nil)
	if err == nil {
		t.Fatal("expected error when directory creation is blocked")
	}
}

func TestResolveActiveTranscript(t *testing.T) {
	tmp := t.TempDir()

	// Scenario 1: AG_TRANSCRIPT_PATH points to an existing file
	validFile := filepath.Join(tmp, "custom_transcript.jsonl")
	if err := os.WriteFile(validFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(zqkenv.AGTranscriptPath().Key, validFile)

	got := ResolveActiveTranscript()
	if got != validFile {
		t.Fatalf("expected %q, got %q", validFile, got)
	}

	// Scenario 2: AG_TRANSCRIPT_PATH points to non-existent file, and HOME has mock brains
	t.Setenv(zqkenv.AGTranscriptPath().Key, filepath.Join(tmp, "non_existent.jsonl"))

	mockHome := filepath.Join(tmp, "home")
	t.Setenv("HOME", mockHome)

	// Sub-scenario 2a: No brain directory in mockHome
	if res := ResolveActiveTranscript(); res != "" {
		t.Fatalf("expected empty when no transcripts match, got %q", res)
	}

	// Sub-scenario 2b: Create two mock transcripts with different mod times
	conv1Log := filepath.Join(mockHome, ".gemini", "antigravity-cli", "brain", "conv1", systemGeneratedDir, "logs", "transcript.jsonl")
	conv2Log := filepath.Join(mockHome, ".gemini", "antigravity-cli", "brain", "conv2", systemGeneratedDir, "logs", "transcript.jsonl")

	if err := fileutil.EnsureDir(filepath.Dir(conv1Log)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(filepath.Dir(conv2Log)); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(conv1Log, []byte("conv1"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(conv2Log, []byte("conv2"), 0o600); err != nil {
		t.Fatal(err)
	}

	// conv2 is newer
	gotNewest := ResolveActiveTranscript()
	if gotNewest != conv2Log {
		t.Fatalf("expected newest %q, got %q", conv2Log, gotNewest)
	}

	// Scenario 3: Empty home
	t.Setenv(zqkenv.AGTranscriptPath().Key, "")
	t.Setenv("HOME", "")
	if res := ResolveActiveTranscript(); res != "" {
		t.Fatalf("expected empty transcript path when HOME is empty, got %q", res)
	}
}
