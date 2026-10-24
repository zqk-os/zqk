package agentfeed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupTestFeedRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := datacell.AgentChatChannelConfig{
		Enabled:      true,
		DeliveryMode: datacell.DeliveryModeNotify,
	}
	err := datacell.WriteAgentChatChannelConfig(dir, cfg)
	require.NoError(t, err)
	return dir
}

func TestCycle146_Append_EdgeCasesAndReceipts(t *testing.T) {
	root := setupTestFeedRoot(t)

	// AppendEvent: empty message
	_, err := AppendEvent(AppendEventInput{ProjectRoot: root, Message: ""})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required parameter")

	// AppendDeliveryReceipt: empty inReplyTo
	_, err = AppendDeliveryReceipt(root, "agent-1", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "in_reply_to is required")

	// AppendEvent: success with empty sender and agentID defaults
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "test message",
		AgentID:     "",
		Sender:      "",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.EventID)

	// AppendDeliveryReceipt: valid receipt with default summary
	recRes, err := AppendDeliveryReceipt(root, "agent-1", res.EventID, "")
	require.NoError(t, err)
	assert.NotEmpty(t, recRes.EventID)
	recMsg, _ := recRes.Event["message"].(string)
	assert.Contains(t, recMsg, "DELIVERY_RECEIPT")
}

func TestCycle146_Append_DeliveryModeOffAndDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := datacell.AgentChatChannelConfig{
		Enabled:      false,
		DeliveryMode: datacell.DeliveryModeOff,
	}
	err := datacell.WriteAgentChatChannelConfig(dir, cfg)
	require.NoError(t, err)

	// Disabled feed
	_, err = AppendEvent(AppendEventInput{
		ProjectRoot:      dir,
		Message:          "hello",
		SkipEnabledCheck: false,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")

	// SkipEnabledCheck bypasses enabled=false
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot:      dir,
		Message:          "hello bypass",
		SkipEnabledCheck: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.EventID)
}

func TestCycle146_AppendPeerAckWithSession_Validations(t *testing.T) {
	root := setupTestFeedRoot(t)

	// Empty inReplyTo
	_, err := AppendPeerAckWithSession(root, "agent-1", "persona-1", "session-1", "", "summary")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "in_reply_to is required")

	// Missing parent event authorization failure
	_, err = AppendPeerAckWithSession(root, "agent-1", "persona-1", "session-1", "EVT-NONEXISTENT", "summary")
	assert.Error(t, err)
}

func TestCycle146_AwaitPeerAck_LifecycleAndFire(t *testing.T) {
	root := setupTestFeedRoot(t)

	prevFire := PeerAckAwaitFire
	defer func() { PeerAckAwaitFire = prevFire }()
	var fired []PeerAckAwait
	PeerAckAwaitFire = func(a PeerAckAwait) error {
		fired = append(fired, a)
		return nil
	}

	// Register await
	registered, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "EVT-AWAIT-1",
		FromAgentID: "agent-caller",
		ToAgentID:   "agent-callee",
		Action:      AwaitActionWake,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, registered.ID)

	// List open awaits
	awaits, err := ListOpenPeerAckAwaits(root, "agent-caller")
	require.NoError(t, err)
	assert.Len(t, awaits, 1)
	assert.Equal(t, "EVT-AWAIT-1", awaits[0].EventID)

	// Filter for different caller should return empty
	otherAwaits, err := ListOpenPeerAckAwaits(root, "different-caller")
	require.NoError(t, err)
	assert.Empty(t, otherAwaits)

	// Complete awaits
	completed, err := CompletePeerAckAwaits(root, "EVT-AWAIT-1", "agent-callee")
	require.NoError(t, err)
	assert.Len(t, completed, 1)
	assert.Len(t, fired, 1)

	// List after completion
	remaining, err := ListOpenPeerAckAwaits(root, "agent-caller")
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestCycle146_AwaitPeerAck_ExpireAndDefaultFire(t *testing.T) {
	root := setupTestFeedRoot(t)

	// Register await
	_, err := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     "EVT-EXPIRED-1",
		FromAgentID: "caller",
		ToAgentID:   "callee",
		Action:      AwaitActionWake,
	})
	require.NoError(t, err)

	// Expire with zero duration (expires immediately)
	expired, err := ExpirePeerAckAwaits(root, 0)
	require.NoError(t, err)
	assert.Len(t, expired, 1)

	// Test defaultPeerAckAwaitFire with unknown action
	unknownAwait := PeerAckAwait{
		EventID: "EVT-UNKNOWN",
		Action:  "custom_unknown_action",
	}
	fireErr := defaultPeerAckAwaitFire(unknownAwait)
	assert.NoError(t, fireErr)
}

