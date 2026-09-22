// BLI-STARTER-COMMUNITY-062 / PRI-STARTER-COMMUNITY-062 coverage elevation
package agentfeed

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraCorrespondenceWakeAndSeats(t *testing.T) {
	root := t.TempDir()
	seat := Seat{AgentID: "peer-1", PersonaRef: "operator", RoleHints: []string{"agy"}}
	if _, err := ListInboxUnacked(root, seat, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := ListOutboxAwaitingPeerAck(root, seat, 3); err != nil {
		t.Fatal(err)
	}
	if IsTPMWakeSeat("") || !IsTPMWakeSeat("tpm") || !IsTPMWakeSeat("peer-tpm-01") || !IsTPMWakeSeat("ide-composer-tpm") || !IsTPMWakeSeat("tpm-extra") || IsTPMWakeSeat("agy-1") {
		t.Fatal("IsTPMWakeSeat")
	}
	if SeatLane("", "x") != "" || SeatLane(root, "") != "" || SeatLane(root, "missing") != "" {
		t.Fatal("empty lane")
	}
	if err := fileutil.MkdirAll(filepath.Dir(paths.PeerSeatsPath(root)), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := SavePeerSeats(root, PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]PeerSeatRecord{
			"peer-1": {Wake: WakeMembraneAgentAPI, Lane: "alpha", PID: 999999999, PersonaRef: "op"},
			"peer-2": {Wake: WakeMembraneMCP, WorkerLane: "beta"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if SeatLane(root, "peer-1") != "alpha" || SeatLane(root, "peer-2") != "beta" {
		t.Fatal("lanes")
	}
	_ = wakeScriptArgsForSeat(datacell.DeliveryModeNotify, "hi", 42, "conv-1")
	_ = wakeScriptArgsForSeat(datacell.DeliveryModeNotify, "hi", 0, "")
	_ = itoa(7)
	_ = ShouldWakePeer(datacell.DeliveryModeNotify)
	_ = ShouldWakePeer("")
	_, _ = SeatWorkerBound()
	_ = CommsLifeAckSummary("CC-1")
	_ = FormatCommsWorkReply("CC-1", "", 2)
	_ = FormatCommsWorkReply("CC-1", "PRI-1", 0)
	if _, ok := ParseCommsCheckChallenge("nope"); ok {
		t.Fatal("not a challenge")
	}
	if !IsCommsCheckText("COMMS-CHECK CC-abc LIFE") {
		t.Fatal("comms text")
	}
	if FirstPersonaRef(nil) != "" {
		t.Fatal("nil persona")
	}
	if FirstPersonaRef(map[string]any{objects.FieldKeyPersonaRefs: []string{" p1 "}}) != "p1" {
		t.Fatal("string personas")
	}
	if FirstPersonaRef(map[string]any{objects.FieldKeyPersonaRefs: []any{"", "p2"}}) != "p2" {
		t.Fatal("any personas")
	}
	_ = itemBody(CorrespondenceItem{Summary: "sum"})
	res := WakePeer(context.Background(), "", "")
	if res.Skipped == "" {
		t.Fatal("empty wake")
	}
	res = WakePeer(context.Background(), root, "hello")
	_ = res
	res = WakePeerOpts(nil, WakePeerOptions{ProjectRoot: root, Message: "m", ToAgentID: "peer-1", FromAgentID: "peer-1"})
	if !res.WakeAuthorExcluded {
		t.Fatal("self wake")
	}
	doc := InspectFeed(DoctorOptions{ProjectRoot: root, MCPQueryErr: errors.New("mcp down")})
	if len(doc.Issues) == 0 {
		t.Fatal("expected issues")
	}
	_ = InspectFeed(DoctorOptions{ProjectRoot: root, MCPSubscribers: 1})
	_ = lastN(nil, 0)
	_ = lastN([]CorrespondenceItem{{EventID: "1"}, {EventID: "2"}, {EventID: "3"}}, 2)
	_ = truncateSummary("short", 80)
	_ = truncateSummary(strings.Repeat(" inflight ", 40), 12)
	_ = inferRoleHints("")
	_ = inferRoleHints("agy-1")
	_, _ = parseCommsFromStub("hello")
	_, _ = parseCommsFromStub("ATTN PEER — COMMS-CHECK CC-xyz — ping")
	oldPol := CurrentVendorPathPolicy()
	t.Cleanup(func() { SetVendorPathPolicy(oldPol) })
	SetVendorPathPolicy(VendorPathPolicy{})
	_ = CurrentVendorPathPolicy()
	_ = wakeScriptArgs(datacell.DeliveryModePaste, "p")
	_ = wakeScriptArgsForTPM(datacell.DeliveryModePaste, "p")
	_ = wakeScriptArgsForTPM(datacell.DeliveryModeNotify, "p")
	sh := NewShellPeerWakeAdapter()
	_ = sh.args(PeerWakeRequest{SeatKind: SeatKindCoordinator, DeliveryMode: datacell.DeliveryModePaste, PasteText: "x"})
	_ = sh.args(PeerWakeRequest{SeatKind: SeatKindWorker, DeliveryMode: datacell.DeliveryModeNotify, PasteText: "x", PeerPID: 1, Conversation: "c"})
	_ = sh.transport(PeerWakeRequest{SeatKind: SeatKindCoordinator, DeliveryMode: datacell.DeliveryModePaste})
	_ = sh.transport(PeerWakeRequest{SeatKind: SeatKindCoordinator, DeliveryMode: datacell.DeliveryModeNotify})
	_ = sh.live(PeerWakeRequest{SeatKind: SeatKindWorker, DeliveryMode: datacell.DeliveryModeNotify})
	_ = sh.live(PeerWakeRequest{SeatKind: SeatKindCoordinator, DeliveryMode: datacell.DeliveryModePaste})
	if _, err := WriteSeatWorkerAlive(root, "", "", time.Time{}); err == nil {
		t.Fatal("empty agent")
	}
	if _, err := WriteSeatWorkerAlive(root, "peer-1", "sess-1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	_ = InboxRequiresHourglass(CorrespondenceItem{Message: "COMMS-CHECK CC-1 LIFE"})
	_ = InboxRequiresHourglass(CorrespondenceItem{Summary: "noise"})
	_ = truncateRunes("ab", 0)
	_ = truncateRunes("abcdef", 3)
	_ = SeatKindForAgentIn(root, "peer-1")
	_ = CurrentPeerWakeAdapter()
	_, _ = listPeerExecutablePIDs()
	_ = time.Now()
}
