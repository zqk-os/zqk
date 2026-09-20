package antigravity

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestConversationRootFromTranscript(t *testing.T) {
	nested := filepath.Join("brain", "conv-abc", systemGeneratedDir, "logs", "nested", "transcript.jsonl")
	root, err := ConversationRootFromTranscript(nested)
	if err != nil {
		t.Fatalf("ConversationRootFromTranscript: %v", err)
	}
	want := filepath.Join("brain", "conv-abc")
	if root != want {
		t.Fatalf("root=%q want %q", root, want)
	}
	if ConversationID(root) != "conv-abc" {
		t.Fatalf("ConversationID=%q", ConversationID(root))
	}
	if !IsBrainTranscript(nested) {
		t.Fatal("expected IsBrainTranscript true")
	}
}

func TestIsBrainTranscript_RejectsNonVendorPaths(t *testing.T) {
	for _, p := range []string{
		"",
		"transcript.jsonl",
		filepath.Join("foo", "not.system_generated", "logs", "transcript.jsonl"),
		filepath.Join("foo", ".system_generated_backup", "logs", "transcript.jsonl"),
	} {
		if IsBrainTranscript(p) {
			t.Fatalf("IsBrainTranscript(%q) = true", p)
		}
	}
}

func TestInboxAdapter_DeliverWritesVendorInbox(t *testing.T) {
	conv := filepath.Join(t.TempDir(), "conv-xyz")
	transcript := filepath.Join(conv, systemGeneratedDir, "logs", "transcript.jsonl")
	if err := fileutil.EnsureDir(filepath.Dir(transcript)); err != nil {
		t.Fatal(err)
	}

	a := &InboxAdapter{TranscriptPath: transcript}
	if a.Vendor() != VendorID {
		t.Fatalf("Vendor=%q", a.Vendor())
	}
	if err := a.Deliver(context.Background(), "hello", "msg-1", map[string]string{"messageTitle": "t"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	raw, err := fileutil.ReadFile(filepath.Join(conv, systemGeneratedDir, messagesDir, "msg-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["Recipient"] != "conv-xyz" || got["Message"] != "hello" {
		t.Fatalf("payload=%v", got)
	}
	if !fileutil.Exists(filepath.Join(conv, systemGeneratedDir, messagesDir, undeliveredDir, "msg-1")) {
		t.Fatal("expected undelivered pointer")
	}
}
