package agentfeed

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestWorkerSeatForPersona(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := SavePeerSeats(root, PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]PeerSeatRecord{
			"worker-alpha": {Wake: WakeMembraneAgentAPI, PersonaRef: objects.ConstPersonaOrchestratorAlpha},
			"worker-beta":  {Wake: WakeMembraneAgentAPI, PersonaRef: objects.ConstPersonaOrchestratorBeta},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := WorkerSeatForPersona(root, objects.ConstPersonaOrchestratorAlpha); got != "worker-alpha" {
		t.Fatalf("alpha=%q", got)
	}
	if got := WorkerSeatForPersona(root, objects.ConstPersonaOrchestratorBeta); got != "worker-beta" {
		t.Fatalf("beta=%q", got)
	}
	if WorkerSeatForPersona(root, objects.ConstPersonaDefaultOperator) != "" {
		t.Fatal("undeclared persona must not invent a seat")
	}
	if WorkerSeatForPersona("", objects.ConstPersonaOrchestratorAlpha) != "" {
		t.Fatal("empty root")
	}
}

func TestIsCoordinatorDutySeat(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := SavePeerSeats(root, PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]PeerSeatRecord{
			"ide-seat":    {Wake: WakeMembraneMCP, Duty: SeatKindCoordinator},
			"worker-seat": {Wake: WakeMembraneAgentAPI, Duty: SeatKindWorker},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !IsCoordinatorDutySeat(root, "ide-seat", "") {
		t.Fatal("declared coordinator duty must dispatch")
	}
	if IsCoordinatorDutySeat(root, "worker-seat", "") {
		t.Fatal("plan primary must not auto-dispatch")
	}
	inferRoot := t.TempDir()
	if err := SavePeerSeats(inferRoot, PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]PeerSeatRecord{
			"ide-mcp":     {Wake: WakeMembraneMCP},
			"stamp-seat":  {Wake: WakeMembraneStamp},
			"agentapi-ok": {Wake: WakeMembraneAgentAPI},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !IsCoordinatorDutySeat(inferRoot, "ide-mcp", "") {
		t.Fatal("wake=mcp without duty is the IDE coordinator")
	}
	if IsCoordinatorDutySeat(inferRoot, "stamp-seat", "") {
		t.Fatal("wake=stamp without duty is not coordinator")
	}
	if IsCoordinatorDutySeat(inferRoot, "agentapi-ok", "") {
		t.Fatal("wake=agentapi without duty is not coordinator")
	}
	if IsCoordinatorDutySeat(root, "undeclared-seat", "") {
		t.Fatal("missing seat must fail closed")
	}
	if IsCoordinatorDutySeat("", "ide-seat", "") {
		t.Fatal("empty root must fail closed")
	}
}

func TestCorrespondenceHasOrchestratePlan(t *testing.T) {
	t.Parallel()
	items := []CorrespondenceItem{
		{Message: "ATTN stale"},
		{Message: ComposeOrchestratePlanMessage("PRI-CEF-R20-BRANCH-PROVENANCE-001", "go")},
	}
	if !CorrespondenceHasOrchestratePlan(items, "PRI-CEF-R20-BRANCH-PROVENANCE-001") {
		t.Fatal("expected hit")
	}
	if CorrespondenceHasOrchestratePlan(items, "PRI-CEF-R13-TEST-ISOLATION-001") {
		t.Fatal("other plan")
	}
}

func TestFirstPersonaRef(t *testing.T) {
	t.Parallel()
	if got := FirstPersonaRef(map[string]any{objects.FieldKeyPersonaRefs: []any{"PER-ORCH-ALPHA"}}); got != "PER-ORCH-ALPHA" {
		t.Fatalf("got %q", got)
	}
}

func TestDispatchOrchestratePlan_skipsPeerInboxAndCooldown(t *testing.T) {
	root := t.TempDir()
	mustWriteLiteFeed(t, root)
	if _, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     ComposeOrchestratePlanMessage("PRI-LEAD", "peer already has it"),
		AgentID:     "other-seat",
		ToAgentID:   "antigravity-1",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	}); err != nil {
		t.Fatal(err)
	}
	peer, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root, FromAgentID: "cursor-composer", ToAgentID: "antigravity-1", PlanID: "PRI-LEAD",
	})
	if err != nil {
		t.Fatal(err)
	}
	if peer.Skipped != "already_peer_inbox" {
		t.Fatalf("peer inbox: %+v", peer)
	}

	root2 := t.TempDir()
	mustWriteLiteFeed(t, root2)
	now := time.Now().UTC()
	if err := writeOrchestrateDuty(root2, orchestrateDuty{
		Schema: orchestrateDutySchema, FromAgentID: "cursor-composer", ToAgentID: "antigravity-1",
		PlanID: "PRI-LEAD", EventID: "AFE-OLD", DispatchedAt: now.Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	cool, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root2, FromAgentID: "cursor-composer", ToAgentID: "antigravity-1",
		PlanID: "PRI-LEAD", Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cool.Skipped != "cooldown" {
		t.Fatalf("cooldown: %+v", cool)
	}
}

func TestDispatchOrchestratePlan_skipsRemint(t *testing.T) {
	root := t.TempDir()
	mustWriteLiteFeed(t, root)
	first, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root,
		FromAgentID: "cursor-composer",
		ToAgentID:   "antigravity-1",
		PlanID:      "PRI-LEAD",
		Extra:       "self-serve whats-next",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.EventID == "" || first.Skipped != "" {
		t.Fatalf("first=%+v", first)
	}
	second, err := DispatchOrchestratePlan(DispatchOrchestratePlanInput{
		ProjectRoot: root,
		FromAgentID: "cursor-composer",
		ToAgentID:   "antigravity-1",
		PlanID:      "PRI-LEAD",
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Skipped != "already_awaiting" || second.EventID != "" {
		t.Fatalf("remint not skipped: %+v", second)
	}
}
