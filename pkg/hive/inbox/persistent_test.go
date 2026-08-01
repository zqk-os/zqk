package inbox_test

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/hive/inbox"
)

func TestPersistentInbox_Replay(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "inbox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ibx, err := inbox.NewPersistentInbox(tempDir)
	if err != nil {
		t.Fatalf("Failed to create persistent inbox: %v", err)
	}

	env1 := inbox.TDEEnvelope{
		ID:      "env-1",
		AgentID: "agent-x",
		Intent:  "Delete log files",
	}
	env2 := inbox.TDEEnvelope{
		ID:      "env-2",
		AgentID: "agent-y",
		Intent:  "Format disk",
	}

	_ = ibx.Submit(env1)
	_ = ibx.Submit(env2)
	_ = ibx.Approve("env-1")
	_ = ibx.Reject("env-2", "Too dangerous")

	ibx.Close()

	// Re-open and verify replay
	ibx2, err := inbox.NewPersistentInbox(tempDir)
	if err != nil {
		t.Fatalf("Failed to reopen persistent inbox: %v", err)
	}
	defer ibx2.Close()

	// Verify env-1 is approved
	fetched1, _ := ibx2.Get("env-1")
	if fetched1.Status != inbox.StatusApproved {
		t.Errorf("Expected env-1 to be approved, got %s", fetched1.Status)
	}

	// Verify env-2 is rejected
	fetched2, _ := ibx2.Get("env-2")
	if fetched2.Status != inbox.StatusRejected {
		t.Errorf("Expected env-2 to be rejected, got %s", fetched2.Status)
	}
	if fetched2.RejectReason != "Too dangerous" {
		t.Errorf("Expected reject reason 'Too dangerous', got '%s'", fetched2.RejectReason)
	}

	// Verify pending count is 0
	if len(ibx2.ListPending()) != 0 {
		t.Errorf("Expected 0 pending envelopes")
	}
}

func TestPersistentInbox_Errors(t *testing.T) {
	// Test failure to create dir
	_, err := inbox.NewPersistentInbox("/root/forbidden-dir-test-xyz")
	if err == nil {
		t.Errorf("Expected error creating persistent inbox in forbidden dir")
	}
}
