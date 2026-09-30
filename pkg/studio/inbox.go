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

const (
	errMsgMethodNotAllowed     = "method not allowed"
	errMsgLoadCorrespondence   = "Failed to load correspondence for inbox"
	errMsgInvalidJSONPayload   = "invalid json payload: "
	errMsgFailedStagingWAL     = "failed to open staging WAL: "
	errMsgFailedCommitEnvelope = "failed to commit envelope: "
	errMsgReplyOrEnvRequired   = "in_reply_to or envelope_id is required"
	errMsgMessageRequired      = "message is required"
	errMsgToAgentRequired      = "to_agent_id is required"
	warnPeerAckAwaits          = "failed to complete peer ack awaits"
	warnPeerAckAppend          = "failed to append peer ack"
	defaultOperatorPersona     = "PER-DEFAULT-OPERATOR"
	defaultStudioAckSummary    = "Acknowledged via Web Studio"
	studioReplyPrefix          = "Replied via Web Studio: "
	keyAcknowledgedEnvelope    = "acknowledged_envelope"
	keyAwaitsCompleted         = "awaits_completed"
	queryParamAgentID          = "agent_id"
	queryParamPersonaRef       = "persona_ref"
	queryParamLimit            = "limit"
	defaultCoordinatorSeat     = "coordinator"
	defaultOperatorSeat        = "operator"
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

func writeJSONResponse(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSONResponse(w, status, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusError,
		"error":                message,
	})
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	q := r.URL.Query()
	agentID := strings.TrimSpace(q.Get(queryParamAgentID))
	if agentID == "" {
		agentID = agentfeed.CoordinatorSeatID(s.projectRoot)
		if agentID == "" && len(agentfeed.DefaultPeerSeatIDs) > 0 {
			agentID = agentfeed.DefaultPeerSeatIDs[0]
		}
		if agentID == "" {
			agentID = defaultCoordinatorSeat
		}
	}

	personaRef := strings.TrimSpace(q.Get(queryParamPersonaRef))
	if personaRef == "" {
		personaRef = agentfeed.SeatPersonaRef(s.projectRoot, agentID)
	}

	limit := 50
	if lStr := q.Get(queryParamLimit); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	snap, err := agentfeed.LoadCorrespondence(s.projectRoot, agentfeed.Seat{
		AgentID:    agentID,
		PersonaRef: personaRef,
	}, limit)
	if err != nil && s.logger != nil {
		s.logger.Warn(errMsgLoadCorrespondence, logging.String("error", err.Error()))
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

	writeJSONResponse(w, http.StatusOK, payload)
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
		writeJSONError(w, http.StatusBadRequest, errMsgInvalidJSONPayload+err.Error())
		return
	}

	envID := strings.TrimSpace(req.EnvelopeID)
	inReplyTo := strings.TrimSpace(req.InReplyTo)

	// If envelope_id is provided, mark it committed in the staging WAL
	if envID != "" {
		wal, err := tde.NewStagingWAL(s.projectRoot)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, errMsgFailedStagingWAL+err.Error())
			return
		}
		defer wal.Close()
		if err := wal.MarkCommitted(envID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, errMsgFailedCommitEnvelope+err.Error())
			return
		}
		if err := wal.Sync(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, errMsgFailedCommitEnvelope+err.Error())
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{
			objects.FieldKeyStatus:  objects.ObjectStatusSuccess,
			keyAcknowledgedEnvelope: envID,
		})
		return
	}

	if inReplyTo == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgReplyOrEnvRequired)
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = agentfeed.CoordinatorSeatID(s.projectRoot)
		if agentID == "" && len(agentfeed.DefaultPeerSeatIDs) > 0 {
			agentID = agentfeed.DefaultPeerSeatIDs[0]
		}
		if agentID == "" {
			agentID = defaultOperatorSeat
		}
	}
	personaRef := strings.TrimSpace(req.PersonaRef)
	if personaRef == "" {
		personaRef = agentfeed.SeatPersonaRef(s.projectRoot, agentID)
		if personaRef == "" {
			personaRef = defaultOperatorPersona
		}
	}
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = defaultStudioAckSummary
	}

	res, err := agentfeed.AppendPeerAck(s.projectRoot, agentID, personaRef, inReplyTo, summary)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	completed, cErr := agentfeed.CompletePeerAckAwaits(s.projectRoot, inReplyTo, agentID)
	if cErr != nil && s.logger != nil {
		logging.Fluent(s.logger).Warn(warnPeerAckAwaits).WithError(cErr).Log()
	}
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
		out[keyAwaitsCompleted] = ids
	}

	writeJSONResponse(w, http.StatusOK, out)
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
		writeJSONError(w, http.StatusBadRequest, errMsgInvalidJSONPayload+err.Error())
		return
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgMessageRequired)
		return
	}

	toAgentID := strings.TrimSpace(req.ToAgentID)
	if toAgentID == "" {
		writeJSONError(w, http.StatusBadRequest, errMsgToAgentRequired)
		return
	}

	// POL-AGENT-ORCH-HOURGLASS-001: directed collaboration requires await_peer_ack
	if err := agentfeed.EnforceDirectedHourglass(toAgentID, req.AwaitPeerAck); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = defaultOperatorSeat
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
		writeJSONError(w, http.StatusBadRequest, err.Error())
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
			writeJSONError(w, http.StatusBadRequest, aerr.Error())
			return
		}
		out[agentfeed.JSONFieldPeerAckAwaitID] = aw.ID
		out[agentfeed.JSONFieldAwaitPeerAck] = true
	}

	inReplyTo := strings.TrimSpace(req.InReplyTo)
	if inReplyTo != "" {
		personaRef := agentfeed.SeatPersonaRef(s.projectRoot, agentID)
		if personaRef == "" {
			personaRef = defaultOperatorPersona
		}
		if _, ackErr := agentfeed.AppendPeerAck(s.projectRoot, agentID, personaRef, inReplyTo, studioReplyPrefix+truncateText(msg, 60)); ackErr != nil && s.logger != nil {
			logging.Fluent(s.logger).Warn(warnPeerAckAppend).WithError(ackErr).Log()
		}
		if _, compErr := agentfeed.CompletePeerAckAwaits(s.projectRoot, inReplyTo, agentID); compErr != nil && s.logger != nil {
			logging.Fluent(s.logger).Warn(warnPeerAckAwaits).WithError(compErr).Log()
		}
		out[agentfeed.JSONFieldInReplyTo] = inReplyTo
	}

	writeJSONResponse(w, http.StatusOK, out)
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
