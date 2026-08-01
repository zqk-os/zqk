package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
)

// inMemSpine provides a simple in-memory broadcast for testing.
type inMemSpine struct {
	mu       sync.RWMutex
	handlers map[string][]infrastructure.Handler
}

func (s *inMemSpine) Publish(ctx context.Context, event infrastructure.Event) error {
	s.mu.RLock()
	handlers := s.handlers[event.Kind]
	s.mu.RUnlock()

	for _, h := range handlers {
		// Run in goroutine to simulate async nature
		go func(handler infrastructure.Handler, ev infrastructure.Event) {
			_ = handler(context.Background(), ev)
		}(h, event)
	}
	return nil
}

func (s *inMemSpine) Subscribe(ctx context.Context, kind string, handler infrastructure.Handler) error {
	s.mu.Lock()
	if s.handlers == nil {
		s.handlers = make(map[string][]infrastructure.Handler)
	}
	s.handlers[kind] = append(s.handlers[kind], handler)
	s.mu.Unlock()
	return nil
}

func (s *inMemSpine) Replay(ctx context.Context, appliedSeq int64, handler infrastructure.Handler) error {
	return nil
}

func (s *inMemSpine) Close() error {
	return nil
}

type mockSpine struct {
	infrastructure.SpinalSpine
	published []infrastructure.Event
}

func (m *mockSpine) Publish(_ context.Context, event infrastructure.Event) error {
	m.published = append(m.published, event)
	return nil
}

func (m *mockSpine) Subscribe(ctx context.Context, kind string, handler infrastructure.Handler) error {
	return nil
}

func TestFederatedMeshLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Shared Infrastructure
	baseSpine := &inMemSpine{}
	bufferedSpine := infrastructure.NewBufferedSpine(baseSpine, 100)
	defer bufferedSpine.Close()

	// 2. Kernel-001 (Source)
	signer1, _ := crypto.GenerateKeypair()
	engine1 := NewFederatedQueryEngine("kernel-001", bufferedSpine, signer1)
	if err := engine1.Start(ctx); err != nil {
		t.Fatalf("failed to start engine1: %v", err)
	}

	// 3. Kernel-002 (Target)
	signer2, _ := crypto.GenerateKeypair()
	engine2 := NewFederatedQueryEngine("kernel-002", bufferedSpine, signer2)
	if err := engine2.Start(ctx); err != nil {
		t.Fatalf("failed to start engine2: %v", err)
	}

	// Wait a bit for subscriptions to settle
	time.Sleep(100 * time.Millisecond)

	// 4. Kernel-001 routes a query
	query := "How do I optimize cross-kernel graph traversal?"
	reqID, err := engine1.RouteQuery(ctx, "architect_advice", query)
	if err != nil {
		t.Fatalf("RouteQuery failed: %v", err)
	}

	// 5. Kernel-001 waits for response (ResponseListener)
	resp, err := engine1.ResponseListener(ctx, reqID)
	if err != nil {
		t.Fatalf("ResponseListener failed: %v", err)
	}

	// 6. Validation
	if resp.Status != "success" {
		t.Errorf("expected success status, got %s", resp.Status)
	}

	msg, ok := resp.Result["message"].(string)
	if !ok || msg == "" {
		t.Errorf("expected message in result, got %+v", resp.Result)
	}

	// Verify signature using DataToSign() logic
	if !validResponseSignature(resp) {
		t.Errorf("invalid response signature")
	}

	if resp.PublicKey != signer2.PublicKey() {
		t.Errorf("expected response from kernel-002 (%s), got %s", signer2.PublicKey(), resp.PublicKey)
	}

	fmt.Printf("✅ Received valid federated response: %s\n", msg)
}

func validResponseSignature(resp *QueryResponse) bool {
	valid, _ := crypto.GlobalVerifier.Verify(resp.DataToSign(), resp.Signature, resp.PublicKey)
	return valid
}

func TestFederatedQueryEngine_CrystallineFortress(t *testing.T) {
	ctx := context.Background()
	spine := &mockSpine{}
	signer, _ := crypto.GenerateKeypair()
	engine := NewFederatedQueryEngine("kernel-001", spine, signer)

	// 1. Test REQ-501: Mandatory Request Validation
	// a) Valid request
	req := QueryRequest{
		ID:        "QRY-1",
		Query:     "test query",
		Kind:      "test-kind",
		SourceID:  "kernel-002",
		PublicKey: signer.PublicKey(),
	}
	sig, _ := signer.Sign(req.DataToSign())
	req.Signature = sig

	payload := make(map[string]any)
	data, _ := json.Marshal(req)
	_ = json.Unmarshal(data, &payload)

	event := infrastructure.Event{
		ObjectID: "QRY-1",
		Kind:     "federated_query",
		Op:       "request",
		Payload:  payload,
	}

	if err := engine.HandleQueryRequest(ctx, event); err != nil {
		t.Fatalf("HandleQueryRequest failed on valid request: %v", err)
	}

	// b) Invalid request (missing signature)
	req.Signature = ""
	data, _ = json.Marshal(req)
	_ = json.Unmarshal(data, &payload)
	event.Payload = payload

	if err := engine.HandleQueryRequest(ctx, event); err != nil {
		t.Fatalf("HandleQueryRequest error on invalid request: %v", err)
	}

	// 2. Test REQ-504: Ontology Sync
	layerHashes := map[string]string{"L1": "hash1", "L2": "hash2"}
	if err := engine.SyncOntology(ctx, layerHashes); err != nil {
		t.Fatalf("SyncOntology failed: %v", err)
	}
}
