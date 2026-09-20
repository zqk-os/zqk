package agentfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// SeatWorkerAliveSchema marks heartbeat files written by zqk agent seat-worker.
	SeatWorkerAliveSchema = "zqk_seat_worker_alive_v1"
)

// SeatWorkerOptions configures a single ProcessSeatInbox pass.
type SeatWorkerOptions struct {
	ProjectRoot string
	AgentID     string
	PersonaRef  string
	// ReplyToAgentID overrides challenge FromAgentID for WORK replies.
	ReplyToAgentID string
	// SessionID is the worker zqk_session binding runtime provenance for this process.
	// TRACK: REQ-COMMS-RUNTIME-SESSION-001
	SessionID string
	// PriorityID is embedded in COMMS-CHECK-REPLY WORK (from whats-next).
	PriorityID string
	// InboxCount is embedded in WORK reply (remaining unacked after life ack).
	InboxCount int
	// SkipWake suppresses agentapi/MCP doorbell after WORK append (tests).
	SkipWake bool
	// Now overrides time for heartbeat (tests).
	Now time.Time
	// OnNonComms, when set, is invoked for each non-COMMS directed inbox item
	// (AgentX / agent execute path). When nil, non-COMMS items are left unacked
	// unless OnNamedATK handles a single-ATK hourglass.
	OnNonComms NonCommsHandler
	// OnNamedATK runs when the body names exactly one live ATK (or ONLY ATK-…).
	// Wired even when --execute-non-comms is off so hourglass work is not skipped.
	// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001
	OnNamedATK NonCommsHandler
}

// SeatWorkerResult summarizes one inbox processing pass.
type SeatWorkerResult struct {
	ProcessedCOMMS int      `json:"processed_comms"`
	Skipped        int      `json:"skipped"`
	NonComms       []string `json:"non_comms_event_ids,omitempty"`
	EventIDs       []string `json:"event_ids,omitempty"`
	Errors         []string `json:"errors,omitempty"`
	AlivePath      string   `json:"alive_path,omitempty"`
}

// NonCommsHandler optionally handles directed non-COMMS inbox items (AgentX / execute).
// Return nil to leave the item unacked for a human or later pass.
type NonCommsHandler func(ctx context.Context, item CorrespondenceItem, body string) error

// SeatWorkerAlivePath returns the heartbeat file for a seat.
func SeatWorkerAlivePath(projectRoot, agentID string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(agentID), "/", "_")
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "mesh", "seat_workers", safe+".alive.json")
}

// WriteSeatWorkerAlive records that a seat worker is executing for agentID.
// sessionID is the worker zqk_session when known (CRIT-COMMS-RUNTIME-SESSION-FEED-LINK-001).
func WriteSeatWorkerAlive(projectRoot, agentID, sessionID string, now time.Time) (string, error) {
	if strings.TrimSpace(agentID) == "" {
		return "", errfmt.Errorf("agent_id required for seat-worker heartbeat")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	path := SeatWorkerAlivePath(projectRoot, agentID)
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return "", errfmt.Errorf("mkdir seat_workers: %w", err)
	}
	payload := map[string]any{
		"schema":                  SeatWorkerAliveSchema,
		objects.FieldKeyAgentID:   strings.TrimSpace(agentID),
		objects.FieldKeyUpdatedAt: now.UTC().Format(time.RFC3339),
		"pid":                     os.Getpid(),
	}
	if sid := strings.TrimSpace(sessionID); sid != "" {
		payload[objects.FieldKeySessionID] = sid
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := fileutil.WriteStandardFile(path, append(raw, '\n')); err != nil {
		return "", errfmt.Errorf("write seat-worker alive: %w", err)
	}
	return path, nil
}

// SeatWorkerAttemptsPath returns the per-seat ledger of failed AgentX attempts.
func SeatWorkerAttemptsPath(projectRoot, agentID string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(agentID), "/", "_")
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "mesh", "seat_workers", safe+".attempts.json")
}

func readSeatEventAttempts(path string) map[string]int {
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		return map[string]int{}
	}
	ledger := map[string]int{}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return map[string]int{}
	}
	return ledger
}