func TestCycle146_WakeAdapter_RegistrationAndCurrent(t *testing.T) {
	prev := CurrentPeerWakeAdapter()
	defer SetPeerWakeAdapter(prev)

	// Set nil resets to default
	SetPeerWakeAdapter(nil)
	assert.NotNil(t, CurrentPeerWakeAdapter())

	// SeatKind classification
	kind := SeatKindForAgent("")
	assert.Equal(t, SeatKindWorker, kind)

	kindCoord := SeatKindForAgent("coordinator")
	assert.NotEmpty(t, kindCoord)

	kindIn := SeatKindForAgentIn("", "tpm-agent")
	assert.NotEmpty(t, kindIn)
}

func TestCycle146_ShellPeerWakeAdapter_MissingScriptAndFallback(t *testing.T) {
	dir := t.TempDir()

	// Missing script without fallback
	adapter := &ShellPeerWakeAdapter{
		WorkerScript: "nonexistent-script.sh",
		Fallback:     nil,
	}
	res, err := adapter.Wake(context.Background(), PeerWakeRequest{
		ProjectRoot: dir,
		ToAgentID:   "worker-1",
		SeatKind:    SeatKindWorker,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wake membrane missing")
	assert.False(t, res.Live)

	// Missing script with fallback
	mockFallback := &stubWakeAdapter{
		result: PeerWakeAdapterResult{Live: true, Transport: "mock-fallback"},
	}
	adapterWithFallback := NewShellPeerWakeAdapterWithFallback(mockFallback)
	fallbackRes, err := adapterWithFallback.Wake(context.Background(), PeerWakeRequest{
		ProjectRoot: dir,
		ToAgentID:   "worker-1",
		SeatKind:    SeatKindWorker,
	})
	require.NoError(t, err)
	assert.True(t, fallbackRes.Live)
	assert.Equal(t, "mock-fallback", fallbackRes.Transport)
}

type stubWakeAdapter struct {
	result PeerWakeAdapterResult
	err    error
}

func (s *stubWakeAdapter) Wake(ctx context.Context, req PeerWakeRequest) (PeerWakeAdapterResult, error) {
	return s.result, s.err
}

func TestCycle146_ShellPeerWakeAdapter_Execution(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := paths.ScriptsDirPath(dir)
	err := fileutil.EnsureDir(scriptsDir)
	require.NoError(t, err)

	scriptPath := filepath.Join(scriptsDir, "test-wake.sh")
	scriptContent := "#!/bin/sh\necho 'wake executed successfully'\nexit 0\n"
	err = fileutil.WriteStandardFile(scriptPath, []byte(scriptContent))
	require.NoError(t, err)
	err = os.Chmod(scriptPath, 0755)
	require.NoError(t, err)

	adapter := &ShellPeerWakeAdapter{
		WorkerScript: "test-wake.sh",
		Fallback:     nil,
	}
	res, err := adapter.Wake(context.Background(), PeerWakeRequest{
		ProjectRoot:  dir,
		ToAgentID:    "worker-1",
		SeatKind:     SeatKindWorker,
		DeliveryMode: datacell.DeliveryModeNotify,
	})
	require.NoError(t, err)
	assert.True(t, res.Live)
	assert.Contains(t, res.Output, "wake executed successfully")
}

func TestCycle146_InFlightState_ReprimeAndSteering(t *testing.T) {
	dir := t.TempDir()

	// Empty agent ID
	_, err := LoadAgentInFlightState(dir, "")
	assert.Error(t, err)

	err = SaveAgentInFlightState(dir, AgentInFlightState{})
	assert.Error(t, err)

	// Non-existent state returns default aligned state
	state, err := LoadAgentInFlightState(dir, "agent-xyz")
	require.NoError(t, err)
	assert.Equal(t, "agent-xyz", state.AgentID)
	assert.Equal(t, InFlightStatusAligned, state.Status)

	// Record in-flight steering
	steering := InFlightSteering{
		EventID:     "EVT-STEER-1",
		Message:     "Focus on testing",
		FromAgentID: "tpm",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	err = RecordInFlightSteering(dir, "agent-xyz", steering)
	require.NoError(t, err)

	// Load and verify steering persisted
	updated, err := LoadAgentInFlightState(dir, "agent-xyz")
	require.NoError(t, err)
	assert.Equal(t, InFlightStatusPending, updated.Status)
	assert.Len(t, updated.ActiveGuidance, 1)

	// Format guidance
	formatted := FormatInFlightGuidance(updated.ActiveGuidance)
	assert.Contains(t, formatted, "Focus on testing")
}

func TestCycle146_VendorPathPolicy_LeaksAndIssues(t *testing.T) {
	policy := CurrentVendorPathPolicy()

	// FindLeak on clean string
	marker, found := policy.FindLeak("/usr/local/bin/zqk")
	assert.False(t, found)
	assert.Empty(t, marker)

	// FindLeak on vendor path (.gemini is a default marker)
	marker, found = policy.FindLeak("/home/user/.gemini/workspaces/app")
	assert.True(t, found)
	assert.Equal(t, ".gemini", marker)

	// LeakIssues reporting
	issues := policy.LeakIssues("/clean/root", "/user/.gemini/feed.jsonl", "safe-policy")
	assert.Len(t, issues, 1)
	assert.Contains(t, issues[0], "vendor_path_leak")

	// Set custom policy
	SetVendorPathPolicy(VendorPathPolicy{
		ForbiddenMarkers: []string{".custom_vendor/"},
	})
	defer SetVendorPathPolicy(VendorPathPolicy{})

	customPolicy := CurrentVendorPathPolicy()
	_, customFound := customPolicy.FindLeak("path/with/.custom_vendor/leak")
	assert.True(t, customFound)
}

func TestCycle146_TPMComposer_InjectAndHourglass(t *testing.T) {
	// PeerAck is ack theatre
	peerAckItem := CorrespondenceItem{
		EventID:   "EVT-ACK-1",
		EventType: FeedEventTypePeerAck,
		Message:   "PEER_ACK — received EVT-0",
	}
	assert.True(t, IsAckTheatreInboxItem(peerAckItem))
	assert.False(t, InboxRequiresHourglass(peerAckItem))

	// Normal work item is NOT ack theatre
	workItem := CorrespondenceItem{
		EventID:     "EVT-WORK-1",
		EventType:   FeedEventTypeChat,
		FromAgentID: "worker-1",
		Message:     "Completed component review",
	}
	assert.False(t, IsAckTheatreInboxItem(workItem))

	// Hourglass item naming ATK
	atkItem := CorrespondenceItem{
		EventID:     "EVT-ATK-1",
		EventType:   FeedEventTypeChat,
		FromAgentID: "worker-1",
		Message:     "Execution ready for ATK-1792790000000000000-abcd1234",
	}
	assert.True(t, InboxRequiresHourglass(atkItem))

	// FirstComposerHourglassItem
	inbox := []CorrespondenceItem{peerAckItem, workItem}
	item, ok := FirstComposerHourglassItem(inbox)
	assert.True(t, ok)
	assert.Equal(t, "EVT-WORK-1", item.EventID)

	// TPMComposerAttnCue
	cue := TPMComposerAttnCue(workItem, "tpm-lead")
	assert.Contains(t, cue, "ATTN TPM inbox")
	assert.Contains(t, cue, "tpm-lead")
}

func TestCycle146_UnrepairedWake_DetectionAndTruncate(t *testing.T) {
	// Error wake is unrepaired
	errWake := PeerWakeResult{
		Attempted: true,
		Error:     "connection refused",
	}
	isUnrep, reason, detail := IsUnrepairedWake(errWake)
	assert.True(t, isUnrep)
	assert.Equal(t, UnrepairedWakeFailed, reason)
	assert.Equal(t, "connection refused", detail)

	// Non-live wake is unrepaired
	stampWake := PeerWakeResult{
		Attempted: true,
		Live:      false,
		Transport: "stamp-only",
	}
	isUnrep, reason, detail = IsUnrepairedWake(stampWake)
	assert.True(t, isUnrep)
	assert.Equal(t, UnrepairedWakeStampNotLive, reason)
	assert.Equal(t, "stamp-only", detail)

	// Live successful wake
	liveWake := PeerWakeResult{
		Attempted: true,
		Live:      true,
		Transport: "native",
	}
	isUnrep, liveReason, liveDetail := IsUnrepairedWake(liveWake)
	assert.False(t, isUnrep)
	assert.Empty(t, liveReason)
	assert.Empty(t, liveDetail)

	// truncateRunes test
	assert.Equal(t, "hello", truncateRunes("hello", 10))
	assert.Equal(t, "hel…", truncateRunes("hello world", 3))
}

func TestCycle146_SeatWorker_AliveAndAttempts(t *testing.T) {
	dir := t.TempDir()

	// Initial alive is false
	alive, _, err := SeatWorkerAlive(dir, "worker-1", 5*time.Second)
	require.NoError(t, err)
	assert.False(t, alive)

	// Write alive
	_, err = WriteSeatWorkerAlive(dir, "worker-1", "sess-1", time.Now().UTC())
	require.NoError(t, err)

	alive, _, err = SeatWorkerAlive(dir, "worker-1", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, alive)

	// Event attempt count
	count := SeatEventAttemptCount(dir, "worker-1", "EVT-TEST-1")
	assert.Equal(t, 0, count)

	_, err = RecordSeatEventAttempt(dir, "worker-1", "EVT-TEST-1")
	require.NoError(t, err)

	countAfter := SeatEventAttemptCount(dir, "worker-1", "EVT-TEST-1")
	assert.Equal(t, 1, countAfter)

	// Clear attempts
	err = ClearSeatEventAttempts(dir, "worker-1", "EVT-TEST-1")
	require.NoError(t, err)
	assert.Equal(t, 0, SeatEventAttemptCount(dir, "worker-1", "EVT-TEST-1"))
}

func TestCycle146_PeerSeats_LivenessAndMembrane(t *testing.T) {
	// PID liveness
	assert.False(t, IsPIDAlive(-1))
	assert.False(t, IsPIDAlive(0))
	assert.True(t, IsPIDAlive(os.Getpid()))

	// NormalizeWakeMembrane
	assert.Equal(t, WakeMembraneAgentAPI, NormalizeWakeMembrane("agentapi"))
	assert.Equal(t, WakeMembraneMCP, NormalizeWakeMembrane("mcp"))
	assert.Equal(t, WakeMembraneStamp, NormalizeWakeMembrane("stamp"))
	assert.Equal(t, "", NormalizeWakeMembrane("unknown_value"))
}

func TestCycle146_PeerSeats_SaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()

	// Empty project root
	err := SavePeerSeats("", PeerSeatsFile{})
	assert.Error(t, err)

	seats := PeerSeatsFile{
		SchemaVersion: "1.0",
		Seats: map[string]PeerSeatRecord{
			"worker-alpha": {
				PersonaRef: "PER-COMMUNITY-SOFTWARE-ENGINEER",
				Duty:       SeatKindWorker,
				Wake:       WakeMembraneAgentAPI,
			},
		},
	}
	err = SavePeerSeats(dir, seats)
	require.NoError(t, err)

	loaded, err := LoadPeerSeats(dir)
	require.NoError(t, err)
	assert.Len(t, loaded.Seats, 1)
	assert.Equal(t, "PER-COMMUNITY-SOFTWARE-ENGINEER", loaded.Seats["worker-alpha"].PersonaRef)
}

func TestCycle146_Doctor_InspectFeed(t *testing.T) {
	dir := setupTestFeedRoot(t)

	// Write an event so feedPath is non-empty
	_, err := AppendEvent(AppendEventInput{
		ProjectRoot: dir,
		Message:     "init event",
	})
	require.NoError(t, err)

	// Inspect clean feed
	report := InspectFeed(DoctorOptions{
		ProjectRoot:    dir,
		MCPSubscribers: 1,
	})
	assert.True(t, report.PeerWakeLive)
	assert.Equal(t, 1, report.MCPSubscribers)
	assert.Empty(t, report.Issues)
}

func TestCycle146_PrioritizeSeatInbox_Ordering(t *testing.T) {
	items := []CorrespondenceItem{
		{EventID: "EVT-REGULAR", Message: "just regular update"},
		{EventID: "EVT-ORCH", Message: "ORCHESTRATE_PLAN PRI-1\nnext line"},
		{EventID: "EVT-COMMS", Message: "COMMS-CHECK CC-12345 nonce test"},
	}

	ordered := prioritizeSeatInbox(items)
	require.Len(t, ordered, 3)
	// Comms check is first
	assert.Equal(t, "EVT-COMMS", ordered[0].EventID)
	// Orchestrate plan is second
	assert.Equal(t, "EVT-ORCH", ordered[1].EventID)
	// Regular is third
	assert.Equal(t, "EVT-REGULAR", ordered[2].EventID)
}

func TestCycle146_PeerAckAuth_Matrix(t *testing.T) {
	root := setupTestFeedRoot(t)

	// Append initial event so feed file exists
	_, initErr := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "feed initialized",
	})
	require.NoError(t, initErr)

	// AuthorizePeerAck: validation errors
	err := AuthorizePeerAck(root, "", "EVT-1")
	assert.Error(t, err)

	err = AuthorizePeerAck(root, "worker-1", "")
	assert.Error(t, err)

	// Unknown parent: returns nil (fail-open for legacy events)
	err = AuthorizePeerAck(root, "worker-1", "EVT-UNKNOWN")
	assert.NoError(t, err)

	// Append directed parent event
	parentRes, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "directed steer",
		AgentID:     "coord-1",
		ToAgentID:   "worker-target",
	})
	require.NoError(t, err)

	// Denied when ack stamped by wrong agent
	err = AuthorizePeerAck(root, "worker-impostor", parentRes.EventID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPeerAckSeatMismatch)

	// Allowed when ack stamped by addressed agent
	err = AuthorizePeerAck(root, "worker-target", parentRes.EventID)
	assert.NoError(t, err)

	// PeerAckSatisfiesParent matrix
	parentMap := map[string]any{"to_agent_id": "worker-target"}
	assert.False(t, PeerAckSatisfiesParent(parentMap, ""))
	assert.False(t, PeerAckSatisfiesParent(parentMap, FeedKernelAgentID))
	assert.True(t, PeerAckSatisfiesParent(map[string]any{}, "any-worker"))
	assert.True(t, PeerAckSatisfiesParent(parentMap, "worker-target"))
	assert.False(t, PeerAckSatisfiesParent(parentMap, "other-worker"))

	// LookupFeedEvent with empty ID
	foundObj, ok, err := LookupFeedEvent(root, "")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, foundObj)
}

