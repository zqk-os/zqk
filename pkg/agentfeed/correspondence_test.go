package agentfeed

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAppendEvent_assignsEventIDAndKernelAck(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-id",
	}); err != nil {
		t.Fatal(err)
	}
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN AGY: do work",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-01",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.EventID == "" || res.Event[JSONFieldEventID] != res.EventID {
		t.Fatalf("event_id missing: %+v", res.Event)
	}
	if res.ACK == nil {
		t.Fatal("expected kernel ack")
	}
	if res.ACK[objects.FieldKeyEventType] != FeedEventTypeKernelAck {
		t.Fatalf("kernel ack type=%v", res.ACK[objects.FieldKeyEventType])
	}
	if res.ACK[JSONFieldInReplyTo] != res.EventID {
		t.Fatalf("kernel ack in_reply_to=%v want %s", res.ACK[JSONFieldInReplyTo], res.EventID)
	}
}

func TestCorrespondence_inboxOutboxPeerAck(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-corr",
	}); err != nil {
		t.Fatal(err)
	}

	steer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN AGY — START ATK-1",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-01",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	agy := Seat{AgentID: "peer-agent-01", PersonaRef: "PER-coder", RoleHints: []string{"agy"}}
	tpm := Seat{AgentID: "peer-tpm-01", RoleHints: []string{"tpm"}}

	snapAGY, err := LoadCorrespondence(root, agy, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapAGY.InboxUnacked) != 1 || snapAGY.InboxUnacked[0].EventID != steer.EventID {
		t.Fatalf("agy inbox=%+v", snapAGY.InboxUnacked)
	}
	if snapAGY.NextActionHint != HintAckThenContinue {
		t.Fatalf("hint=%s", snapAGY.NextActionHint)
	}

	snapTPM, err := LoadCorrespondence(root, tpm, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapTPM.OutboxAwaitingPeerAck) != 1 {
		t.Fatalf("tpm outbox=%+v", snapTPM.OutboxAwaitingPeerAck)
	}
	if len(snapTPM.InboxUnacked) != 0 {
		t.Fatalf("tpm should not see own steer as inbox: %+v", snapTPM.InboxUnacked)
	}

	if _, err := AppendPeerAck(root, agy.AgentID, agy.PersonaRef, steer.EventID, "ACK START"); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendDeliveryReceipt(root, "wake", steer.EventID, ""); err != nil {
		t.Fatal(err)
	}

	snapAGY2, err := LoadCorrespondence(root, agy, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapAGY2.InboxUnacked) != 0 {
		t.Fatalf("inbox should clear after peer_ack: %+v", snapAGY2.InboxUnacked)
	}
	snapTPM2, err := LoadCorrespondence(root, tpm, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapTPM2.OutboxAwaitingPeerAck) != 0 {
		t.Fatalf("outbox should clear after peer_ack: %+v", snapTPM2.OutboxAwaitingPeerAck)
	}
}

func TestCorrespondence_meshStatusDoesNotBlockOutbox(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-status",
	}); err != nil {
		t.Fatal(err)
	}
	agy := Seat{AgentID: "peer-agent-02", RoleHints: []string{"agy"}}
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ONLINE: working ATK-1",
		AgentID:     agy.AgentID,
		Sender:      FeedSenderMeshStatus,
		EventType:   FeedEventTypeMeshStatus,
		SelfACK:     false,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := LoadCorrespondence(root, agy, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.OutboxAwaitingPeerAck) != 0 {
		t.Fatalf("untargeted mesh_status must not await peer_ack: %+v", snap.OutboxAwaitingPeerAck)
	}
	if snap.NextActionHint != HintContinue {
		t.Fatalf("hint=%s want continue (no idle after emit-status)", snap.NextActionHint)
	}

	// Directed steer still awaits peer_ack.
	steer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN TPM: need next ATK",
		AgentID:     agy.AgentID,
		ToAgentID:   "peer-tpm-02",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	snap2, err := LoadCorrespondence(root, agy, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap2.OutboxAwaitingPeerAck) != 1 || snap2.OutboxAwaitingPeerAck[0].EventID != steer.EventID {
		t.Fatalf("directed steer should await peer_ack: %+v", snap2.OutboxAwaitingPeerAck)
	}
	if snap2.NextActionHint != HintAwaitPeerAckKeepWorking {
		t.Fatalf("hint=%s", snap2.NextActionHint)
	}
}

