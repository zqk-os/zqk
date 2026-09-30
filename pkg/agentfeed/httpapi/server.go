// Package httpapi exposes a private (node-local) HTTP surface for agent_feed I/O
// so operators and swarm peers can steer/ack/pending without an IDE.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	pathHealth        = "/v1/health"
	pathFeedSteer     = "/v1/feed/steer"
	pathFeedAck       = "/v1/feed/ack"
	pathFeedPending   = "/v1/feed/pending"
	pathFeedWake      = "/v1/feed/wake"
	pathOpenAPI       = "/v1/openapi.json"
	defaultAgentID    = "http_api"
	maxBodyBytes      = 1 << 20 // 1 MiB
	readHeaderTimeout = 5 * time.Second
)

// Config binds a private feed API to a project root.
type Config struct {
	ProjectRoot string
	// ListenAddr is host:port (default loopback). Prefer 127.0.0.1 for private API.
	ListenAddr string
	// Token when non-empty requires Authorization: Bearer <token>.
	Token string
	// TLSCertFile and TLSKeyFile when non-empty enable HTTPS/TLS.
	TLSCertFile string
	TLSKeyFile  string
	// MCPTCPAddr is the MCP daemon TCP address for live interrupt probes
	// (CRIT-COMMS-003). Empty uses mcp.DefaultDaemonTCP.
	MCPTCPAddr string
	// SkipMCPProbe disables MCP ActionRequired / subscriber probes (unit tests).
	SkipMCPProbe bool
	// Logger for MCP probe diagnostics; optional.
	Logger logging.Logger
}

// Server is a long-lived HTTP listener for feed operations.
type Server struct {
	cfg    Config
	mux    *http.ServeMux
	server *http.Server
}

// New builds a Server with routes registered (not listening yet).
func New(cfg Config) (*Server, error) {
	cfg.ProjectRoot = strings.TrimSpace(cfg.ProjectRoot)
	if cfg.ProjectRoot == "" {
		return nil, errfmt.Errorf("httpapi: project root required")
	}
	cfg.ListenAddr = strings.TrimSpace(cfg.ListenAddr)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8787"
	}
	isLoopback := mcp.IsLoopbackAddr(cfg.ListenAddr)
	hasTLS := strings.TrimSpace(cfg.TLSCertFile) != "" && strings.TrimSpace(cfg.TLSKeyFile) != ""
	hasToken := strings.TrimSpace(cfg.Token) != ""
	if !isLoopback && (!hasTLS || !hasToken) {
		return nil, errfmt.Errorf("refusing to bind unauthenticated plain HTTP feed server to non-loopback address %q; both TLS and token authentication are required for network exposure (CRIT-CEF-R2-SEC-FEED-AUTH / F-SEC-005)", cfg.ListenAddr)
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc(pathHealth, s.handleHealth)
	s.mux.HandleFunc(pathOpenAPI, s.handleOpenAPI)
	s.mux.HandleFunc(pathFeedSteer, s.withAuth(s.handleSteer))
	s.mux.HandleFunc(pathFeedAck, s.withAuth(s.handleAck))
	s.mux.HandleFunc(pathFeedPending, s.withAuth(s.handlePending))
	s.mux.HandleFunc(pathFeedWake, s.withAuth(s.handleWake))
	s.server = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s, nil
}

// HTTPServer returns the internal http.Server instance.
func (s *Server) HTTPServer() *http.Server { return s.server }

// Handler returns the HTTP handler (for tests).
func (s *Server) Handler() http.Handler { return s.mux }

// ListenAndServe runs until ctx is cancelled or the listener fails.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return errfmt.Newf("httpapi listen %s", s.cfg.ListenAddr).Wrap(err)
	}
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("feed_httpapi_serve", "agent_feed private HTTP Serve").
		StartSimple(func() {
			if strings.TrimSpace(s.cfg.TLSCertFile) != "" && strings.TrimSpace(s.cfg.TLSKeyFile) != "" {
				errCh <- s.server.ServeTLS(ln, s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
			} else {
				errCh <- s.server.Serve(ln)
			}
		})
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		err := <-errCh
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimSpace(s.cfg.Token)
		if tok != "" {
			got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(tok)) != 1 {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		} else if !mcp.IsLoopbackAddr(s.cfg.ListenAddr) {
			writeErr(w, http.StatusUnauthorized, "unauthorized: token required for non-loopback interface")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusPass,
		"time":                 time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			objects.FieldKeyTitle:       "ZQK Agent Feed API",
			objects.FieldKeyVersion:     "1.0.0",
			objects.FieldKeyDescription: "Private node-local HTTP API for ZQK Agent Feed steer, ack, pending, and wake I/O.",
		},
		"paths": map[string]any{
			"/v1/health": map[string]any{
				"get": map[string]any{objects.FieldKeySummary: "Health check"},
			},
			"/v1/feed/steer": map[string]any{
				"post": map[string]any{objects.FieldKeySummary: "Append steering event"},
			},
			"/v1/feed/ack": map[string]any{
				"post": map[string]any{objects.FieldKeySummary: "Acknowledge event delivery"},
			},
			"/v1/feed/pending": map[string]any{
				"get": map[string]any{objects.FieldKeySummary: "Get unacknowledged inbox events"},
			},
			"/v1/feed/wake": map[string]any{
				"post": map[string]any{objects.FieldKeySummary: "Trigger peer wake interrupt"},
			},
		},
	}
	writeJSON(w, http.StatusOK, spec)
}