func TestCycle146_DispatchOrchestratePlan_Branches(t *testing.T) {
	root := setupTestFeedRoot(t)

	// Missing parameters
	_, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root,
	})
	assert.Error(t, err)

	// Same seat returns skipped same_seat
	resSame, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root,
		FromAgentID: "seat-a",
		ToAgentID:   "seat-a",
		PlanID:      "PRI-123",
	})
	require.NoError(t, err)
	assert.Equal(t, "same_seat", resSame.Skipped)

	// First successful dispatch
	resFirst, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root,
		FromAgentID: "coord-tpm",
		ToAgentID:   "worker-dev",
		PlanID:      "PRI-123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resFirst.EventID)

	// Second dispatch immediately returns already_awaiting
	resSecond, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root,
		FromAgentID: "coord-tpm",
		ToAgentID:   "worker-dev",
		PlanID:      "PRI-123",
	})
	require.NoError(t, err)
	assert.Equal(t, "already_awaiting", resSecond.Skipped)
}

func TestCycle146_SeatWorker_ProcessSeatInbox_Advanced(t *testing.T) {
	root := setupTestFeedRoot(t)

	// Missing project root or agent ID
	_, err := ProcessSeatInbox(context.Background(), SeatWorkerOptions{})
	assert.Error(t, err)

	// Append a directed COMMS-CHECK event
	commsRes, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "COMMS-CHECK CC-99999 challenge",
		AgentID:     "peer-coord",
		ToAgentID:   "worker-target",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, commsRes.EventID)

	// Append a plan directive event
	planRes, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "ORCHESTRATE_PLAN PRI-CYCLE-146\nExecute testing",
		AgentID:     "peer-coord",
		ToAgentID:   "worker-target",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, planRes.EventID)

	nonCommsHandled := false
	opts := SeatWorkerOptions{
		ProjectRoot: root,
		AgentID:     "worker-target",
		PersonaRef:  "PER-COMMUNITY-SOFTWARE-ENGINEER",
		OnNonComms: func(ctx context.Context, item CorrespondenceItem, body string) error {
			nonCommsHandled = true
			return nil
		},
	}

	result, err := ProcessSeatInbox(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, nonCommsHandled)
	assert.Equal(t, 1, result.ProcessedCOMMS)
}

