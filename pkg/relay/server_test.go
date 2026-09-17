package relay

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBlocklist(t *testing.T) {
	s := NewServer(":8443")
	subID := "user123"

	if s.IsBlocked(subID) {
		t.Errorf("expected %s not to be blocked initially", subID)
	}

	s.AddToBlocklist(subID)
	if !s.IsBlocked(subID) {
		t.Errorf("expected %s to be blocked", subID)
	}

	s.RemoveFromBlocklist(subID)
	if s.IsBlocked(subID) {
		t.Errorf("expected %s to be unblocked", subID)
	}
}

func TestRateLimiter(t *testing.T) {
	s := NewServer(":8443")
	clientID := "client_abc"

	// Initial token bucket should have 5 tokens.
	// First call consumes 1, returns true.
	for i := 0; i < MaxRateLimitTokens; i++ {
		if !s.Allow(clientID) {
			t.Errorf("expected Allow to return true on iteration %d", i)
		}
	}

	// 6th call should be false
	if s.Allow(clientID) {
		t.Errorf("expected Allow to return false when rate limit exceeded")
	}

	// Wait for refill (at least 1/RateLimitRefillSec seconds)
	// Let's manually manipulate lastRefill to avoid slow tests
	s.rateMu.Lock()
	rl := s.limiters[clientID]
	rl.lastRefill = rl.lastRefill.Add(-time.Second) // simulate 1 second passed
	s.rateMu.Unlock()

	// Should have refilled 5 tokens
	if !s.Allow(clientID) {
		t.Errorf("expected Allow to return true after refill")
	}
}

func TestBufferPayload(t *testing.T) {
	s := NewServer(":8443")
	clientID := "client_xyz"
	payload1 := []byte("hello")
	payload2 := []byte("world")

	// Initially empty
	if payloads := s.GetBufferedPayloads(clientID); payloads != nil {
		t.Errorf("expected nil payloads initially")
	}

	s.BufferPayload(clientID, payload1)
	s.BufferPayload(clientID, payload2)

	payloads := s.GetBufferedPayloads(clientID)
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if !bytes.Equal(payloads[0], payload1) {
		t.Errorf("payload1 mismatch")
	}
	if !bytes.Equal(payloads[1], payload2) {
		t.Errorf("payload2 mismatch")
	}

	// Buffer should be empty after GetBufferedPayloads
	if len(s.GetBufferedPayloads(clientID)) != 0 {
		t.Errorf("expected buffer to be empty after retrieval")
	}
}

func TestBufferExpiration(t *testing.T) {
	s := NewServer(":8443")
	clientID := "client_exp"
	payload := []byte("expire_me")

	s.BufferPayload(clientID, payload)

	// Force expiration
	s.bufferMu.Lock()
	s.buffers[clientID][0].ExpiresAt = time.Now().Add(-time.Second)
	s.bufferMu.Unlock()

	payloads := s.GetBufferedPayloads(clientID)
	if len(payloads) != 0 {
		t.Errorf("expected expired payload to be filtered out")
	}
}

func TestCleanup(t *testing.T) {
	s := NewServer(":8443")
	clientID := "client_clean"

	// Add an expired payload
	s.BufferPayload(clientID, []byte("data"))
	s.bufferMu.Lock()
	s.buffers[clientID][0].ExpiresAt = time.Now().Add(-time.Second)
	s.bufferMu.Unlock()

	// Add an old rate limiter
	s.rateMu.Lock()
	s.limiters[clientID] = &rateLimit{
		tokens:     5,
		lastRefill: time.Now().Add(-2 * time.Minute),
	}
	s.rateMu.Unlock()

	// Run cleanups
	s.cleanupBuffers()
	s.cleanupLimiters()

	// Verify cleanup
	s.bufferMu.RLock()
	if _, exists := s.buffers[clientID]; exists {
		t.Errorf("expected expired buffer to be cleaned up")
	}
	s.bufferMu.RUnlock()

	s.rateMu.RLock()
	if _, exists := s.limiters[clientID]; exists {
		t.Errorf("expected old limiter to be cleaned up")
	}
	s.rateMu.RUnlock()
}

func TestRoutes(t *testing.T) {
	s := NewServer(":8443")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	s.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}
	if rr.Body.String() != "{\"status\":\"relay active\"}\n" {
		t.Errorf("expected '{\"status\":\"relay active\"}\\n', got %q", rr.Body.String())
	}
}

func TestStart_Error(t *testing.T) {
	s := NewServer("invalid-addr")
	err := s.Start()
	if err == nil {
		t.Errorf("expected error starting server with invalid address")
	}
}

func TestRelayServer_Timeouts(t *testing.T) {
	s := NewServer(":8443")
	srv := s.BuildHTTPServer()
	if srv == nil {
		t.Fatal("expected non-nil http.Server")
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("expected ReadHeaderTimeout > 0, got %v", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout <= 0 {
		t.Errorf("expected ReadTimeout > 0, got %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout <= 0 {
		t.Errorf("expected WriteTimeout > 0, got %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Errorf("expected IdleTimeout > 0, got %v", srv.IdleTimeout)
	}
}