type steerRequest struct {
	Message      string `json:"message"`
	AgentID      string `json:"agent_id"`
	ToAgentID    string `json:"to_agent_id"`
	FeedID       string `json:"feed_id"`
	NoAck        bool   `json:"no_ack"`
	AwaitPeerAck bool   `json:"await_peer_ack"`
}

func (s *Server) handleSteer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req steerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeErr(w, http.StatusBadRequest, "message is required")
		return
	}
	toAgentID := strings.TrimSpace(req.ToAgentID)
	// POL-AGENT-ORCH-HOURGLASS-001: directed without await_peer_ack is forbidden.
	if err := agentfeed.EnforceDirectedHourglass(toAgentID, req.AwaitPeerAck); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = defaultAgentID
	}
	res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: s.cfg.ProjectRoot,
		Message:     req.Message,
		AgentID:     agentID,
		ToAgentID:   strings.TrimSpace(req.ToAgentID),
		Sender:      agentfeed.FeedSenderHTTPAPI,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     !req.NoAck,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if want := strings.TrimSpace(req.FeedID); want != "" && res.FeedID != "" && want != res.FeedID {
		writeErr(w, http.StatusConflict, "lite feed_id does not match feed_id")
		return
	}
	out := map[string]any{
		objects.FieldKeyStatus:       objects.ObjectStatusSuccess,
		agentfeed.JSONFieldEventID:   res.EventID,
		agentfeed.JSONFieldFeedID:    res.FeedID,
		agentfeed.JSONFieldEventPath: res.EventPath,
		objects.FieldKeyDeliveryMode: res.DeliveryMode,
		agentfeed.JSONFieldAgentID:   agentID,
		agentfeed.JSONFieldSender:    agentfeed.FeedSenderHTTPAPI,
	}
	if req.AwaitPeerAck {
		aw, aerr := agentfeed.RegisterPeerAckAwait(s.cfg.ProjectRoot, agentfeed.PeerAckAwaitInput{
			EventID:     res.EventID,
			FromAgentID: agentID,
			ToAgentID:   toAgentID,
			Action:      agentfeed.AwaitActionWake,
			WakeMessage: agentfeed.PeerAckPasteStub(res.EventID),
		})
		if aerr != nil {
			writeErr(w, http.StatusBadRequest, aerr.Error())
			return
		}
		out[agentfeed.JSONFieldPeerAckAwaitID] = aw.ID
		out[agentfeed.JSONFieldAwaitPeerAck] = true
	}
	// TRACK: CRIT-COMMS-003 — HTTP steer must match CLI: WakePeer + MCP ActionRequired evidence.
	s.attachPeerWake(r.Context(), out, req.Message, agentID, toAgentID, res)
	writeJSON(w, http.StatusOK, out)
}

type ackRequest struct {
	InReplyTo  string `json:"in_reply_to"`
	AgentID    string `json:"agent_id"`
	PersonaRef string `json:"persona_ref"`
	Summary    string `json:"summary"`
}