func TestCycle146_Wake_DeliverAndPasteStubs(t *testing.T) {
	// ShouldWakePeer
	assert.True(t, ShouldWakePeer(datacell.DeliveryModeNotify))
	assert.True(t, ShouldWakePeer(datacell.DeliveryModePaste))
	assert.False(t, ShouldWakePeer(datacell.DeliveryModeOff))
	assert.False(t, ShouldWakePeer(datacell.DeliveryModeLog))

	// Paste stub formatting
	stub := PeerAckPasteStub("AFE-12345")
	assert.Contains(t, strings.ToUpper(stub), "PEER_ACK")
	assert.Contains(t, stub, "AFE-12345")

	stubComms, ok := commsCheckWakeStub("", "worker", "EVT-1", "worker-1", "COMMS-CHECK CC-555 nonce")
	assert.True(t, ok)
	assert.Contains(t, stubComms, "COMMS-CHECK")
	assert.Contains(t, stubComms, "CC-555")

	// WakePeerOpts with self wake and empty message
	dir := t.TempDir()
	resSelf := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot: dir,
		Message:     "wake payload",
		ToAgentID:   "same-agent",
		FromAgentID: "same-agent",
	})
	assert.False(t, resSelf.Attempted)
	assert.Equal(t, "self_wake", resSelf.SkippedReason)

	resEmpty := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot: dir,
		Message:     "",
	})
	assert.False(t, resEmpty.Attempted)
	assert.Equal(t, "empty_message", resEmpty.Skipped)
}

