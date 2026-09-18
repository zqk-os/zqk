package agentfeed

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestParseCommsCheckChallenge(t *testing.T) {
	t.Parallel()
	chal, ok := ParseCommsCheckChallenge("COMMS-CHECK CC-20260814T120000Z-1: ack and work")
	if !ok || chal.Nonce != "CC-20260814T120000Z-1" {
		t.Fatalf("got %+v ok=%v", chal, ok)
	}
	if _, ok := ParseCommsCheckChallenge("ATTN PEER — wake"); ok {
		t.Fatal("expected non-COMMS")
	}
}

func TestProcessSeatInbox_commsLifeAndWork(t *testing.T) {
	root := t.TempDir()
	// Minimal lite config + channel path so AppendEvent works.
	mustWriteLiteFeed(t, root)

	const seat = "peer-agent-1"
	const workerSession = "ZQK-TEST-WORKER-SESSION-001"
	nonce := "CC-20260814T160000Z-42"
	challenge := "COMMS-CHECK " + nonce + ": life and work required"
	chalRes, err := AppendEvent(AppendEventInput{
		ProjectRoot:      root,
		Message:          challenge,
		AgentID:          "cursor-composer",
		ToAgentID:        seat,
		Sender:           FeedSenderHumanSteer,
		EventType:        FeedEventTypeSteering,
		SelfACK:          true,
		SkipEnabledCheck: true,
	})
	if err != nil {
		t.Fatalf("append challenge: %v", err)
	}

	res, err := ProcessSeatInbox(context.Background(), SeatWorkerOptions{
		ProjectRoot: root,
		AgentID:     seat,
		PersonaRef:  "PER-ORCH-ALPHA",
		SessionID:   workerSession,
		PriorityID:  "PRI-TEST",
		SkipWake:    true,
		Now:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ProcessSeatInbox: %v", err)
	}
	if res.ProcessedCOMMS != 1 {
		t.Fatalf("processed=%d errors=%v", res.ProcessedCOMMS, res.Errors)
	}
	if res.AlivePath == "" {
		t.Fatal("expected alive path")
	}
	alive, _, aerr := SeatWorkerAlive(root, seat, time.Minute)
	if aerr != nil || !alive {
		t.Fatalf("alive=%v err=%v", alive, aerr)
	}
	rawAlive, readErr := fileutil.ReadFile(res.AlivePath)
	if readErr != nil {
		t.Fatalf("read alive: %v", readErr)
	}
	if !strings.Contains(string(rawAlive), workerSession) {
		t.Fatalf("alive heartbeat missing session_id %q: %s", workerSession, rawAlive)
	}

	// LIFE ack + WORK reply should clear challenge from inbox.
	snap, err := LoadCorrespondence(root, Seat{AgentID: seat}, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snap.InboxUnacked {
		if item.EventID == chalRes.EventID {
			t.Fatalf("challenge still unacked: %+v", item)
		}
	}

	// Coordinator should see WORK in its inbox.
	snapTPM, err := LoadCorrespondence(root, Seat{AgentID: "cursor-composer"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	foundWork := false
	foundLife := false
	for _, item := range snapTPM.InboxUnacked {
		body := item.Message
		if body == "" {
			body = item.Summary
		}
		if strings.Contains(body, CommsCheckReplyPrefix) &&
			strings.Contains(body, nonce) &&
			strings.Contains(body, CommsWorkMarker) &&
			strings.Contains(body, "priority=PRI-TEST") {
			foundWork = true
			break
		}
	}
	if !foundWork {
		t.Fatalf("WORK reply missing in TPM inbox: %+v", snapTPM.InboxUnacked)
	}

	eventsPath := datacell.AgentChatChannelEventsJSONLPath(root)
	f, err := fileutil.Open(eventsPath)
	if err != nil {
		t.Fatalf("open events: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		sid, _ := m[objects.FieldKeySessionID].(string)
		et, _ := m[objects.FieldKeyEventType].(string)
		ir, _ := m[JSONFieldInReplyTo].(string)
		if et == FeedEventTypePeerAck && ir == chalRes.EventID && sid == workerSession {
			foundLife = true
		}
		if et == FeedEventTypeSteering && sid == workerSession &&
			strings.Contains(fmtString(m[JSONFieldMessage]), CommsWorkMarker) {
			foundWork = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan events: %v", err)
	}
	if !foundLife {
		t.Fatalf("LIFE peer_ack missing session_id %q on feed", workerSession)
	}
	if !foundWork {
		t.Fatalf("WORK steering missing session_id %q on feed", workerSession)
	}
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
}

func TestSeatWorkerAlive_failClosedWhenMissing(t *testing.T) {
	t.Parallel()
	ok, _, err := SeatWorkerAlive(t.TempDir(), "peer-agent-1", time.Minute)
	if err != nil || ok {
		t.Fatalf("expected missing heartbeat fail-closed ok=%v err=%v", ok, err)
	}
}

func TestSeatEventAttemptLedger(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const seat = "peer-agent-1"
	const event = "AFE-1786944399124957000-537bd4b7"

	for want := 1; want <= 3; want++ {
		got, err := RecordSeatEventAttempt(root, seat, event)
		if err != nil {
			t.Fatalf("RecordSeatEventAttempt: %v", err)
		}
		if got != want {
			t.Fatalf("attempt count = %d, want %d", got, want)
		}
	}

	// A second event must count independently, so one poisoned steer cannot
	// park unrelated work.
	other, err := RecordSeatEventAttempt(root, seat, "AFE-other")
	if err != nil {
		t.Fatalf("RecordSeatEventAttempt other: %v", err)
	}
	if other != 1 {
		t.Fatalf("other event count = %d, want 1", other)
	}

	if err := ClearSeatEventAttempts(root, seat, event); err != nil {
		t.Fatalf("ClearSeatEventAttempts: %v", err)
	}
	reset, err := RecordSeatEventAttempt(root, seat, event)
	if err != nil {
		t.Fatalf("RecordSeatEventAttempt after clear: %v", err)
	}
	if reset != 1 {
		t.Fatalf("count after clear = %d, want 1", reset)
	}
}

func TestProcessSeatInbox_namedATKWithoutOnNonComms(t *testing.T) {
	root := t.TempDir()
	mustWriteLiteFeed(t, root)
	const seat = "peer-agent-1"
	body := "COMMS+WORK ND-1: Execute ONLY ATK-1787738919414925000-41b3c9d7. Do NOT execute ATK-1787739837478620000-7db4746e."
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root, Message: body, AgentID: "cursor-composer",
		ToAgentID: seat, Sender: FeedSenderHumanSteer, EventType: FeedEventTypeSteering,
		SelfACK: true, SkipEnabledCheck: true,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	var got string
	res, err := ProcessSeatInbox(context.Background(), SeatWorkerOptions{
		ProjectRoot: root,
		AgentID:     seat,
		PersonaRef:  "PER-ORCH-ALPHA",
		SkipWake:    true,
		Now:         time.Now().UTC(),
		OnNamedATK: func(_ context.Context, _ CorrespondenceItem, b string) error {
			got = NamedATKID(b)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ProcessSeatInbox: %v", err)
	}
	if res.Skipped != 0 {
		t.Fatalf("skipped=%d want 0 errors=%v", res.Skipped, res.Errors)
	}
	if got != "ATK-1787738919414925000-41b3c9d7" {
		t.Fatalf("named=%q", got)
	}
}

func TestSeatAttemptKey_prefersEventOverATK(t *testing.T) {
	t.Parallel()
	if got := SeatAttemptKey("AFE-1", "ATK-9"); got != "AFE-1" {
		t.Fatalf("key=%q want event", got)
	}
	if got := SeatAttemptKey("  ", "ATK-9"); got != "ATK-9" {
		t.Fatalf("key=%q want ATK fallback", got)
	}
}

func TestSeatEventAttemptLedger_newAFEDoesNotInheritATKBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const seat = "peer-agent-1"
	const atk = "ATK-1787738929491339000-57449893"
	for i := 0; i < 3; i++ {
		if _, err := RecordSeatEventAttempt(root, seat, SeatAttemptKey("AFE-old", atk)); err != nil {
			t.Fatal(err)
		}
	}
	if got := SeatEventAttemptCount(root, seat, SeatAttemptKey("AFE-new", atk)); got != 0 {
		t.Fatalf("new AFE inherited ATK budget: count=%d", got)
	}
}

func TestSeatEventAttemptCount(t *testing.T) {
	root := t.TempDir()
	const seat = "peer-agent-1"
	const ev = "AFE-exhausted"
	if got := SeatEventAttemptCount(root, seat, ev); got != 0 {
		t.Fatalf("empty ledger count=%d", got)
	}
	if _, err := RecordSeatEventAttempt(root, seat, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordSeatEventAttempt(root, seat, ev); err != nil {
		t.Fatal(err)
	}
	if got := SeatEventAttemptCount(root, seat, ev); got != 2 {
		t.Fatalf("count=%d want 2", got)
	}
	if got := SeatEventAttemptCount(root, seat, ev); got != 2 {
		t.Fatalf("peek mutated ledger to %d", got)
	}
}

func TestPrioritizeSeatInbox_newestNamedATKFirst(t *testing.T) {
	t.Parallel()
	got := prioritizeSeatInbox([]CorrespondenceItem{
		{EventID: "AFE-old", Timestamp: "2026-08-26T19:00:00Z", Message: "Execute ONLY ATK-1787738919414925000-41b3c9d7"},
		{EventID: "AFE-new", Timestamp: "2026-08-26T20:08:00Z", Message: "Execute ONLY ATK-1787738919414925000-41b3c9d7"},
		{EventID: "AFE-attn", Timestamp: "2026-08-26T20:09:00Z", Message: "ATTN peer: no ATK"},
	})
	if len(got) != 3 || got[0].EventID != "AFE-new" || got[1].EventID != "AFE-old" || got[2].EventID != "AFE-attn" {
		t.Fatalf("order=%v", []string{got[0].EventID, got[1].EventID, got[2].EventID})
	}
}

func TestPrioritizeSeatInbox_orchestrateBeforeAttn(t *testing.T) {
	t.Parallel()
	got := prioritizeSeatInbox([]CorrespondenceItem{
		{EventID: "AFE-attn", Message: "ATTN antigravity-1: R19 is NOT done"},
		{EventID: "AFE-orch", Message: ComposeOrchestratePlanMessage("PRI-CEF-R20-BRANCH-PROVENANCE-001", "ACK pile")},
		{EventID: "AFE-comms", Message: "COMMS-CHECK CC-20260826T010000Z-1: ack"},
	})
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].EventID != "AFE-comms" || got[1].EventID != "AFE-orch" || got[2].EventID != "AFE-attn" {
		t.Fatalf("order=%v %v %v", got[0].EventID, got[1].EventID, got[2].EventID)
	}
}

func TestProcessSeatInbox_orchestratePlanBeforeAttn(t *testing.T) {
	root := t.TempDir()
	mustWriteLiteFeed(t, root)
	const seat = "peer-agent-1"
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root, Message: "ATTN peer: stale wake", AgentID: "cursor-composer",
		ToAgentID: seat, Sender: FeedSenderHumanSteer, EventType: FeedEventTypeSteering,
		SelfACK: true, SkipEnabledCheck: true,
	}); err != nil {
		t.Fatalf("append attn: %v", err)
	}
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root, Message: "ORCHESTRATE_PLAN PRI-CEF-R20-BRANCH-PROVENANCE-001\nstart",
		AgentID: "cursor-composer", ToAgentID: seat, Sender: FeedSenderHumanSteer,
		EventType: FeedEventTypeSteering, SelfACK: true, SkipEnabledCheck: true,
	}); err != nil {
		t.Fatalf("append orch: %v", err)
	}
	var firstBody string
	var n int
	_, err := ProcessSeatInbox(context.Background(), SeatWorkerOptions{
		ProjectRoot: root,
		AgentID:     seat,
		PersonaRef:  "PER-ORCH-ALPHA",
		SkipWake:    true,
		Now:         time.Now().UTC(),
		OnNonComms: func(_ context.Context, _ CorrespondenceItem, body string) error {
			n++
			if firstBody == "" {
				firstBody = body
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ProcessSeatInbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("handled %d items, want 1 (ORCHESTRATE_PLAN only; ATTN parked)", n)
	}
	if !IsOrchestratePlanBody(firstBody) {
		t.Fatalf("first handled %q, want ORCHESTRATE_PLAN", firstBody)
	}
	snap, err := LoadCorrespondence(root, Seat{AgentID: seat, PersonaRef: "PER-ORCH-ALPHA"}, 20)
	if err != nil {
		t.Fatalf("LoadCorrespondence: %v", err)
	}
	for _, item := range snap.InboxUnacked {
		body := item.Message
		if body == "" {
			body = item.Summary
		}
		if strings.HasPrefix(strings.TrimSpace(body), "ATTN ") {
			t.Fatalf("stale ATTN still unacked: %s", item.EventID)
		}
	}
}

func TestProcessSeatInbox_oneAgentXPerPass(t *testing.T) {
	root := t.TempDir()
	mustWriteLiteFeed(t, root)
	const seat = "peer-agent-1"
	for _, msg := range []string{"ATTN peer: first stale", "ATTN peer: second stale"} {
		if _, err := AppendEvent(AppendEventInput{
			ProjectRoot: root, Message: msg, AgentID: "cursor-composer",
			ToAgentID: seat, Sender: FeedSenderHumanSteer, EventType: FeedEventTypeSteering,
			SelfACK: true, SkipEnabledCheck: true,
		}); err != nil {
			t.Fatalf("append %q: %v", msg, err)
		}
	}
	var handled []string
	_, err := ProcessSeatInbox(context.Background(), SeatWorkerOptions{
		ProjectRoot: root,
		AgentID:     seat,
		PersonaRef:  "PER-ORCH-ALPHA",
		SkipWake:    true,
		Now:         time.Now().UTC(),
		OnNonComms: func(_ context.Context, item CorrespondenceItem, _ string) error {
			handled = append(handled, item.EventID)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ProcessSeatInbox: %v", err)
	}
	if len(handled) != 1 {
		t.Fatalf("handled %d AgentX items, want 1 per pass: %v", len(handled), handled)
	}
}

func TestSeatEventAttemptLedger_requiresEventID(t *testing.T) {
	t.Parallel()
	if _, err := RecordSeatEventAttempt(t.TempDir(), "peer-agent-1", "  "); err == nil {
		t.Fatal("expected error for blank event id")
	}
}

func mustWriteLiteFeed(t *testing.T, root string) {
	t.Helper()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-seat-worker-test",
	}); err != nil {
		t.Fatal(err)
	}
}
