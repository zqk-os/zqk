package agentfeed

import (
	"errors"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
)

func TestAuthorizePeerAck_deniesCrossSeatImpersonation(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-auth",
	}); err != nil {
		t.Fatal(err)
	}
	steer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN AGY-2: do the thing",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-02",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = AuthorizePeerAck(root, "peer-agent-01", steer.EventID)
	if err == nil {
		t.Fatal("expected deny when AGY-1 acks as AGY-2's steer")
	}
	if !errors.Is(err, ErrPeerAckSeatMismatch) {
		t.Fatalf("want ErrPeerAckSeatMismatch, got %v", err)
	}

	if _, err := AppendPeerAck(root, "peer-agent-01", "PER-DEFAULT-AGENT", steer.EventID, "cleared by AGY-1"); err == nil {
		t.Fatal("AppendPeerAck must reject impersonation")
	}

	if _, err := AppendPeerAck(root, "peer-agent-02", "PER-DEFAULT-AGENT", steer.EventID, "real ack"); err != nil {
		t.Fatalf("addressed seat must be allowed: %v", err)
	}
}

func TestCorrespondence_impersonatedAckDoesNotClearOutbox(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-imp",
	}); err != nil {
		t.Fatal(err)
	}
	steer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "COMMS to AGY-2",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-02",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     false,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Bypass AuthorizePeerAck: stamp a forged peer_ack row directly (historical breach shape).
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "cleared by AGY-1 to unblock swarm",
		AgentID:     "peer-agent-02", // forged stamp
		PersonaRef:  "PER-DEFAULT-AGENT",
		Sender:      FeedSenderPeerAck,
		EventType:   FeedEventTypePeerAck,
		InReplyTo:   steer.EventID,
		SelfACK:     false,
	}); err != nil {
		t.Fatal(err)
	}
	// Wait — forged stamp uses peer-agent-02 as agent_id which WOULD satisfy parent.
	// Real breach was CLI --agent-id peer-agent-02 run by AGY-1. Correspondence
	// cannot see the human operator; AuthorizePeerAck is the gate. Simulate wrong
	// agent_id on the ack row instead:
	steer2, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "COMMS2 to AGY-2",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-02",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "cleared by AGY-1",
		AgentID:     "peer-agent-01", // wrong seat on the wire
		PersonaRef:  "PER-DEFAULT-AGENT",
		Sender:      FeedSenderPeerAck,
		EventType:   FeedEventTypePeerAck,
		InReplyTo:   steer2.EventID,
		SelfACK:     false,
	}); err != nil {
		t.Fatal(err)
	}

	tpm := Seat{AgentID: "peer-tpm-01"}
	snap, err := LoadCorrespondence(root, tpm, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range snap.OutboxAwaitingPeerAck {
		if x.EventID == steer2.EventID {
			found = true
		}
	}
	if !found {
		t.Fatalf("wrong-seat peer_ack must not clear TPM outbox; outbox=%+v", snap.OutboxAwaitingPeerAck)
	}
}

func TestCompletePeerAckAwaits_requiresAddressedSeat(t *testing.T) {
	root := t.TempDir()
	_, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "AFE-bound",
		FromAgentID: "peer-tpm-01",
		ToAgentID:   "peer-agent-02",
		Action:      AwaitActionWake,
	})
	if err != nil {
		t.Fatal(err)
	}
	done, err := CompletePeerAckAwaits(root, "AFE-bound", "peer-agent-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 0 {
		t.Fatalf("wrong seat must not complete await: %+v", done)
	}
	done, err = CompletePeerAckAwaits(root, "AFE-bound", "peer-agent-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 {
		t.Fatalf("addressed seat should complete: %+v", done)
	}
}