func TestCycle146_SeatWorker_HandleNonComms_Branches(t *testing.T) {
	ctx := context.Background()

	// 1. IsOrchestratePlanBody with nil OnNonComms -> skipped
	item := CorrespondenceItem{EventID: "EVT-1"}
	out := SeatWorkerResult{}
	orchHandled := false
	agentXStarted := false
	handleNonCommsItem(ctx, SeatWorkerOptions{}, item, "ORCHESTRATE_PLAN PRI-1", false, &orchHandled, &agentXStarted, &out)
	assert.Equal(t, 1, out.Skipped)
	assert.False(t, orchHandled)

	// 2. IsOrchestratePlanBody with OnNonComms error
	errOpts := SeatWorkerOptions{
		OnNonComms: func(ctx context.Context, item CorrespondenceItem, body string) error {
			return assert.AnError
		},
	}
	out = SeatWorkerResult{}
	handleNonCommsItem(ctx, errOpts, item, "ORCHESTRATE_PLAN PRI-1", false, &orchHandled, &agentXStarted, &out)
	assert.Len(t, out.Errors, 1)

	// 3. inboxHasOrch with orchHandled=false -> skipped
	out = SeatWorkerResult{}
	orchHandled = false
	handleNonCommsItem(ctx, SeatWorkerOptions{}, item, "regular task", true, &orchHandled, &agentXStarted, &out)
	assert.Equal(t, 1, out.Skipped)

	// 4. Named ATK with OnNamedATK error
	out = SeatWorkerResult{}
	agentXStarted = false
	atkOpts := SeatWorkerOptions{
		OnNamedATK: func(ctx context.Context, item CorrespondenceItem, body string) error {
			return assert.AnError
		},
	}
	handleNonCommsItem(ctx, atkOpts, item, "ATK-1792790000000000000-abcd1234", false, &orchHandled, &agentXStarted, &out)
	assert.Len(t, out.Errors, 1)
	assert.True(t, agentXStarted)

	// 5. Named ATK when agentXStarted is already true -> skipped
	out = SeatWorkerResult{}
	handleNonCommsItem(ctx, atkOpts, item, "ATK-1792790000000000000-abcd1234", false, &orchHandled, &agentXStarted, &out)
	assert.Equal(t, 1, out.Skipped)
}

