package mesh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Mock graph connection and pool for state sync testing
type mockMeshGraphConn struct {
	provider.GraphConnection
	executedQueries []provider.Query
}

func (m *mockMeshGraphConn) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	m.executedQueries = append(m.executedQueries, query)
	return &provider.QueryResult{}, nil
}

type mockMeshGraphPool struct {
	conn       *mockMeshGraphConn
	getConnErr error
	returned   bool
}

func (p *mockMeshGraphPool) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
	if p.getConnErr != nil {
		return nil, p.getConnErr
	}
	return p.conn, nil
}

func (p *mockMeshGraphPool) ReturnConnection(conn provider.GraphConnection) error {
	p.returned = true
	return nil
}

func (p *mockMeshGraphPool) Execute(ctx context.Context, fn func(conn provider.GraphConnection) error) error {
	return nil
}

func (p *mockMeshGraphPool) Stats() provider.PoolStats {
	return provider.PoolStats{}
}

func (p *mockMeshGraphPool) Close() error {
	return nil
}

// Mock registry for trust verification testing
type mockMeshRegistry struct {
	nodes map[string]*RemoteKernel
	err   error
}

func (m *mockMeshRegistry) Verify(ctx context.Context, nodeID string) (*RemoteKernel, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.nodes[nodeID], nil
}

func (m *mockMeshRegistry) Register(ctx context.Context, kernel RemoteKernel) error {
	if m.nodes == nil {
		m.nodes = make(map[string]*RemoteKernel)
	}
	m.nodes[kernel.ID] = &kernel
	return nil
}

