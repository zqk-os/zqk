package inbox_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/hive/inbox"
)

func TestMemoryInbox_Lifecycle(t *testing.T) {
	ibx := inbox.NewMemoryInbox()

	env := inbox.TDEEnvelope{
		ID:             "env-1",
		AgentID:        "agent-x",
		Intent:         "Delete log files",
		CapabilityRefs: []string{"fs:delete"},
		Payload:        []byte(`{"target": "*.log"}`),
	}

	// 1. Test Submit
	if err := ibx.Submit(env); err != nil {
		t.Fatalf("Failed to submit envelope: %v", err)
	}

	// 2. Test Duplicate Submit
	if err := ibx.Submit(env); err != inbox.ErrEnvelopeExists {
		t.Errorf("Expected ErrEnvelopeExists, got: %v", err)
	}

	// 3. Test ListPending
	pending := ibx.ListPending()
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending envelope, got %d", len(pending))
	}
	if pending[0].ID != "env-1" {
		t.Errorf("Expected env-1, got %s", pending[0].ID)
	}

	// 4. Test Get
	fetched, err := ibx.Get("env-1")
	if err != nil {
		t.Fatalf("Failed to get envelope: %v", err)
	}
	if fetched.Status != inbox.StatusPending {
		t.Errorf("Expected status pending, got %s", fetched.Status)
	}

	// 5. Test Approve
	if err := ibx.Approve("env-1"); err != nil {
		t.Fatalf("Failed to approve envelope: %v", err)
	}

	// Verify it's no longer pending
	if len(ibx.ListPending()) != 0 {
		t.Errorf("Expected 0 pending envelopes after approval")
	}

	// 6. Test already approved / Not Pending
	if err := ibx.Approve("env-1"); err != inbox.ErrNotPending {
		t.Errorf("Expected ErrNotPending on double approval, got: %v", err)
	}
	if err := ibx.Reject("env-1", "Changed my mind"); err != inbox.ErrNotPending {
		t.Errorf("Expected ErrNotPending when rejecting approved env, got: %v", err)
	}
}

func TestMemoryInbox_Reject(t *testing.T) {
	ibx := inbox.NewMemoryInbox()

	env := inbox.TDEEnvelope{
		ID:             "env-2",
		AgentID:        "agent-y",
		Intent:         "Drop production database",
		CapabilityRefs: []string{"db:drop"},
	}

	_ = ibx.Submit(env)

	if err := ibx.Reject("env-2", "Too risky"); err != nil {
		t.Fatalf("Failed to reject envelope: %v", err)
	}

	fetched, _ := ibx.Get("env-2")
	if fetched.Status != inbox.StatusRejected {
		t.Errorf("Expected status rejected, got %s", fetched.Status)
	}
	if fetched.RejectReason != "Too risky" {
		t.Errorf("Expected reject reason 'Too risky', got '%s'", fetched.RejectReason)
	}
}

func TestMemoryInbox_NotFound(t *testing.T) {
	ibx := inbox.NewMemoryInbox()

	if _, err := ibx.Get("missing"); err != inbox.ErrEnvelopeNotFound {
		t.Errorf("Expected ErrEnvelopeNotFound, got: %v", err)
	}

	if err := ibx.Approve("missing"); err != inbox.ErrEnvelopeNotFound {
		t.Errorf("Expected ErrEnvelopeNotFound on approve, got: %v", err)
	}

	if err := ibx.Reject("missing", "no"); err != inbox.ErrEnvelopeNotFound {
		t.Errorf("Expected ErrEnvelopeNotFound on reject, got: %v", err)
	}
}