func TestCycle146_PeerAckAwait_Storage_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	// Load non-existent file
	f, err := loadPeerAckAwaitFile(dir)
	require.NoError(t, err)
	assert.Empty(t, f.Awaits)

	// Save file
	saveData := peerAckAwaitFile{
		Awaits: []PeerAckAwait{
			{EventID: "EVT-1", Status: AwaitStatusOpen},
		},
	}
	err = savePeerAckAwaitFile(dir, saveData)
	require.NoError(t, err)

	// Load existing file
	loaded, err := loadPeerAckAwaitFile(dir)
	require.NoError(t, err)
	assert.Len(t, loaded.Awaits, 1)

	// Load invalid JSON
	storePath := peerAckAwaitStorePath(dir)
	err = fileutil.WriteStandardFile(storePath, []byte("invalid json content"))
	require.NoError(t, err)

	_, err = loadPeerAckAwaitFile(dir)
	assert.Error(t, err)

	// loadLockedPeerAckAwaitFile on invalid JSON
	_, unlock, err := loadLockedPeerAckAwaitFile(dir)
	assert.Error(t, err)
	if unlock != nil {
		unlock()
	}

	// Load empty file
	err = fileutil.WriteStandardFile(storePath, []byte("   \n"))
	require.NoError(t, err)

	emptyLoaded, err := loadPeerAckAwaitFile(dir)
	require.NoError(t, err)
	assert.Empty(t, emptyLoaded.Awaits)
}

