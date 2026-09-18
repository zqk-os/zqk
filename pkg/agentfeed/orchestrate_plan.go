package agentfeed

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// OrchestratePlanPrefix is the first-line directive seat-workers execute
// without AgentX. Chat prose is not this protocol.
const OrchestratePlanPrefix = "ORCHESTRATE_PLAN "

// WorkerSeatForPersona returns the mesh seat that declared personaRef.
// Fail-closed when peer_seats has no matching persona_ref — do not
// invent ALPHA→antigravity-1 in this package.
func WorkerSeatForPersona(projectRoot, personaRef string) string {
	want := strings.TrimSpace(personaRef)
	if want == "" || strings.TrimSpace(projectRoot) == "" {
		return ""
	}
	f, err := LoadPeerSeats(projectRoot)
	if err != nil {
		return ""
	}
	for seatID, rec := range f.Seats {
		if strings.EqualFold(strings.TrimSpace(rec.PersonaRef), want) {
			return seatID
		}
	}
	return ""
}

// IsCoordinatorDutySeat is the TPM / operator seat that may auto-dispatch
// the lead plan. Plan primaries must not steal that gland.
// Truth is peer_seats duty (or wake membrane), never hardcoded agent/persona IDs.
func IsCoordinatorDutySeat(projectRoot, agentID, personaRef string) bool {
	_ = personaRef
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || strings.TrimSpace(projectRoot) == "" {
		return false
	}
	f, err := LoadPeerSeats(projectRoot)
	if err != nil {
		return false
	}
	rec, ok := f.Seats[agentID]
	if !ok {
		return false
	}
	if duty := NormalizeSeatDuty(rec.Duty); duty != "" {
		return duty == SeatKindCoordinator
	}
	// Infer only from the IDE/MCP membrane. stamp is shared by workers
	// (feed-only) and must not steal coordinator dispatch.
	return NormalizeWakeMembrane(rec.Wake) == WakeMembraneMCP
}

// IsOrchestratePlanBody reports whether body is a plan-dispatch directive.
func IsOrchestratePlanBody(body string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	return strings.HasPrefix(strings.TrimSpace(first), OrchestratePlanPrefix)
}

// ComposeOrchestratePlanMessage builds the fail-closed first line.
func ComposeOrchestratePlanMessage(planID, extra string) string {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return ""
	}
	msg := OrchestratePlanPrefix + planID
	if extra = strings.TrimSpace(extra); extra != "" {
		msg += "\n" + extra
	}
	return msg
}