func TestLoadCorrespondence_requiresAgentID(t *testing.T) {
	snap, err := LoadCorrespondence(t.TempDir(), Seat{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SkipReason != "agent_id_required" {
		t.Fatalf("skip=%q", snap.SkipReason)
	}
}

func TestLoadCorrespondence_registeredCallbacks(t *testing.T) {
	root := t.TempDir()
	_, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "AFE-await-corr",
		FromAgentID: "peer-tpm-01",
		ToAgentID:   "peer-agent-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := LoadCorrespondence(root, Seat{AgentID: "peer-tpm-01"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.RegisteredCallbacks) != 1 || snap.RegisteredCallbacks[0].EventID != "AFE-await-corr" {
		t.Fatalf("registered_callbacks=%+v", snap.RegisteredCallbacks)
	}
	other, err := LoadCorrespondence(root, Seat{AgentID: "peer-agent-01"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.RegisteredCallbacks) != 0 {
		t.Fatalf("peer should not own TPM await: %+v", other.RegisteredCallbacks)
	}
}

func TestCorrespondence_AdditionalEventTypes(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-types",
	}); err != nil {
		t.Fatal(err)
	}

	types := []string{FeedEventTypeScanTests, FeedEventTypeCheckpoint, FeedEventTypePromote, FeedEventTypeWake}
	agy := Seat{AgentID: "peer-agent-03", RoleHints: []string{"agy"}}
	tpm := Seat{AgentID: "peer-tpm-02", RoleHints: []string{"tpm"}}

	for _, et := range types {
		ev, err := AppendEvent(AppendEventInput{
			ProjectRoot: root,
			Message:     "System event: " + et,
			AgentID:     "peer-tpm-02",
			ToAgentID:   "peer-agent-03",
			Sender:      "system",
			EventType:   et,
			SelfACK:     true,
		})
		if err != nil {
			t.Fatal(err)
		}

		snapAGY, err := LoadCorrespondence(root, agy, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapAGY.InboxUnacked) == 0 || snapAGY.InboxUnacked[len(snapAGY.InboxUnacked)-1].EventID != ev.EventID {
			t.Fatalf("agy inbox should have %s event", et)
		}

		snapTPM, err := LoadCorrespondence(root, tpm, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapTPM.OutboxAwaitingPeerAck) == 0 || snapTPM.OutboxAwaitingPeerAck[len(snapTPM.OutboxAwaitingPeerAck)-1].EventID != ev.EventID {
			t.Fatalf("tpm outbox should have %s event", et)
		}

		if _, err := AppendPeerAck(root, agy.AgentID, agy.PersonaRef, ev.EventID, "ACK "+et); err != nil {
			t.Fatal(err)
		}
	}

	snapAGY2, err := LoadCorrespondence(root, agy, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapAGY2.InboxUnacked) != 0 {
		t.Fatalf("inbox should be empty after peer_acks: %+v", snapAGY2.InboxUnacked)
	}

	snapTPM2, err := LoadCorrespondence(root, tpm, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapTPM2.OutboxAwaitingPeerAck) != 0 {
		t.Fatalf("outbox should be empty after peer_acks: %+v", snapTPM2.OutboxAwaitingPeerAck)
	}
}