func writeSeatEventAttempts(path string, ledger map[string]int) error {
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return errfmt.Errorf("mkdir seat_workers: %w", err)
	}
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteStandardFile(path, append(raw, '\n'))
}

// SeatAttemptKey is the ledger key for a seat-worker AgentX budget.
// Prefer the feed event so a planner re-steer after recycle or a new
// binary can run. ATK-only keys made assigned work unrecoverable once
// three earlier hourglasses failed (new AFEs parked at attempts=3).
// Reminting the same AFE is still forbidden (do not remint live orch).
// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001 — remove the event-first
// preference when: remint automation is gone and ATK close is honest.
func SeatAttemptKey(eventID, taskID string) string {
	if id := strings.TrimSpace(eventID); id != "" {
		return id
	}
	return strings.TrimSpace(taskID)
}

// RecordSeatEventAttempt increments and returns the failed-attempt count for one
// ledger key (ATK id or feed event). Without this ledger a run that always
// fails is re-polled forever, so the seat never reaches the next inbox item.
func RecordSeatEventAttempt(projectRoot, agentID, eventID string) (int, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return 0, errfmt.Errorf("event_id required for seat-worker attempt ledger")
	}
	path := SeatWorkerAttemptsPath(projectRoot, agentID)
	ledger := readSeatEventAttempts(path)
	ledger[eventID]++
	return ledger[eventID], writeSeatEventAttempts(path, ledger)
}

// SeatEventAttemptCount returns the failed-attempt count without incrementing.
func SeatEventAttemptCount(projectRoot, agentID, eventID string) int {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return 0
	}
	return readSeatEventAttempts(SeatWorkerAttemptsPath(projectRoot, agentID))[eventID]
}

// ClearSeatEventAttempts drops the ledger entry once an event is resolved.
func ClearSeatEventAttempts(projectRoot, agentID, eventID string) error {
	eventID = strings.TrimSpace(eventID)
	path := SeatWorkerAttemptsPath(projectRoot, agentID)
	ledger := readSeatEventAttempts(path)
	if _, ok := ledger[eventID]; !ok {
		return nil
	}
	delete(ledger, eventID)
	return writeSeatEventAttempts(path, ledger)
}