func (s *Server) handleAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req ackRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inReplyTo := strings.TrimSpace(req.InReplyTo)
	agentID := strings.TrimSpace(req.AgentID)
	personaRef := strings.TrimSpace(req.PersonaRef)
	if inReplyTo == "" || agentID == "" || personaRef == "" {
		writeErr(w, http.StatusBadRequest, "in_reply_to, agent_id, and persona_ref are required")
		return
	}
	res, err := agentfeed.AppendPeerAck(s.cfg.ProjectRoot, agentID, personaRef, inReplyTo, req.Summary)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	completed, _ := agentfeed.CompletePeerAckAwaits(s.cfg.ProjectRoot, inReplyTo, agentID)
	out := map[string]any{
		objects.FieldKeyStatus:       objects.ObjectStatusSuccess,
		agentfeed.JSONFieldEventID:   res.EventID,
		agentfeed.JSONFieldEventPath: res.EventPath,
		agentfeed.JSONFieldInReplyTo: inReplyTo,
		agentfeed.JSONFieldAgentID:   agentID,
		objects.FieldKeyPersonaRef:   personaRef,
		objects.FieldKeyEventType:    agentfeed.FeedEventTypePeerAck,
	}
	if len(completed) > 0 {
		ids := make([]string, 0, len(completed))
		for _, a := range completed {
			ids = append(ids, a.ID)
		}
		out[agentfeed.JSONFieldAwaitsCompleted] = ids
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	q := r.URL.Query()
	agentID := strings.TrimSpace(q.Get(agentfeed.JSONFieldAgentID))
	if agentID == "" {
		writeErr(w, http.StatusBadRequest, "agent_id query param is required")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = n
	}
	snap, err := agentfeed.LoadCorrespondence(s.cfg.ProjectRoot, agentfeed.Seat{
		AgentID:    agentID,
		PersonaRef: strings.TrimSpace(q.Get(objects.FieldKeyPersonaRef)),
	}, limit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		objects.FieldKeyStatus:          objects.ObjectStatusSuccess,
		agentfeed.JSONFieldPersonaID:    snap.PersonaID,
		agentfeed.JSONFieldAgentID:      snap.AgentID,
		agentfeed.JSONFieldInboxUnacked: snap.InboxUnacked,
		agentfeed.JSONFieldOutboxAwait:  snap.OutboxAwaitingPeerAck,
		objects.FieldKeyNextActionHint:  snap.NextActionHint,
		agentfeed.JSONFieldSkipReason:   snap.SkipReason,
	})
}

type wakeRequest struct {
	AgentID   string `json:"agent_id"`
	ToAgentID string `json:"to_agent_id"`
	Message   string `json:"message"`
}

func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req wakeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeErr(w, http.StatusBadRequest, "message is required")
		return
	}
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = defaultAgentID
	}
	toAgentID := strings.TrimSpace(req.ToAgentID)
	res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: s.cfg.ProjectRoot,
		Message:     msg,
		AgentID:     agentID,
		ToAgentID:   toAgentID,
		Sender:      agentfeed.FeedSenderHTTPAPI,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     true,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	out := map[string]any{
		objects.FieldKeyStatus:       objects.ObjectStatusSuccess,
		agentfeed.JSONFieldEventID:   res.EventID,
		agentfeed.JSONFieldFeedID:    res.FeedID,
		agentfeed.JSONFieldEventPath: res.EventPath,
		objects.FieldKeyDeliveryMode: res.DeliveryMode,
		agentfeed.JSONFieldAgentID:   agentID,
		agentfeed.JSONFieldSender:    agentfeed.FeedSenderHTTPAPI,
	}
	// Name said "wake" but previously only appended — must invoke membrane.
	s.attachPeerWake(r.Context(), out, msg, agentID, toAgentID, res)
	writeJSON(w, http.StatusOK, out)
}

// attachPeerWake runs the seat membrane wake (and MCP live interrupt for notify)
// into out. TRACK: CRIT-COMMS-003 / BLI-COMMS-TPM-LIVE-WAKE-001.
func (s *Server) attachPeerWake(ctx context.Context, out map[string]any, message, agentID, toAgentID string, res agentfeed.AppendEventResult) {
	if !agentfeed.ShouldWakePeer(res.DeliveryMode) {
		return
	}
	wakeOpts := agentfeed.WakePeerOptions{
		ProjectRoot:  s.cfg.ProjectRoot,
		Message:      message,
		InReplyTo:    res.EventID,
		FromAgentID:  agentID,
		DeliveryMode: res.DeliveryMode,
		ToAgentID:    toAgentID,
	}
	if !s.cfg.SkipMCPProbe {
		probe := mcp.ProbeFeedSteerMCPWake(ctx, s.cfg.MCPTCPAddr, message, agentID, res.EventID, s.cfg.Logger)
		wakeOpts.MCPIPCDelivered = probe.IPCDelivered
		wakeOpts.MCPSubscribersProbed = probe.SubscribersProbed
		wakeOpts.MCPSubscriberCount = probe.SubscriberCount
		if probe.SubscribersProbed {
			out["mcp_subscribers"] = probe.SubscriberCount
		}
	}
	wake := agentfeed.WakePeerOpts(ctx, wakeOpts)
	out["peer_wake"] = wake
	if wake.Transport != "" {
		out["peer_wake_transport"] = wake.Transport
	}
	if unrepaired, reason, detail := agentfeed.IsUnrepairedWake(wake); unrepaired {
		_ = agentfeed.AlertUnrepairedWake(res.EventID, detail, reason)
		out["peer_wake_unrepaired"] = string(reason)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	body := io.LimitReader(r.Body, maxBodyBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errfmt.Newf("json body").Wrap(err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusError, agentfeed.JSONFieldError: msg})
}