func TestExpectsPeerAck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		eventType string
		to        string
		evTo      string
		want      bool
	}{
		{"steer directed", FeedEventTypeSteering, "agy-1", "", true},
		{"steer undirected", FeedEventTypeSteering, "", "", false},
		{"chat directed via map", FeedEventTypeChat, "", "agy-1", true},
		{"mesh undirected", FeedEventTypeMeshStatus, "", "", false},
		{"mesh directed", FeedEventTypeMeshStatus, "agy-1", "", true},
		{"scan_tests directed", FeedEventTypeScanTests, "agy-1", "", true},
		{"checkpoint directed", FeedEventTypeCheckpoint, "agy-1", "", true},
		{"promote directed", FeedEventTypePromote, "agy-1", "", true},
		{"wake directed", FeedEventTypeWake, "agy-1", "", true},
		{"ack never", FeedEventTypePeerAck, "agy-1", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := map[string]any{}
			if tc.evTo != "" {
				ev[JSONFieldToAgentID] = tc.evTo
			}
			if got := expectsPeerAck(ev, tc.eventType, tc.to); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestInferRoleHints_exactSeatIDOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-exact-hints",
	}); err != nil {
		t.Fatal(err)
	}
	vendorSteer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN AGY: COMMS-CHECK must not route by vendor nickname",
		AgentID:     "peer-operator-1",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	seat := Seat{AgentID: "peer-agent-1"}
	snap, err := LoadCorrespondence(root, seat, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.InboxUnacked) != 0 {
		t.Fatalf("opaque seat must not ingest ATTN AGY without RoleHints: %+v", snap.InboxUnacked)
	}

	idSteer, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN peer-agent-1: COMMS-CHECK CC-test",
		AgentID:     "peer-operator-1",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = vendorSteer
	snap, err = LoadCorrespondence(root, seat, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.InboxUnacked) != 1 || snap.InboxUnacked[0].EventID != idSteer.EventID {
		t.Fatalf("exact seat id ATTN inbox=%+v", snap.InboxUnacked)
	}
}

func TestCorrespondence_NoIdleOnEmptyInbox(t *testing.T) {
	// CAP POLICY PROBE: peer_wake_live=false is NOT a license to standby.
	// When inbox and outbox are empty, deriveHint must return HintContinue,
	// NOT standby. Workers must continue executing claimed ATK / next BLI.
	t.Parallel()
	hint := deriveHint(nil, nil)
	if hint != HintContinue {
		t.Fatalf("deriveHint(nil, nil) = %q, want %q (standby is forbidden)", hint, HintContinue)
	}
}

func TestLoadCorrespondence_WorktreeEnvBindsToSeatedRoot(t *testing.T) {
	studioRoot := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(studioRoot, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-test-seated",
	}); err != nil {
		t.Fatal(err)
	}

	event, err := AppendEvent(AppendEventInput{
		ProjectRoot: studioRoot,
		Message:     "ATTN peer-agent-01: important task",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-01",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Unbonded worktree (no settings) must be refused
	unbondedWorktree := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-unbonded")
	if err := fileutil.EnsureDir(unbondedWorktree); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(unbondedWorktree) }()

	_, err = LoadCorrespondence(unbondedWorktree, Seat{AgentID: "peer-agent-01"}, 10)
	if err == nil {
		t.Fatal("expected error loading correspondence from unbonded agent worktree, got nil")
	}

	_, err = AppendEvent(AppendEventInput{
		ProjectRoot: unbondedWorktree,
		Message:     "ATTN: fail",
		AgentID:     "peer-tpm-01",
	})
	if err == nil {
		t.Fatal("expected error appending event to unbonded agent worktree, got nil")
	}

	// 2. Bound worktree (with settings paths.project_root) must resolve to seated studio root
	boundWorktree := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-bound")
	if err := fileutil.EnsureDir(boundWorktree); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(boundWorktree) }()

	if err := paths.BootstrapWorktreeConfig(boundWorktree, studioRoot); err != nil {
		t.Fatal(err)
	}

	snap, err := LoadCorrespondence(boundWorktree, Seat{AgentID: "peer-agent-01"}, 10)
	if err != nil {
		t.Fatalf("unexpected error from bound worktree: %v", err)
	}
	if len(snap.InboxUnacked) != 1 || snap.InboxUnacked[0].EventID != event.EventID {
		t.Fatalf("expected 1 unacked event %s from studio root, got %+v", event.EventID, snap.InboxUnacked)
	}

	appRes, err := AppendEvent(AppendEventInput{
		ProjectRoot: boundWorktree,
		Message:     "ATTN peer-agent-01: appended via bound worktree",
		AgentID:     "peer-tpm-01",
		ToAgentID:   "peer-agent-01",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error appending via bound worktree: %v", err)
	}
	wantEventPath := datacell.EffectiveAgentChatChannelEventsJSONLPath(studioRoot, datacell.DefaultAgentChatChannelConfig())
	if appRes.EventPath != wantEventPath {
		t.Fatalf("expected event written to studio root %s, got %s", wantEventPath, appRes.EventPath)
	}
}