// SeatWorkerAlive reports whether a recent heartbeat exists (fail-closed gate).
func SeatWorkerAlive(projectRoot, agentID string, maxAge time.Duration) (bool, time.Time, error) {
	path := SeatWorkerAlivePath(projectRoot, agentID)
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}
	var payload struct {
		UpdatedAt string `json:"updated_at"`
		AgentID   string `json:"agent_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, time.Time{}, err
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.UpdatedAt))
	if err != nil {
		return false, time.Time{}, err
	}
	if maxAge <= 0 {
		maxAge = 2 * time.Minute
	}
	ok := time.Since(ts) <= maxAge && strings.TrimSpace(payload.AgentID) == strings.TrimSpace(agentID)
	return ok, ts, nil
}

// ProcessSeatInbox handles directed inbox items for a seat: deterministic COMMS
// life∧work first; non-COMMS items are left unacked (AgentX / human).
func ProcessSeatInbox(ctx context.Context, opts SeatWorkerOptions) (SeatWorkerResult, error) {
	out := SeatWorkerResult{}
	root := strings.TrimSpace(opts.ProjectRoot)
	agentID := strings.TrimSpace(opts.AgentID)
	persona := strings.TrimSpace(opts.PersonaRef)
	if root == "" || agentID == "" {
		return out, errfmt.Errorf("project root and agent-id are required")
	}
	if persona == "" {
		persona = objects.ConstPersonaDefaultOperator
	}

	alivePath, aerr := WriteSeatWorkerAlive(root, agentID, opts.SessionID, opts.Now)
	if aerr != nil {
		return out, aerr
	}
	out.AlivePath = alivePath

	snap, err := LoadCorrespondence(root, Seat{AgentID: agentID, PersonaRef: persona}, 100)
	if err != nil {
		return out, errfmt.Errorf("load correspondence: %w", err)
	}

	inboxCount := opts.InboxCount
	if inboxCount <= 0 {
		inboxCount = len(snap.InboxUnacked)
	}

	// COMMS then ORCHESTRATE_PLAN then other non-comms. FIFO AgentX on a
	// superseded ATTN pile starves the plan directive for tens of minutes
	// (10m timeout × 3 attempts per stale AFE). When a plan directive is
	// present and handled, park remaining ATTN — do not start AgentX in
	// the same pass. Without a plan line, run at most one AgentX item so
	// the next poll can still promote a newly arrived ORCHESTRATE_PLAN.
	ordered := prioritizeSeatInbox(snap.InboxUnacked)
	inboxHasOrch := inboxHasOrchestratePlan(ordered)
	orchHandled := false
	agentXStarted := false
	for _, item := range ordered {
		body := strings.TrimSpace(item.Message)
		if body == "" {
			body = strings.TrimSpace(item.Summary)
		}
		chal, ok := ParseCommsCheckChallenge(body)
		if !ok {
			if !IsCommsCheckText(body) {
				handleNonCommsItem(ctx, opts, item, body, inboxHasOrch, &orchHandled, &agentXStarted, &out)
				if agentXStarted && !inboxHasOrch {
					break
				}
				continue
			}
			chal, ok = parseCommsFromStub(body)
			if !ok {
				out.NonComms = append(out.NonComms, item.EventID)
				out.Skipped++
				continue
			}
		}
		chal.EventID = item.EventID
		chal.FromSeat = strings.TrimSpace(item.FromAgentID)
		if err := handleCommsChallenge(ctx, opts, chal, inboxCount); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", item.EventID, err))
			continue
		}
		out.ProcessedCOMMS++
		out.EventIDs = append(out.EventIDs, item.EventID)
		if inboxCount > 0 {
			inboxCount--
		}
	}
	return out, nil
}

func inboxHasOrchestratePlan(items []CorrespondenceItem) bool {
	for _, item := range items {
		if IsOrchestratePlanBody(itemBody(item)) {
			return true
		}
	}
	return false
}

func handleNonCommsItem(
	ctx context.Context,
	opts SeatWorkerOptions,
	item CorrespondenceItem,
	body string,
	inboxHasOrch bool,
	orchHandled *bool,
	agentXStarted *bool,
	out *SeatWorkerResult,
) {
	out.NonComms = append(out.NonComms, item.EventID)
	if IsOrchestratePlanBody(body) {
		if opts.OnNonComms == nil {
			out.Skipped++
			return
		}
		if err := opts.OnNonComms(ctx, item, body); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: non-comms: %v", item.EventID, err))
			return
		}
		*orchHandled = true
		return
	}
	if inboxHasOrch {
		if orchHandled != nil && *orchHandled {
			if err := parkSupersededNonComms(opts, item); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: supersede: %v", item.EventID, err))
				return
			}
			out.EventIDs = append(out.EventIDs, item.EventID)
			return
		}
		// Plan line still unhandled — leave ATTN for the next pass.
		out.Skipped++
		return
	}
	if atk := NamedATKID(body); atk != "" && opts.OnNamedATK != nil {
		if agentXStarted != nil && *agentXStarted {
			out.Skipped++
			return
		}
		if agentXStarted != nil {
			*agentXStarted = true
		}
		if err := opts.OnNamedATK(ctx, item, body); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: named-atk %s: %v", item.EventID, atk, err))
			return
		}
		out.EventIDs = append(out.EventIDs, item.EventID)
		return
	}
	if opts.OnNonComms == nil || (agentXStarted != nil && *agentXStarted) {
		out.Skipped++
		return
	}
	*agentXStarted = true
	if err := opts.OnNonComms(ctx, item, body); err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("%s: non-comms: %v", item.EventID, err))
	}
}

func parkSupersededNonComms(opts SeatWorkerOptions, item CorrespondenceItem) error {
	persona := strings.TrimSpace(opts.PersonaRef)
	if persona == "" {
		persona = objects.ConstPersonaDefaultOperator
	}
	summary := fmt.Sprintf("SEAT_WORKER_SUPERSEDED %s — ORCHESTRATE_PLAN present; ATTN parked", item.EventID)
	_, err := AppendPeerAckWithSession(opts.ProjectRoot, opts.AgentID, persona, opts.SessionID, item.EventID, summary)
	if err != nil {
		return errfmt.Errorf("supersede ack: %w", err)
	}
	return nil
}

func prioritizeSeatInbox(items []CorrespondenceItem) []CorrespondenceItem {
	if len(items) < 2 {
		return items
	}
	comms := make([]CorrespondenceItem, 0, len(items))
	orch := make([]CorrespondenceItem, 0, len(items))
	named := make([]CorrespondenceItem, 0, len(items))
	rest := make([]CorrespondenceItem, 0, len(items))
	for _, item := range items {
		body := itemBody(item)
		switch {
		case IsCommsCheckText(body):
			comms = append(comms, item)
		case IsOrchestratePlanBody(body):
			orch = append(orch, item)
		case NamedATKID(body) != "":
			named = append(named, item)
		default:
			rest = append(rest, item)
		}
	}
	// Newest one-ATK hourglass first. FIFO on a stale ATTN pile starves the
	// live assigned ATK for a full AgentX budget per leftover event.
	sort.SliceStable(named, func(i, j int) bool {
		return named[i].Timestamp > named[j].Timestamp
	})
	out := make([]CorrespondenceItem, 0, len(items))
	out = append(out, comms...)
	out = append(out, orch...)
	out = append(out, named...)
	out = append(out, rest...)
	return out
}

func parseCommsFromStub(body string) (CommsCheckChallenge, bool) {
	fields := strings.Fields(body)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == CommsCheckPrefix && strings.HasPrefix(strings.TrimRight(fields[i+1], ":,;"), CommsNoncePrefix) {
			return CommsCheckChallenge{Nonce: strings.TrimRight(fields[i+1], ":,;"), Message: body}, true
		}
	}
	return CommsCheckChallenge{}, false
}

func handleCommsChallenge(ctx context.Context, opts SeatWorkerOptions, chal CommsCheckChallenge, inboxCount int) error {
	root := strings.TrimSpace(opts.ProjectRoot)
	agentID := strings.TrimSpace(opts.AgentID)
	persona := strings.TrimSpace(opts.PersonaRef)
	if persona == "" {
		persona = objects.ConstPersonaDefaultOperator
	}
	if chal.EventID == "" {
		return errfmt.Errorf("COMMS challenge missing event_id")
	}

	lifeSummary := CommsLifeAckSummary(chal.Nonce)
	if _, err := AppendPeerAckWithSession(root, agentID, persona, opts.SessionID, chal.EventID, lifeSummary); err != nil {
		return errfmt.Errorf("COMMS life ack: %w", err)
	}

	replyTo := strings.TrimSpace(opts.ReplyToAgentID)
	if replyTo == "" {
		replyTo = strings.TrimSpace(chal.FromSeat)
	}
	if replyTo == "" {
		replyTo = CoordinatorSeatID(root)
	}
	if replyTo == "" {
		return errfmt.Errorf("COMMS work reply: no coordinator seat in peer_seats (duty=coordinator or wake=mcp)")
	}

	workMsg := FormatCommsWorkReply(chal.Nonce, opts.PriorityID, inboxCount)
	if err := EnforceDirectedHourglass(replyTo, true); err != nil {
		return err
	}
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     workMsg,
		AgentID:     agentID,
		PersonaRef:  persona,
		SessionID:   opts.SessionID,
		ToAgentID:   replyTo,
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		InReplyTo:   chal.EventID,
		SelfACK:     true,
	})
	if err != nil {
		return errfmt.Errorf("COMMS work reply: %w", err)
	}
	if _, aerr := RegisterPeerAckAwait(root, PeerAckAwaitInput{
		EventID:     res.EventID,
		FromAgentID: agentID,
		ToAgentID:   replyTo,
		Action:      AwaitActionWake,
		WakeMessage: PeerAckPasteStub(res.EventID),
	}); aerr != nil {
		return errfmt.Errorf("COMMS work await: %w", aerr)
	}
	if opts.SkipWake {
		return nil
	}
	_ = WakePeerOpts(ctx, WakePeerOptions{
		ProjectRoot: root,
		Message:     workMsg,
		ToAgentID:   replyTo,
		InReplyTo:   res.EventID,
	})
	return nil
}