// FirstPersonaRef returns the first persona_refs entry (plan owner).
func FirstPersonaRef(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	switch refs := obj[objects.FieldKeyPersonaRefs].(type) {
	case []string:
		if len(refs) > 0 {
			return strings.TrimSpace(refs[0])
		}
	case []any:
		for _, raw := range refs {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func itemBody(item CorrespondenceItem) string {
	body := strings.TrimSpace(item.Message)
	if body == "" {
		body = strings.TrimSpace(item.Summary)
	}
	return body
}

// CorrespondenceHasOrchestratePlan is true when an item already names planID.
func CorrespondenceHasOrchestratePlan(items []CorrespondenceItem, planID string) bool {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return false
	}
	needle := OrchestratePlanPrefix + planID
	for _, item := range items {
		body := itemBody(item)
		if !IsOrchestratePlanBody(body) {
			continue
		}
		first, _, _ := strings.Cut(body, "\n")
		if strings.HasPrefix(strings.TrimSpace(first), needle) {
			return true
		}
	}
	return false
}

// OrchestratePlanRemintCooldown is how long a coordinator waits after a
// successful dispatch before sending the same plan again. Prevents the
// 30s seat-worker poll from reminting once the peer has acked and is working.
const OrchestratePlanRemintCooldown = 15 * time.Minute

const orchestrateDutySchema = "zqk_orchestrate_duty_v1"

// DispatchOrchestratePlanInput is one hourglass plan handoff.
type DispatchOrchestratePlanInput struct {
	ProjectRoot    string
	FromAgentID    string
	ToAgentID      string
	PlanID         string
	Extra          string
	Now            time.Time
	RemintCooldown time.Duration
}

// DispatchOrchestratePlanResult is the appended event (empty EventID if skipped).
type DispatchOrchestratePlanResult struct {
	EventID string
	Skipped string
}

// DispatchOrchestratePlan appends ORCHESTRATE_PLAN once. Skips when the
// caller's outbox already awaits that plan (no remint).
func DispatchOrchestratePlan(in DispatchOrchestratePlanInput) (DispatchOrchestratePlanResult, error) {
	root := strings.TrimSpace(in.ProjectRoot)
	from := strings.TrimSpace(in.FromAgentID)
	to := strings.TrimSpace(in.ToAgentID)
	planID := strings.TrimSpace(in.PlanID)
	if root == "" || from == "" || to == "" || planID == "" {
		return DispatchOrchestratePlanResult{}, errfmt.Errorf("dispatch requires root, from, to, and plan id")
	}
	if from == to {
		return DispatchOrchestratePlanResult{Skipped: "same_seat"}, nil
	}
	snap, err := LoadCorrespondence(root, Seat{AgentID: from}, 100)
	if err != nil {
		return DispatchOrchestratePlanResult{}, err
	}
	if CorrespondenceHasOrchestratePlan(snap.OutboxAwaitingPeerAck, planID) {
		return DispatchOrchestratePlanResult{Skipped: "already_awaiting"}, nil
	}
	if CorrespondenceHasOrchestratePlan(snap.InboxUnacked, planID) {
		return DispatchOrchestratePlanResult{Skipped: "already_inbox"}, nil
	}
	peerSnap, perr := LoadCorrespondence(root, Seat{AgentID: to}, 100)
	if perr == nil && CorrespondenceHasOrchestratePlan(peerSnap.InboxUnacked, planID) {
		return DispatchOrchestratePlanResult{Skipped: "already_peer_inbox"}, nil
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cooldown := in.RemintCooldown
	if cooldown <= 0 {
		cooldown = OrchestratePlanRemintCooldown
	}
	if duty, ok := readOrchestrateDuty(root, from); ok && duty.PlanID == planID && duty.ToAgentID == to {
		if at, aerr := time.Parse(time.RFC3339, duty.DispatchedAt); aerr == nil && now.Sub(at) < cooldown {
			return DispatchOrchestratePlanResult{Skipped: "cooldown"}, nil
		}
	}
	msg := ComposeOrchestratePlanMessage(planID, in.Extra)
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     msg,
		AgentID:     from,
		ToAgentID:   to,
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		return DispatchOrchestratePlanResult{}, errfmt.Newf("dispatch orchestrate plan").Wrap(err)
	}
	if _, aerr := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     res.EventID,
		FromAgentID: from,
		ToAgentID:   to,
		Action:      AwaitActionWake,
		WakeMessage: PeerAckPasteStub(res.EventID),
	}); aerr != nil {
		return DispatchOrchestratePlanResult{EventID: res.EventID}, errfmt.Newf("dispatch register await").Wrap(aerr)
	}
	_ = writeOrchestrateDuty(root, orchestrateDuty{
		Schema:       orchestrateDutySchema,
		FromAgentID:  from,
		ToAgentID:    to,
		PlanID:       planID,
		EventID:      res.EventID,
		DispatchedAt: now.UTC().Format(time.RFC3339),
	})
	return DispatchOrchestratePlanResult{EventID: res.EventID}, nil
}

type orchestrateDuty struct {
	Schema       string `json:"schema"`
	FromAgentID  string `json:"from_agent_id"`
	ToAgentID    string `json:"to_agent_id"`
	PlanID       string `json:"plan_id"`
	EventID      string `json:"event_id"`
	DispatchedAt string `json:"dispatched_at"`
}

func orchestrateDutyPath(projectRoot, fromAgentID string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(fromAgentID), "/", "_")
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "mesh", "seat_workers", safe+".orchestrate-duty.json")
}

func readOrchestrateDuty(projectRoot, fromAgentID string) (orchestrateDuty, bool) {
	var duty orchestrateDuty
	raw, err := fileutil.ReadFile(orchestrateDutyPath(projectRoot, fromAgentID))
	if err != nil {
		return duty, false
	}
	if json.Unmarshal(raw, &duty) != nil || strings.TrimSpace(duty.PlanID) == "" {
		return duty, false
	}
	return duty, true
}

func writeOrchestrateDuty(projectRoot string, duty orchestrateDuty) error {
	path := orchestrateDutyPath(projectRoot, duty.FromAgentID)
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(duty, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteStandardFile(path, append(raw, '\n'))
}
