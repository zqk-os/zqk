package studio

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/tde"
)

// StagedEnvelopeItem summarizes a staged TDE envelope for UI inbox inspection.
type StagedEnvelopeItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	TargetID  string `json:"target_id"`
	Operation string `json:"operation"`
	ExecuteAt string `json:"execute_at"`
	CreatedAt string `json:"created_at"`
	Status    string `json:"status"`
}

// InboxPayload represents the consolidated inbox inspection response.
type InboxPayload struct {
	Status                string                         `json:"status"`
	AgentID               string                         `json:"agent_id"`
	PersonaID             string                         `json:"persona_id,omitempty"`
	InboxUnacked          []agentfeed.CorrespondenceItem `json:"inbox_unacked"`
	OutboxAwaitingPeerAck []agentfeed.CorrespondenceItem `json:"outbox_awaiting_peer_ack"`
	StagedEnvelopes       []StagedEnvelopeItem           `json:"staged_envelopes"`
	TotalUnacked          int                            `json:"total_unacked"`
	Count                 int                            `json:"count"`
	NextActionHint        string                         `json:"next_action_hint,omitempty"`
}

// InboxAckRequest represents an acknowledgment request.
type InboxAckRequest struct {
	InReplyTo  string `json:"in_reply_to"`
	AgentID    string `json:"agent_id"`
	PersonaRef string `json:"persona_ref"`
	Summary    string `json:"summary"`
	EnvelopeID string `json:"envelope_id,omitempty"`
}

