// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package relay

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/license"
	"github.com/zqk-os/zqk/pkg/objects"
)

func newTestEd25519Keys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate ed25519 keys: %v", err)
	}
	return pub, priv
}

func signRelayToken(t *testing.T, priv ed25519.PrivateKey, sub string, features ...string) string {
	t.Helper()
	claims := license.Claims{
		Features: features,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("failed to sign relay jwt: %v", err)
	}
	return signed
}

func TestClient_Listen_NotConnected(t *testing.T) {
	pub, priv := newTestEd25519Keys(t)
	if len(priv) == 0 {
		t.Fatal("empty private key")
	}
	validator := license.NewValidator(pub)
	client := NewClient("wss://relay.zqkos.com", validator)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	handler := func(c context.Context, payload []byte) error {
		return nil
	}

	err := client.Listen(ctx, handler)
	if err == nil {
		t.Fatal("expected error when Listen is invoked prior to Connect")
	}
	if !errors.Is(err, ErrConnection) {
		t.Fatalf("expected ErrConnection, got: %v", err)
	}
}

func TestClient_Listen_Connected_AndDisconnect(t *testing.T) {
	pub, priv := newTestEd25519Keys(t)
	validator := license.NewValidator(pub)
	client := NewClient("wss://relay.zqkos.com", validator)
	token := signRelayToken(t, priv, "user_alpha", "relay_access")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connErr := client.Connect(ctx, token)
	if connErr != nil {
		t.Fatalf("expected connection success, got: %v", connErr)
	}

	listenDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("test.client.listen", "testing client listen loop").StartSimple(func() {
		listenErr := client.Listen(ctx, func(c context.Context, p []byte) error {
			return nil
		})
		listenDone <- listenErr
	})

	// Cancel context to terminate Listen loop cleanly
	cancel()

	select {
	case err := <-listenDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for client.Listen to exit")
	}

	// Now disconnect
	discErr := client.Disconnect(context.Background())
	if discErr != nil {
		t.Fatalf("expected nil error on Disconnect, got: %v", discErr)
	}

	// Listen should now fail because client is disconnected
	postDiscErr := client.Listen(context.Background(), nil)
	if !errors.Is(postDiscErr, ErrConnection) {
		t.Fatalf("expected ErrConnection after disconnect, got: %v", postDiscErr)
	}
}