func TestGossipSyncer_ComprehensiveCoverage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Invalid address in constructor
	_, err := NewGossipSyncer("invalid:::address", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid address in NewGossipSyncer")
	}

	// 2. Unstarted Stop & Broadcast errors
	syncerUnstarted, err := NewGossipSyncer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatalf("failed to create unstarted syncer: %v", err)
	}
	if err := syncerUnstarted.Stop(); err != nil {
		t.Errorf("expected nil error on unstarted Stop, got: %v", err)
	}
	if err := syncerUnstarted.Broadcast(ctx, SyncEvent{ItemID: "item-1"}); err == nil {
		t.Fatal("expected error on unstarted Broadcast, got nil")
	}

	// 3. Port conflict in Start()
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen UDP for port conflict: %v", err)
	}
	defer l.Close()

	syncerConflict, err := NewGossipSyncer(l.LocalAddr().String(), nil, nil)
	if err != nil {
		t.Fatalf("failed to create conflict syncer: %v", err)
	}
	if err := syncerConflict.Start(ctx); err == nil {
		t.Fatal("expected error starting syncer on occupied port, got nil")
	}

	// 4. Setup mock graph pool & trust verifier
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}
	nodeID := "node-test-1"
	reg := &mockMeshRegistry{
		nodes: map[string]*RemoteKernel{
			nodeID: {
				ID:        nodeID,
				PublicKey: hex.EncodeToString(pub),
			},
		},
	}
	verifier := NewTrustVerifier(reg)

	mockConn := &mockMeshGraphConn{}
	mockPool := &mockMeshGraphPool{conn: mockConn}

	// 5. Normal start on dynamic port
	syncer, err := NewGossipSyncer("127.0.0.1:0", mockPool, verifier)
	if err != nil {
		t.Fatalf("failed to create valid syncer: %v", err)
	}
	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("failed to start syncer: %v", err)
	}
	defer func() { _ = syncer.Stop() }()

	receivedChan := make(chan SyncEvent, 5)
	syncer.Subscribe(func(event SyncEvent) {
		receivedChan <- event
	})

	localAddr := syncer.conn.LocalAddr().(*net.UDPAddr)

	// 6. Test UDP receive - invalid JSON
	clientConn, err := net.DialUDP("udp", nil, localAddr)
	if err != nil {
		t.Fatalf("failed to dial UDP client: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("not valid json")); err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}

	// 7. Test UDP receive - unverified / invalid signature event
	unverifiedEvent := SyncEvent{
		NodeID:    nodeID,
		Timestamp: time.Now().UTC(),
		Kind:      "GraphState",
		ItemID:    "item-bad-sig",
		Signature: hex.EncodeToString(make([]byte, 64)),
	}
	badPayload, _ := json.Marshal(unverifiedEvent)
	if _, err := clientConn.Write(badPayload); err != nil {
		t.Fatalf("failed to write bad signature event: %v", err)
	}

	// 8. Test UDP receive - valid signed event
	validEvent := SyncEvent{
		NodeID:    nodeID,
		Timestamp: time.Now().UTC(),
		Kind:      "GraphState",
		ItemID:    "item-good-1",
	}
	sig := ed25519.Sign(priv, validEvent.PayloadForSignature())
	validEvent.Signature = hex.EncodeToString(sig)
	goodPayload, _ := json.Marshal(validEvent)
	if _, err := clientConn.Write(goodPayload); err != nil {
		t.Fatalf("failed to write valid event: %v", err)
	}

	// Wait for handler to receive valid event
	select {
	case received := <-receivedChan:
		if received.ItemID != "item-good-1" {
			t.Errorf("expected item-good-1, got %s", received.ItemID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for verified gossip event")
	}

	// Verify validateWithGraph was called
	if !mockPool.returned {
		t.Error("expected connection to be returned to mock pool")
	}
	if len(mockConn.executedQueries) == 0 {
		t.Error("expected graph query to be executed during state sync validation")
	}

	// 9. Test validateWithGraph error branch
	errorPool := &mockMeshGraphPool{getConnErr: errors.New("pool failure")}
	syncerErrGraph := &GossipSyncer{graphPool: errorPool}
	syncerErrGraph.validateWithGraph(ctx, validEvent) // should not panic, returns early

	// 10. Test Broadcast with explicit timestamp, zero timestamp, and closed connection
	eventWithTs := SyncEvent{
		NodeID:    nodeID,
		Timestamp: time.Now().UTC(),
		Kind:      "Schema",
		ItemID:    "schema-1",
	}
	// Broadcast to syncer's own address
	syncer.addr = localAddr
	if err := syncer.Broadcast(ctx, eventWithTs); err != nil {
		t.Errorf("unexpected error broadcasting with timestamp: %v", err)
	}

	eventZeroTs := SyncEvent{
		NodeID: nodeID,
		Kind:   "Schema",
		ItemID: "schema-zero-ts",
	}
	if err := syncer.Broadcast(ctx, eventZeroTs); err != nil {
		t.Errorf("unexpected error broadcasting with zero timestamp: %v", err)
	}

	// Stop syncer cleanly: cancels context and closes UDP conn
	_ = syncer.Stop()

	// Broadcast on stopped syncer should fail with write on closed network connection
	if err := syncer.Broadcast(ctx, eventWithTs); err == nil {
		t.Error("expected error broadcasting on closed UDP connection, got nil")
	}
}

func TestTrustVerifier_BranchCoverage(t *testing.T) {
	ctx := context.Background()

	// 1. Registry returns nil remote and nil error -> ErrUnknownNode
	regEmpty := &mockMeshRegistry{}
	tv := NewTrustVerifier(regEmpty)
	err := tv.VerifyState(ctx, SyncedState{NodeID: "unknown"})
	if !errors.Is(err, ErrUnknownNode) {
		t.Errorf("expected ErrUnknownNode, got: %v", err)
	}

	// 2. Registry returns error -> returns err
	regErr := &mockMeshRegistry{err: errors.New("db error")}
	tvErr := NewTrustVerifier(regErr)
	if err := tvErr.VerifyState(ctx, SyncedState{NodeID: "any"}); err == nil {
		t.Error("expected error from failing registry")
	}

	// 3. Invalid hex in public key
	regBadHexPub := &mockMeshRegistry{
		nodes: map[string]*RemoteKernel{
			"node-bad-hex": {ID: "node-bad-hex", PublicKey: "invalid_hex!?"},
		},
	}
	tvBadHex := NewTrustVerifier(regBadHexPub)
	if err := tvBadHex.VerifyState(ctx, SyncedState{NodeID: "node-bad-hex"}); !errors.Is(err, ErrInvalidPublicKey) {
		t.Errorf("expected ErrInvalidPublicKey, got %v", err)
	}

	// 4. Hex public key of wrong length (e.g. 16 bytes instead of 32)
	regShortPub := &mockMeshRegistry{
		nodes: map[string]*RemoteKernel{
			"node-short": {ID: "node-short", PublicKey: hex.EncodeToString(make([]byte, 16))},
		},
	}
	tvShortPub := NewTrustVerifier(regShortPub)
	if err := tvShortPub.VerifyState(ctx, SyncedState{NodeID: "node-short"}); !errors.Is(err, ErrInvalidPublicKey) {
		t.Errorf("expected ErrInvalidPublicKey, got %v", err)
	}

	// 5. Valid public key, invalid signature hex
	pub, priv, _ := ed25519.GenerateKey(nil)
	regValid := &mockMeshRegistry{
		nodes: map[string]*RemoteKernel{
			"node-valid": {ID: "node-valid", PublicKey: hex.EncodeToString(pub)},
		},
	}
	tvValid := NewTrustVerifier(regValid)
	if err := tvValid.VerifyState(ctx, SyncedState{
		NodeID:    "node-valid",
		Signature: "bad_sig_hex!",
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for bad signature hex, got %v", err)
	}

	// 6. Signature of wrong length (e.g. 32 bytes instead of 64)
	if err := tvValid.VerifyState(ctx, SyncedState{
		NodeID:    "node-valid",
		Signature: hex.EncodeToString(make([]byte, 32)),
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for short signature, got %v", err)
	}

	// 7. Signature mismatch
	sig := ed25519.Sign(priv, []byte("correct payload"))
	if err := tvValid.VerifyState(ctx, SyncedState{
		NodeID:    "node-valid",
		Payload:   []byte("tampered payload"),
		Signature: hex.EncodeToString(sig),
	}); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for tampered payload, got %v", err)
	}

	// 8. Successful verification
	if err := tvValid.VerifyState(ctx, SyncedState{
		NodeID:    "node-valid",
		Payload:   []byte("correct payload"),
		Signature: hex.EncodeToString(sig),
	}); err != nil {
		t.Errorf("expected nil error on valid verification, got %v", err)
	}
}

func TestToolPodCoordinator_ErrorBranches(t *testing.T) {
	coordinator := NewHTTPToolPodCoordinator()

	// 1. Invalid endpoint URL format
	err := coordinator.DispatchFFmpegJob(context.Background(), "://invalid-url\x7f", FFmpegJobRequest{})
	if err == nil {
		t.Error("expected error for invalid endpoint URL")
	}

	// 2. Closed port / connection failure
	err = coordinator.DispatchFFmpegJob(context.Background(), "http://127.0.0.1:54321", FFmpegJobRequest{})
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestAsyncDispatchQueue_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// 1. DequeueBatch with maxBatch <= 0
	q := NewAsyncDispatchQueue(10)
	items, err := q.DequeueBatch(ctx, 0)
	if err != nil || items != nil {
		t.Errorf("expected nil, nil for maxBatch <= 0, got %v, %v", items, err)
	}
	items, err = q.DequeueBatch(ctx, -5)
	if err != nil || items != nil {
		t.Errorf("expected nil, nil for maxBatch < 0, got %v, %v", items, err)
	}

	// 2. Context canceled before DequeueBatch on empty queue
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = q.DequeueBatch(canceledCtx, 5)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	// 3. DequeueBatch on closed empty queue
	_ = q.Close()
	_, err = q.DequeueBatch(ctx, 1)
	if err == nil {
		t.Error("expected error on closed queue dequeue, got nil")
	}

	// 4. Enqueue on closed queue & with explicit timestamp
	err = q.Enqueue(ctx, AsyncMessage{ID: "msg-1"})
	if err == nil {
		t.Error("expected error enqueuing to closed queue")
	}

	// 5. Enqueue with non-zero timestamp on active queue and test step-by-step draining
	qOpen := NewAsyncDispatchQueue(10)
	defer qOpen.Close()
	if err := qOpen.Enqueue(ctx, AsyncMessage{
		ID:        "m-ts-h1",
		Timestamp: time.Now().Add(-1 * time.Minute),
		Priority:  PriorityHigh,
	}); err != nil {
		t.Fatalf("failed to enqueue with timestamp: %v", err)
	}
	_ = qOpen.Enqueue(ctx, AsyncMessage{ID: "m-ts-h2", Priority: PriorityHigh})
	_ = qOpen.Enqueue(ctx, AsyncMessage{ID: "m-ts-n1", Priority: PriorityNormal})
	_ = qOpen.Enqueue(ctx, AsyncMessage{ID: "m-ts-n2", Priority: PriorityNormal})
	_ = qOpen.Enqueue(ctx, AsyncMessage{ID: "m-ts-l1", Priority: PriorityLow})
	_ = qOpen.Enqueue(ctx, AsyncMessage{ID: "m-ts-l2", Priority: PriorityLow})

	// Drain 1 item (hits high queue cap)
	b1, err := qOpen.DequeueBatch(ctx, 1)
	if err != nil || len(b1) != 1 || b1[0].ID != "m-ts-h1" {
		t.Errorf("unexpected batch 1: %v, %v", b1, err)
	}
	// Drain 2 items (empties high queue, takes 1 normal)
	b2, err := qOpen.DequeueBatch(ctx, 2)
	if err != nil || len(b2) != 2 {
		t.Errorf("unexpected batch 2: %v, %v", b2, err)
	}
	// Drain 2 items (empties normal, takes 1 low)
	b3, err := qOpen.DequeueBatch(ctx, 2)
	if err != nil || len(b3) != 2 {
		t.Errorf("unexpected batch 3: %v, %v", b3, err)
	}
	// Drain 1 remaining item
	b4, err := qOpen.DequeueBatch(ctx, 1)
	if err != nil || len(b4) != 1 {
		t.Errorf("unexpected batch 4: %v, %v", b4, err)
	}
}

func TestWebhookHandler_EdgeBranches(t *testing.T) {
	// 1. Success with URL update
	store := &extraRecordingStore{records: make(map[string]map[string]any)}
	handler := WebhookHandler(store)

	reqPayload := WebhookPayload{
		JobID:  "job-rec-1",
		Status: objects.ObjectStatusCompleted,
		URL:    "https://storage.example.com/out.mp4",
	}
	body, _ := json.Marshal(reqPayload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if store.records["job-rec-1"]["artifact_url"] != "https://storage.example.com/out.mp4" {
		t.Errorf("expected artifact_url recorded, got: %v", store.records["job-rec-1"])
	}
}

type extraRecordingStore struct {
	extraFailStore
	records map[string]map[string]any
}

func (s *extraRecordingStore) Update(ctx context.Context, sec *storage.SecurityContext, id string, updates map[string]any) error {
	s.records[id] = updates
	return nil
}