// InboxRespondRequest represents a response dispatch request.
type InboxRespondRequest struct {
	ToAgentID    string `json:"to_agent_id"`
	Message      string `json:"message"`
	AgentID      string `json:"agent_id,omitempty"`
	InReplyTo    string `json:"in_reply_to,omitempty"`
	AwaitPeerAck bool   `json:"await_peer_ack,omitempty"`
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	q := r.URL.Query()
	agentID := strings.TrimSpace(q.Get("agent_id"))
	if agentID == "" {
		agentID = agentfeed.CoordinatorSeatID(s.projectRoot)
		if agentID == "" && len(agentfeed.DefaultPeerSeatIDs) > 0 {
			agentID = agentfeed.DefaultPeerSeatIDs[0]
		}
		if agentID == "" {
			agentID = "coordinator"
		}
	}

	personaRef := strings.TrimSpace(q.Get("persona_ref"))
	if personaRef == "" {
		personaRef = agentfeed.SeatPersonaRef(s.projectRoot, agentID)
	}

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	snap, err := agentfeed.LoadCorrespondence(s.projectRoot, agentfeed.Seat{
		AgentID:    agentID,
		PersonaRef: personaRef,
	}, limit)
	if err != nil && s.logger != nil {
		s.logger.Warn("Failed to load correspondence for inbox", logging.String("error", err.Error()))
	}

	var staged []StagedEnvelopeItem
	activeEnvs, err := tde.LoadActive(s.projectRoot)
	if err == nil && len(activeEnvs) > 0 {
		for _, env := range activeEnvs {
			staged = append(staged, StagedEnvelopeItem{
				ID:        env.ID,
				Kind:      env.Kind,
				TargetID:  env.TargetID,
				Operation: env.Operation,
				ExecuteAt: env.ExecuteAt.UTC().Format(time.RFC3339),
				CreatedAt: env.CreatedAt.UTC().Format(time.RFC3339),
				Status:    string(env.Status),
			})
		}
	}
	if staged == nil {
		staged = make([]StagedEnvelopeItem, 0)
	}
	if snap.InboxUnacked == nil {
		snap.InboxUnacked = make([]agentfeed.CorrespondenceItem, 0)
	}
	if snap.OutboxAwaitingPeerAck == nil {
		snap.OutboxAwaitingPeerAck = make([]agentfeed.CorrespondenceItem, 0)
	}

	total := len(snap.InboxUnacked) + len(staged)
	payload := InboxPayload{
		Status:                objects.ObjectStatusSuccess,
		AgentID:               agentID,
		PersonaID:             snap.PersonaID,
		InboxUnacked:          snap.InboxUnacked,
		OutboxAwaitingPeerAck: snap.OutboxAwaitingPeerAck,
		StagedEnvelopes:       staged,
		TotalUnacked:          total,
		Count:                 total,
		NextActionHint:        snap.NextActionHint,
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleInboxAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	var req InboxAckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "invalid json payload: " + err.Error()})
		return
	}

	envID := strings.TrimSpace(req.EnvelopeID)
	inReplyTo := strings.TrimSpace(req.InReplyTo)

	// If envelope_id is provided, mark it committed in the staging WAL
	if envID != "" {
		wal, err := tde.NewStagingWAL(s.projectRoot)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "failed to open staging WAL: " + err.Error()})
			return
		}
		defer wal.Close()
		if err := wal.MarkCommitted(envID); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "failed to commit envelope: " + err.Error()})
			return
		}
		_ = wal.Sync()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			objects.FieldKeyStatus:  objects.ObjectStatusSuccess,
			"acknowledged_envelope": envID,
		})
		return
	}

	if inReplyTo == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "in_reply_to or envelope_id is required"})
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = agentfeed.CoordinatorSeatID(s.projectRoot)
		if agentID == "" && len(agentfeed.DefaultPeerSeatIDs) > 0 {
			agentID = agentfeed.DefaultPeerSeatIDs[0]
		}
		if agentID == "" {
			agentID = "operator"
		}
	}
	personaRef := strings.TrimSpace(req.PersonaRef)
	if personaRef == "" {
		personaRef = agentfeed.SeatPersonaRef(s.projectRoot, agentID)
		if personaRef == "" {
			personaRef = "PER-DEFAULT-OPERATOR"
		}
	}
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = "Acknowledged via Web Studio"
	}

	res, err := agentfeed.AppendPeerAck(s.projectRoot, agentID, personaRef, inReplyTo, summary)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": err.Error()})
		return
	}

	completed, _ := agentfeed.CompletePeerAckAwaits(s.projectRoot, inReplyTo, agentID)
	out := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusSuccess,
		"event_id":             res.EventID,
		"event_path":           res.EventPath,
		"in_reply_to":          inReplyTo,
		"agent_id":             agentID,
		"persona_ref":          personaRef,
	}
	if len(completed) > 0 {
		var ids []string
		for _, a := range completed {
			ids = append(ids, a.ID)
		}
		out["awaits_completed"] = ids
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleInboxRespond(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	var req InboxRespondRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "invalid json payload: " + err.Error()})
		return
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "message is required"})
		return
	}

	toAgentID := strings.TrimSpace(req.ToAgentID)
	if toAgentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": "to_agent_id is required"})
		return
	}

	// POL-AGENT-ORCH-HOURGLASS-001: directed collaboration requires await_peer_ack
	if err := agentfeed.EnforceDirectedHourglass(toAgentID, req.AwaitPeerAck); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": err.Error()})
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = "operator"
	}

	res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: s.projectRoot,
		Message:     msg,
		AgentID:     agentID,
		ToAgentID:   toAgentID,
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": err.Error()})
		return
	}

	out := map[string]any{
		objects.FieldKeyStatus:       objects.ObjectStatusSuccess,
		agentfeed.JSONFieldEventID:   res.EventID,
		agentfeed.JSONFieldFeedID:    res.FeedID,
		agentfeed.JSONFieldEventPath: res.EventPath,
		agentfeed.JSONFieldAgentID:   agentID,
		agentfeed.JSONFieldToAgentID: toAgentID,
		agentfeed.JSONFieldMessage:   msg,
	}

	if req.AwaitPeerAck {
		aw, aerr := agentfeed.RegisterPeerAckAwait(s.projectRoot, agentfeed.PeerAckAwaitInput{
			EventID:     res.EventID,
			FromAgentID: agentID,
			ToAgentID:   toAgentID,
			Action:      agentfeed.AwaitActionWake,
			WakeMessage: agentfeed.PeerAckPasteStub(res.EventID),
		})
		if aerr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, "error": aerr.Error()})
			return
		}
		out[agentfeed.JSONFieldPeerAckAwaitID] = aw.ID
		out[agentfeed.JSONFieldAwaitPeerAck] = true
	}

	inReplyTo := strings.TrimSpace(req.InReplyTo)
	if inReplyTo != "" {
		personaRef := agentfeed.SeatPersonaRef(s.projectRoot, agentID)
		if personaRef == "" {
			personaRef = "PER-DEFAULT-OPERATOR"
		}
		_, _ = agentfeed.AppendPeerAck(s.projectRoot, agentID, personaRef, inReplyTo, "Replied via Web Studio: "+truncateText(msg, 60))
		_, _ = agentfeed.CompletePeerAckAwaits(s.projectRoot, inReplyTo, agentID)
		out[agentfeed.JSONFieldInReplyTo] = inReplyTo
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
