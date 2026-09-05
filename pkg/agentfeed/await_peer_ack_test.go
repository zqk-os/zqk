package agentfeed

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestRegisterPeerAckAwait_listsOpen(t *testing.T) {
	root := t.TempDir()
	a, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "AFE-parent-1",
		FromAgentID: "peer-tpm-01",
		ToAgentID:   "peer-agent-01",
		Action:      AwaitActionWake,
		WakeMessage: "ATTN TPM: peer ack received for AFE-parent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.Status != AwaitStatusOpen {
		t.Fatalf("await=%+v", a)
	}
	open, err := ListOpenPeerAckAwaits(root, "peer-tpm-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].EventID != "AFE-parent-1" {
		t.Fatalf("open=%+v", open)
	}
	// Other seat sees empty
	other, err := ListOpenPeerAckAwaits(root, "peer-agent-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("other seat should not own await: %+v", other)
	}
}

func TestCompletePeerAckAwaits_firesCallbackOnce(t *testing.T) {
	root := t.TempDir()
	var fires int32
	prev := PeerAckAwaitFire
	PeerAckAwaitFire = func(a PeerAckAwait) error {
		atomic.AddInt32(&fires, 1)
		if a.EventID != "AFE-parent-2" {
			t.Errorf("event=%s", a.EventID)
		}
		return nil
	}
	t.Cleanup(func() { PeerAckAwaitFire = prev })

	_, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "AFE-parent-2",
		FromAgentID: "tpm-seat",
		Action:      AwaitActionWake,
	})
	if err != nil {
		t.Fatal(err)
	}
	done, err := CompletePeerAckAwaits(root, "AFE-parent-2", "agy")
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || done[0].Status != AwaitStatusCompleted {
		t.Fatalf("done=%+v", done)
	}
	if atomic.LoadInt32(&fires) != 1 {
		t.Fatalf("fires=%d", fires)
	}
	// Second complete is no-op
	done2, err := CompletePeerAckAwaits(root, "AFE-parent-2", "agy")
	if err != nil {
		t.Fatal(err)
	}
	if len(done2) != 0 || atomic.LoadInt32(&fires) != 1 {
		t.Fatalf("second complete done=%+v fires=%d", done2, fires)
	}
	open, _ := ListOpenPeerAckAwaits(root, "tpm-seat")
	if len(open) != 0 {
		t.Fatalf("still open: %+v", open)
	}
}

func TestAppendPeerAckWithSession_completesOpenAwait(t *testing.T) {
	prev := PeerAckAwaitFire
	PeerAckAwaitFire = func(a PeerAckAwait) error { return nil }
	t.Cleanup(func() { PeerAckAwaitFire = prev })

	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-ack-closes-await",
	}); err != nil {
		t.Fatal(err)
	}
	steer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN peer-agent-1: COMMS-CHECK",
		AgentID:     "peer-operator-1",
		ToAgentID:   "peer-agent-1",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     steer.EventID,
		FromAgentID: "peer-operator-1",
		ToAgentID:   "peer-agent-1",
		Action:      AwaitActionWake,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendPeerAckWithSession(root, "peer-agent-1", "PER-ORCH-ALPHA", "ZQK-SESS-1", steer.EventID, "LIFE"); err != nil {
		t.Fatal(err)
	}
	open, err := ListOpenPeerAckAwaits(root, "peer-operator-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("seat-worker peer_ack must close the hourglass await, still open=%+v", open)
	}
}

func TestRegisterPeerAckAwait_requiresEventAndFrom(t *testing.T) {
	root := t.TempDir()
	if _, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{FromAgentID: "tpm"}); err == nil {
		t.Fatal("expected error for missing event_id")
	}
	if _, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{EventID: "AFE-x"}); err == nil {
		t.Fatal("expected error for missing from_agent_id")
	}
}

func TestPeerAckAwaitStorePath_underStateMesh(t *testing.T) {
	root := t.TempDir()
	p := peerAckAwaitStorePath(root)
	want := paths.PeerAckAwaitsPath(root)
	if p != want {
		t.Fatalf("path=%q want %q", p, want)
	}
}

func TestExpirePeerAckAwaits(t *testing.T) {
	root := t.TempDir()

	// Register an await
	a, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "AFE-stale",
		FromAgentID: "tpm-seat",
		Action:      AwaitActionWake,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait a tiny bit just in case
	time.Sleep(10 * time.Millisecond)

	// Expire with a very long timeout (should not expire)
	expired, err := ExpirePeerAckAwaits(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 0 {
		t.Fatalf("expected 0 expired, got %d", len(expired))
	}

	// Expire with a 0 timeout (should expire it immediately)
	expired, err = ExpirePeerAckAwaits(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
	if expired[0].ID != a.ID || expired[0].Status != AwaitStatusExpired {
		t.Fatalf("unexpected expired await: %+v", expired[0])
	}

	// List open awaits should now be empty
	open, err := ListOpenPeerAckAwaits(root, "tpm-seat")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("expected 0 open awaits, got %d", len(open))
	}
}