func TestCycle146_PeerSeats_RefreshLivePIDs_Mocked(t *testing.T) {
	dir := t.TempDir()

	prevFn := listPeerExecutablePIDsFn
	defer func() { listPeerExecutablePIDsFn = prevFn }()

	myPID := os.Getpid()
	listPeerExecutablePIDsFn = func() ([]int, error) {
		return []int{myPID}, nil
	}

	// 1. Dry run on empty seats file auto-seeds default seats and assigns live PID
	res, err := RefreshPeerSeatsFromLivePIDs(dir, true)
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.NotEmpty(t, res.LivePIDs)
	assert.NotEmpty(t, res.Assigned)

	// 2. Non-dry run writes seats to disk
	resWrite, err := RefreshPeerSeatsFromLivePIDs(dir, false)
	require.NoError(t, err)
	assert.False(t, resWrite.DryRun)
	assert.NotEmpty(t, resWrite.Assigned)

	// 3. Load seats and verify assigned PID
	loaded, err := LoadPeerSeats(dir)
	require.NoError(t, err)
	foundPID := false
	for _, rec := range loaded.Seats {
		if rec.PID == myPID {
			foundPID = true
			break
		}
	}
	assert.True(t, foundPID)

	// 4. Test ResolvePeerPID with assigned seat
	firstSeatID := ""
	for id, rec := range loaded.Seats {
		if rec.PID == myPID {
			firstSeatID = id
			break
		}
	}
	resolvedPID, err := ResolvePeerPID(dir, firstSeatID)
	require.NoError(t, err)
	assert.Equal(t, myPID, resolvedPID)
}
