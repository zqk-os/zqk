package federation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
	"github.com/lanceman/zqk/pkg/objects"
)

// QueryRequest represents a signed semantic request.
type QueryRequest struct {
	ID        string `json:"id"`
	Query     string `json:"query"`
	Kind      string `json:"kind"`
	SourceID  string `json:"source_id"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
}

// DataToSign returns a deterministic byte representation of the request.
func (r QueryRequest) DataToSign() []byte {
	// REQ-501: Sign full payload to prevent replay/malleability
	data := fmt.Sprintf("%s|%s|%s|%s", r.ID, r.SourceID, r.Kind, r.Query)
	return []byte(data)
}

// QueryResponse represents a signed result.
type QueryResponse struct {
	RequestID string         `json:"request_id"`
	Result    map[string]any `json:"result"`
	Status    string         `json:"status"`
	Signature string         `json:"signature,omitempty"`
	PublicKey string         `json:"public_key,omitempty"`
}

func (r QueryResponse) DataToSign() []byte {
	res, _ := json.Marshal(r.Result)
	data := fmt.Sprintf("%s|%s|%s", r.RequestID, r.Status, string(res))
	return []byte(data)
}

// FederatedQueryEngine routes semantic queries to the appropriate kernel nodes.
type FederatedQueryEngine struct {
	kernelID     string
	spine        infrastructure.SpinalSpine
	signer       crypto.Signer
	mu           sync.RWMutex
	routes       map[string]string // kind -> kernelID
	pending      map[string]chan *QueryResponse
	knownKernels map[string]string // sourceID -> publicKey
}

func NewFederatedQueryEngine(kernelID string, spine infrastructure.SpinalSpine, signer crypto.Signer) *FederatedQueryEngine {
	return &FederatedQueryEngine{
		kernelID:     kernelID,
		spine:        spine,
		signer:       signer,
		routes:       make(map[string]string),
		pending:      make(map[string]chan *QueryResponse),
		knownKernels: make(map[string]string),
	}
}

// RegisterKernel pins a public key to a kernel ID (REQ-501 Trust Anchor).
func (e *FederatedQueryEngine) RegisterKernel(kernelID, publicKey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.knownKernels[kernelID] = publicKey
}

func (e *FederatedQueryEngine) signPulse(message string) string {
	if e.signer == nil {
		return "unsigned"
	}
	sig, err := e.signer.Sign([]byte(message))
	if err != nil {
		return "unsigned"
	}
	return "signed:" + sig[:8]
}

// Start initializes the engine and subscribes to federated query events.
func (e *FederatedQueryEngine) Start(ctx context.Context) error {
	return e.spine.Subscribe(ctx, "federated_query", func(ctx context.Context, event infrastructure.Event) error {
		switch event.Op {
		case "request":
			if event.Payload["source_id"] == e.kernelID {
				return nil
			}
			return e.HandleQueryRequest(ctx, event)
		case "response":
			return e.handleIncomingResponse(ctx, event)
		}
		return nil
	})
}

// RouteQuery sends a semantic query to the mesh for fulfillment.
func (e *FederatedQueryEngine) RouteQuery(ctx context.Context, kind, query string) (string, error) {
	requestID := fmt.Sprintf("QRY-%d", time.Now().UnixNano())

	e.mu.Lock()
	e.pending[requestID] = make(chan *QueryResponse, 1)
	e.mu.Unlock()

	req := QueryRequest{
		ID:        requestID,
		Query:     query,
		Kind:      kind,
		SourceID:  e.kernelID,
		PublicKey: e.signer.PublicKey(),
	}

	sig, err := e.signer.Sign(req.DataToSign())
	if err != nil {
		return "", err
	}
	req.Signature = sig

	payload := make(map[string]any)
	data, _ := json.Marshal(req)
	_ = json.Unmarshal(data, &payload)

	event := infrastructure.Event{
		ObjectID: requestID,
		Kind:     "federated_query",
		Op:       "request",
		Payload:  payload,
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [federation] query-engine: [%s] Routing query %s (Kind: %s)\n", e.kernelID, requestID, kind)).Log()
	return requestID, e.spine.Publish(ctx, event)
}

// HandleQueryRequest processes an incoming query request.
func (e *FederatedQueryEngine) HandleQueryRequest(ctx context.Context, event infrastructure.Event) error {
	var req QueryRequest
	data, _ := json.Marshal(event.Payload)
	_ = json.Unmarshal(data, &req)

	// REQ-501: Mandatory Trust Anchor Verification
	e.mu.RLock()
	trustedKey, known := e.knownKernels[req.SourceID]
	e.mu.RUnlock()

	if known && trustedKey != req.PublicKey {
		msg := fmt.Sprintf("ABORT: Identity mismatch for kernel %s (expected %s, got %s)", req.SourceID, trustedKey[:8], req.PublicKey[:8])
		logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [security_violation] [%s] query-engine: %s\n", e.signPulse(msg), msg)).Log()
		return nil
	}

	// Verify signature
	valid, err := crypto.GlobalVerifier.Verify(req.DataToSign(), req.Signature, req.PublicKey)
	if err != nil || !valid {
		msg := fmt.Sprintf("ABORT: Signature violation for request %s from kernel %s", req.ID, req.SourceID)
		logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [security_violation] [%s] query-engine: %s\n", e.signPulse(msg), msg)).Log()
		return nil
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("🧠 [HIVE PULSE] [federation] query-engine: [%s] Handling request %s: %s\n", e.kernelID, event.ObjectID, req.Query)).Log()

	resp := QueryResponse{
		RequestID: event.ObjectID,
		Result:    map[string]any{"message": "Verified inference result", objects.FieldKeySource: e.kernelID},
		Status:    objects.ObjectStatusSuccess,
		PublicKey: e.signer.PublicKey(),
	}

	sig, err := e.signer.Sign(resp.DataToSign())
	if err != nil {
		return err
	}
	resp.Signature = sig

	respPayload := make(map[string]any)
	respData, _ := json.Marshal(resp)
	_ = json.Unmarshal(respData, &respPayload)

	return e.spine.Publish(ctx, infrastructure.Event{
		ObjectID: event.ObjectID,
		Kind:     "federated_query",
		Op:       "response",
		Payload:  respPayload,
	})
}

// ResponseListener waits for a response to a specific request ID.
func (e *FederatedQueryEngine) ResponseListener(ctx context.Context, requestID string) (*QueryResponse, error) {
	e.mu.RLock()
	ch, ok := e.pending[requestID]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no pending request for ID %s", requestID)
	}

	defer func() {
		e.mu.Lock()
		delete(e.pending, requestID)
		e.mu.Unlock()
	}()

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for response to %s", requestID)
	}
}

func (e *FederatedQueryEngine) handleIncomingResponse(_ context.Context, event infrastructure.Event) error {
	e.mu.RLock()
	ch, ok := e.pending[event.ObjectID]
	e.mu.RUnlock()

	if !ok {
		return nil
	}

	var resp QueryResponse
	respData, _ := json.Marshal(event.Payload)
	_ = json.Unmarshal(respData, &resp)

	// REQ-501: Verify response signature
	valid, err := crypto.GlobalVerifier.Verify(resp.DataToSign(), resp.Signature, resp.PublicKey)
	if err != nil || !valid {
		msg := fmt.Sprintf("ABORT: Signature violation for response %s", event.ObjectID)
		logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [security_violation] [%s] query-engine: %s\n", e.signPulse(msg), msg)).Log()
		return nil
	}

	select {
	case ch <- &resp:
	default:
	}
	return nil
}

func (e *FederatedQueryEngine) SyncOntology(ctx context.Context, layerHashes map[string]string) error {
	data, _ := json.Marshal(layerHashes)
	sig, _ := e.signer.Sign(data)

	msg := fmt.Sprintf("Ontology Sync Pulse (Hash: %x)", sha256.Sum256(data))
	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [ontology_sync] [%s] %s: %s\n", e.signPulse(msg), e.kernelID, msg)).Log()

	return e.spine.Publish(ctx, infrastructure.Event{
		ObjectID: fmt.Sprintf("ONT-SYNC-%d", time.Now().UnixNano()),
		Kind:     "ontology_sync",
		Op:       "broadcast",
		Payload: map[string]any{
			"kernel_id":               e.kernelID,
			"layer_hashes":            layerHashes,
			objects.FieldKeyPublicKey: e.signer.PublicKey(),
			objects.FieldKeySignature: sig,
		},
	})
}