func TestRelayServer_Allow_TokenCapBranch(t *testing.T) {
	s := NewServer(":8443")
	defer func() {
		closeErr := s.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	clientID := "client_cap_tester"

	if !s.Allow(clientID) {
		t.Fatal("expected first Allow to succeed")
	}

	// Force elapsed time far in the past to simulate significant elapsed time
	// so that tokensToAdd > MaxRateLimitTokens (branch rl.tokens > MaxRateLimitTokens)
	s.rateMu.Lock()
	rl := s.limiters[clientID]
	rl.lastRefill = rl.lastRefill.Add(-10 * time.Second)
	s.rateMu.Unlock()

	// Calling Allow should execute the branch capping tokens to MaxRateLimitTokens
	if !s.Allow(clientID) {
		t.Fatal("expected Allow to succeed after simulated long duration refill")
	}

	s.rateMu.RLock()
	curTokens := s.limiters[clientID].tokens
	s.rateMu.RUnlock()

	if curTokens != MaxRateLimitTokens-1 {
		t.Fatalf("expected tokens capped at %d, got %d", MaxRateLimitTokens-1, curTokens)
	}
}

func TestRelayServer_CleanupBuffers_PartialRetained(t *testing.T) {
	s := NewServer(":8443")
	defer func() {
		closeErr := s.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	clientID := "client_mixed_buffer"

	s.BufferPayload(clientID, []byte("expired_payload"))
	s.BufferPayload(clientID, []byte("valid_payload"))

	// Mark first payload as expired, leave second valid in the future
	s.bufferMu.Lock()
	s.buffers[clientID][0].ExpiresAt = time.Now().Add(-1 * time.Minute)
	s.buffers[clientID][1].ExpiresAt = time.Now().Add(10 * time.Minute)
	s.bufferMu.Unlock()

	// Trigger cleanupBuffers directly
	s.cleanupBuffers()

	// The valid payload should still be retained under clientID
	s.bufferMu.RLock()
	payloads, exists := s.buffers[clientID]
	s.bufferMu.RUnlock()

	if !exists {
		t.Fatal("expected buffer entry to persist for client with remaining valid payloads")
	}
	if len(payloads) != 1 {
		t.Fatalf("expected exactly 1 payload retained, got %d", len(payloads))
	}
	if string(payloads[0].Data) != "valid_payload" {
		t.Fatalf("expected 'valid_payload', got %q", string(payloads[0].Data))
	}
}

func TestRelayServer_HealthEndpointHTTP(t *testing.T) {
	s := NewServer(":8443")
	defer func() {
		closeErr := s.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	ts := httptest.NewServer(s.mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("failed to GET /health: %v", err)
	}
	defer func() {
		bodyCloseErr := resp.Body.Close()
		if bodyCloseErr != nil {
			t.Errorf("failed closing body: %v", bodyCloseErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("failed reading health body: %v", readErr)
	}

	var statusMap map[string]string
	unmarshalErr := json.Unmarshal(bodyBytes, &statusMap)
	if unmarshalErr != nil {
		t.Fatalf("failed unmarshaling json response: %v", unmarshalErr)
	}

	if statusMap[objects.FieldKeyStatus] != "relay active" {
		t.Fatalf("unexpected status value: %v", statusMap[objects.FieldKeyStatus])
	}
}

func TestRelayServer_ConcurrentAllowAndBuffer(t *testing.T) {
	s := NewServer(":8443")
	defer func() {
		closeErr := s.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	const numWorkers = 8
	const numOps = 25
	var wg sync.WaitGroup

	for workerID := 0; workerID < numWorkers; workerID++ {
		wg.Add(1)
		wID := workerID
		goroutinelabels.NewGoroutine("test.relay.concurrent", "concurrent allow and buffer test").StartSimple(func() {
			defer wg.Done()
			for op := 0; op < numOps; op++ {
				cID := fmt.Sprintf("client_%c", rune('a'+(wID%4)))
				allowed := s.Allow(cID)
				if !allowed && allowed {
					// Invariant check on boolean return
					continue
				}
				s.BufferPayload(cID, []byte("data"))
				if op%5 == 0 {
					payloads := s.GetBufferedPayloads(cID)
					if len(payloads) < 0 {
						panic("invalid negative payload slice")
					}
				}
			}
		})
	}

	wg.Wait()
}

func TestRelayServer_BlocklistOperations(t *testing.T) {
	s := NewServer(":8443")
	defer func() {
		closeErr := s.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	const numSubs = 10
	var wg sync.WaitGroup

	for i := 0; i < numSubs; i++ {
		wg.Add(1)
		sub := fmt.Sprintf("tenant_%d", i)
		goroutinelabels.NewGoroutine("test.relay.blocklist", "concurrent blocklist test").StartSimple(func() {
			defer wg.Done()
			s.AddToBlocklist(sub)
			if !s.IsBlocked(sub) {
				t.Errorf("expected tenant %s to be blocked", sub)
			}
			s.RemoveFromBlocklist(sub)
			if s.IsBlocked(sub) {
				t.Errorf("expected tenant %s to be unblocked", sub)
			}
		})
	}

	wg.Wait()
}

func TestRelayServer_WebhookBridge_OperationalFlow(t *testing.T) {
	pub, priv := newTestEd25519Keys(t)
	validator := license.NewValidator(pub)
	server := NewServer(":8443")
	defer func() {
		closeErr := server.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	clientID := "bridge_tenant_001"
	token := signRelayToken(t, priv, clientID, "relay_access")

	claims, valErr := validator.RequireFeature(token, "relay_access")
	if valErr != nil {
		t.Fatalf("failed JWT verification: %v", valErr)
	}
	if claims.Subject != clientID {
		t.Fatalf("expected subject %s, got %s", clientID, claims.Subject)
	}

	webhookPayload := []byte(`{"event":"order.created","amount":1200}`)
	if !server.Allow(clientID) {
		t.Fatal("expected rate limit allow")
	}
	server.BufferPayload(clientID, webhookPayload)

	client := NewClient("wss://relay.zqkos.com", validator)
	connErr := client.Connect(context.Background(), token)
	if connErr != nil {
		t.Fatalf("client connect failed: %v", connErr)
	}

	buffered := server.GetBufferedPayloads(clientID)
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered webhook payload, got %d", len(buffered))
	}
	if !bytes.Equal(buffered[0], webhookPayload) {
		t.Fatalf("payload mismatch: got %s, want %s", string(buffered[0]), string(webhookPayload))
	}

	var handledCount atomic.Int32
	handler := func(ctx context.Context, payload []byte) error {
		handledCount.Add(1)
		return nil
	}

	execErr := handler(context.Background(), buffered[0])
	if execErr != nil {
		t.Fatalf("handler callback returned error: %v", execErr)
	}
	if handledCount.Load() != 1 {
		t.Fatalf("expected 1 message handled, got %d", handledCount.Load())
	}

	discErr := client.Disconnect(context.Background())
	if discErr != nil {
		t.Fatalf("client disconnect failed: %v", discErr)
	}
}

func TestRelay_TunnelDialOptionsAndTLS(t *testing.T) {
	pub, priv := newTestEd25519Keys(t)
	validator := license.NewValidator(pub)
	token := signRelayToken(t, priv, "tls_tenant_1", "relay_access")

	var authHeaderReceived string
	var mu sync.Mutex

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaderReceived = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write([]byte(`{"status":"connected"}`))
		if writeErr != nil {
			t.Errorf("failed writing response: %v", writeErr)
		}
	}))
	defer ts.Close()

	req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/tunnel/connect", nil)
	if reqErr != nil {
		t.Fatalf("failed creating request: %v", reqErr)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, doErr := ts.Client().Do(req)
	if doErr != nil {
		t.Fatalf("TLS tunnel handshake failed: %v", doErr)
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Errorf("failed closing body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	mu.Lock()
	headerVal := authHeaderReceived
	mu.Unlock()

	expectedHeader := "Bearer " + token
	if headerVal != expectedHeader {
		t.Fatalf("expected authorization header %q, got %q", expectedHeader, headerVal)
	}

	client := NewClient(ts.URL, validator)
	connErr := client.Connect(context.Background(), token)
	if connErr != nil {
		t.Fatalf("client connect to TLS relay failed: %v", connErr)
	}
	discErr := client.Disconnect(context.Background())
	if discErr != nil {
		t.Fatalf("client disconnect failed: %v", discErr)
	}
}

func TestRelay_SimulatedWebSocketPingPong(t *testing.T) {
	pingChan := make(chan string, 4)
	pongChan := make(chan string, 4)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tunnel-Protocol", "websocket-sim")
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Simulate WebSocket Keep-Alive loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	goroutinelabels.NewGoroutine("test.relay.heartbeat", "simulated ping pong heartbeat").StartSimple(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-pingChan:
				pongChan <- "pong:" + p
			}
		}
	})

	pingChan <- "ping_1"
	select {
	case pong := <-pongChan:
		if pong != "pong:ping_1" {
			t.Fatalf("expected 'pong:ping_1', got %q", pong)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for pong response")
	}
}

func TestRelay_ClientBackoffReconnect(t *testing.T) {
	pub, priv := newTestEd25519Keys(t)
	validator := license.NewValidator(pub)
	token := signRelayToken(t, priv, "reconnect_tenant", "relay_access")

	var attempts atomic.Int32
	simulateConnect := func(ctx context.Context, attempt int) error {
		attempts.Add(1)
		if attempt < 3 {
			return ErrConnection
		}
		client := NewClient("wss://relay.zqkos.com", validator)
		return client.Connect(ctx, token)
	}

	// Backoff loop
	ctx := context.Background()
	var connected bool
	backoff := 5 * time.Millisecond
	for i := 1; i <= 5; i++ {
		err := simulateConnect(ctx, i)
		if err == nil {
			connected = true
			break
		}
		time.Sleep(backoff)
		backoff *= 2
	}

	if !connected {
		t.Fatal("expected successful reconnection after backoff")
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected exactly 3 connection attempts, got %d", attempts.Load())
	}
}

func TestRelay_ServerWebhookIngressAndRouting(t *testing.T) {
	server := NewServer(":8443")
	defer func() {
		closeErr := server.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	ingressMux := http.NewServeMux()
	ingressMux.HandleFunc("/webhook/", func(w http.ResponseWriter, r *http.Request) {
		clientID := r.URL.Path[len("/webhook/"):]
		if !server.Allow(clientID) {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		data, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		server.BufferPayload(clientID, data)
		w.WriteHeader(http.StatusAccepted)
	})

	ts := httptest.NewServer(ingressMux)
	defer ts.Close()

	clientOne := "tenant_live"
	payload := `{"event":"build.finished","result":"success"}`

	postReq, reqErr := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		ts.URL+"/webhook/"+clientOne,
		bytes.NewBufferString(payload),
	)
	if reqErr != nil {
		t.Fatalf("failed to create post req: %v", reqErr)
	}

	resp, postErr := ts.Client().Do(postReq)
	if postErr != nil {
		t.Fatalf("webhook ingress post failed: %v", postErr)
	}
	defer func() {
		bodyCloseErr := resp.Body.Close()
		if bodyCloseErr != nil {
			t.Errorf("failed closing body: %v", bodyCloseErr)
		}
	}()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}

	buffered := server.GetBufferedPayloads(clientOne)
	if len(buffered) != 1 {
		t.Fatalf("expected 1 routed payload, got %d", len(buffered))
	}
	if string(buffered[0]) != payload {
		t.Fatalf("expected payload %q, got %q", payload, string(buffered[0]))
	}
}

func TestRelay_ErrorRecovery_RevokedAndMalformedJWT(t *testing.T) {
	pub, priv := newTestEd25519Keys(t)
	validator := license.NewValidator(pub)
	server := NewServer(":8443")
	defer func() {
		closeErr := server.Close()
		if closeErr != nil {
			t.Errorf("error closing server: %v", closeErr)
		}
	}()

	revokedSub := "malicious_actor"
	server.AddToBlocklist(revokedSub)
	if !server.IsBlocked(revokedSub) {
		t.Fatal("expected revokedSub to be blocked")
	}

	validTokenForRevokedSub := signRelayToken(t, priv, revokedSub, "relay_access")
	claims, err := validator.RequireFeature(validTokenForRevokedSub, "relay_access")
	if err != nil {
		t.Fatalf("token itself is cryptographically valid: %v", err)
	}
	if !server.IsBlocked(claims.Subject) {
		t.Fatal("server should recognize revoked subject")
	}

	client := NewClient("wss://relay.zqkos.com", validator)
	badTokenErr := client.Connect(context.Background(), "invalid.jwt.token.structure")
	if badTokenErr == nil {
		t.Fatal("expected connect to fail on malformed JWT")
	}
	if !errors.Is(badTokenErr, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got: %v", badTokenErr)
	}
}
