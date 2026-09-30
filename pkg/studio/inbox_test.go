package studio

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
)

func setupTestStudio(t *testing.T) (*Server, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Seed agent chat channel directory and enabled config
	channelDir := filepath.Join(tmpDir, ".zqk", "logs", "ide-hooks")
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		t.Fatalf("failed to create test channel dir: %v", err)
	}

	if err := datacell.WriteAgentChatChannelConfig(tmpDir, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeNotify,
		FeedID:        "AGF-test-studio",
	}); err != nil {
		t.Fatalf("WriteAgentChatChannelConfig: %v", err)
	}

	server := NewServer(tmpDir)
	return server, tmpDir
}

// TestInboxInspect verifies CRI-CEF-UI-INBOX-INSPECT: structured inspection of unacknowledged correspondence.
func TestInboxInspect(t *testing.T) {
	server, root := setupTestStudio(t)

	// 1. Initial empty inbox
	req := httptest.NewRequest(http.MethodGet, "/api/inbox?agent_id=operator", nil)
	rec := httptest.NewRecorder()
	server.handleInbox(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp[objects.FieldKeyStatus] != objects.ObjectStatusSuccess {
		t.Errorf("expected status=success, got %v", resp[objects.FieldKeyStatus])
	}

	// 2. Append an inbound steer to operator
	_, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN: operator - urgent review required",
		AgentID:     "peer-seat-1",
		ToAgentID:   "operator",
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatalf("failed to append test event: %v", err)
	}

	// Inspect again
	req2 := httptest.NewRequest(http.MethodGet, "/api/inbox?agent_id=operator", nil)
	rec2 := httptest.NewRecorder()
	server.handleInbox(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var resp2 map[string]any
	if err := json.NewDecoder(rec2.Body).Decode(&resp2); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	count, ok := resp2["count"].(float64)
	if !ok || count < 1 {
		t.Errorf("expected count >= 1, got %v", resp2["count"])
	}

	// 3. Method not allowed for POST /api/inbox
	reqPost := httptest.NewRequest(http.MethodPost, "/api/inbox", nil)
	recPost := httptest.NewRecorder()
	server.handleInbox(recPost, reqPost)
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 MethodNotAllowed for POST, got %d", recPost.Code)
	}
}

// TestInboxAcknowledge verifies CRI-CEF-UI-INBOX-ACKNOWLEDGE: inbound message acknowledgment.
func TestInboxAcknowledge(t *testing.T) {
	server, root := setupTestStudio(t)

	// Append incoming message
	res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: root,
		Message:     "ATTN: operator - please confirm receipt",
		AgentID:     "peer-seat-2",
		ToAgentID:   "operator",
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeMeshStatus,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatalf("failed to append test event: %v", err)
	}

	// 1. Missing in_reply_to fails closed
	badBody := []byte(`{"agent_id": "operator"}`)
	reqBad := httptest.NewRequest(http.MethodPost, "/api/inbox/ack", bytes.NewReader(badBody))
	recBad := httptest.NewRecorder()
	server.handleInboxAck(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing in_reply_to, got %d", recBad.Code)
	}

	// 2. Valid ACK succeeds
	ackPayload := InboxAckRequest{
		InReplyTo:  res.EventID,
		AgentID:    "operator",
		PersonaRef: "PER-DEFAULT-OPERATOR",
		Summary:    "Confirmed receipt from operator",
	}
	body, _ := json.Marshal(ackPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/inbox/ack", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleInboxAck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ACK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp[objects.FieldKeyStatus] != objects.ObjectStatusSuccess {
		t.Errorf("expected status=success, got %v", resp[objects.FieldKeyStatus])
	}
	if resp["in_reply_to"] != res.EventID {
		t.Errorf("expected in_reply_to=%s, got %v", res.EventID, resp["in_reply_to"])
	}

	// 3. Method not allowed for GET /api/inbox/ack
	reqGet := httptest.NewRequest(http.MethodGet, "/api/inbox/ack", nil)
	recGet := httptest.NewRecorder()
	server.handleInboxAck(recGet, reqGet)
	if recGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 MethodNotAllowed for GET, got %d", recGet.Code)
	}
}

// TestInboxRespondAndReject verifies CRI-CEF-UI-INBOX-RESPOND-REJECT: outbound steering and fail-closed validation.
func TestInboxRespondAndReject(t *testing.T) {
	server, _ := setupTestStudio(t)

	// 1. Missing recipient fails closed
	badReq1 := InboxRespondRequest{
		ToAgentID: "",
		Message:   "Hello",
	}
	body1, _ := json.Marshal(badReq1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/inbox/respond", bytes.NewReader(body1))
	rec1 := httptest.NewRecorder()
	server.handleInboxRespond(rec1, req1)
	if rec1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty recipient, got %d", rec1.Code)
	}

	// 2. Empty message fails closed
	badReq2 := InboxRespondRequest{
		ToAgentID: "worker-1",
		Message:   "   ",
	}
	body2, _ := json.Marshal(badReq2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/inbox/respond", bytes.NewReader(body2))
	rec2 := httptest.NewRecorder()
	server.handleInboxRespond(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty message, got %d", rec2.Code)
	}

	// 3. Directed message without await_peer_ack violates hourglass protocol
	badReq3 := InboxRespondRequest{
		ToAgentID:    "worker-1",
		Message:      "Execute task 123",
		AwaitPeerAck: false,
	}
	body3, _ := json.Marshal(badReq3)
	req3 := httptest.NewRequest(http.MethodPost, "/api/inbox/respond", bytes.NewReader(body3))
	rec3 := httptest.NewRecorder()
	server.handleInboxRespond(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for directed steer without await_peer_ack, got %d", rec3.Code)
	}

	// 4. Valid directed response with await_peer_ack succeeds
	validReq := InboxRespondRequest{
		ToAgentID:    "worker-1",
		Message:      "Execute task 123 with hourglass ack",
		AgentID:      "operator",
		AwaitPeerAck: true,
	}
	bodyValid, _ := json.Marshal(validReq)
	reqValid := httptest.NewRequest(http.MethodPost, "/api/inbox/respond", bytes.NewReader(bodyValid))
	recValid := httptest.NewRecorder()
	server.handleInboxRespond(recValid, reqValid)

	if recValid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid response, got %d: %s", recValid.Code, recValid.Body.String())
	}

	var respValid map[string]any
	if err := json.NewDecoder(recValid.Body).Decode(&respValid); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if respValid[objects.FieldKeyStatus] != objects.ObjectStatusSuccess {
		t.Errorf("expected status=success, got %v", respValid[objects.FieldKeyStatus])
	}
	if respValid[agentfeed.JSONFieldAwaitPeerAck] != true {
		t.Errorf("expected await_peer_ack=true, got %v", respValid[agentfeed.JSONFieldAwaitPeerAck])
	}
	if respValid[agentfeed.JSONFieldPeerAckAwaitID] == "" || respValid[agentfeed.JSONFieldPeerAckAwaitID] == nil {
		t.Errorf("expected peer_ack_await_id to be populated")
	}

	// 5. Method not allowed for GET /api/inbox/respond
	reqGet := httptest.NewRequest(http.MethodGet, "/api/inbox/respond", nil)
	recGet := httptest.NewRecorder()
	server.handleInboxRespond(recGet, reqGet)
	if recGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 MethodNotAllowed for GET, got %d", recGet.Code)
	}
}
