package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/transport"
)

const (
	MaxRateLimitTokens = 5
	RateLimitRefillSec = 5 // Refill 5 tokens per second
	BufferTTL          = 60 * time.Second
)

type BufferedPayload struct {
	Data      []byte
	ExpiresAt time.Time
}

type rateLimit struct {
	tokens     int
	lastRefill time.Time
}

// RelayServer represents the Sovereign Relay server.
type RelayServer struct {
	addr string
	mux  *http.ServeMux

	bufferMu sync.RWMutex
	buffers  map[string][]BufferedPayload

	rateMu   sync.RWMutex
	limiters map[string]*rateLimit

	blockMu   sync.RWMutex
	blocklist map[string]struct{}
}

// NewServer creates a new Sovereign Relay Server.
func NewServer(addr string) *RelayServer {
	s := &RelayServer{
		addr:      addr,
		mux:       http.NewServeMux(),
		buffers:   make(map[string][]BufferedPayload),
		limiters:  make(map[string]*rateLimit),
		blocklist: make(map[string]struct{}),
	}

	s.routes()
	go s.cleanupLoop()

	return s
}

func (s *RelayServer) routes() {
	opts := transport.Options{
		HandlerName: "relay_health",
	}
	s.mux.HandleFunc("/health", transport.NewHTTPHandler(opts, func(req transport.Request[any]) (transport.Response[map[string]string], error) {
		return transport.Response[map[string]string]{
			StatusCode: http.StatusOK,
			Body:       map[string]string{objects.FieldKeyStatus: "relay active"},
		}, nil
	}))
	// Webhook and WebSocket endpoints would be added here in the future
}

// BuildHTTPServer creates an http.Server configured with standard timeouts.
func (s *RelayServer) BuildHTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Start boots the HTTP server with graceful shutdown handling.
func (s *RelayServer) Start() error {
	slog.Info("[Sovereign Relay] Booting node", "addr", s.addr)

	srv := s.BuildHTTPServer()

	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func() {
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
				<-sigChan
				slog.Info("[Sovereign Relay] Shutting down gracefully...")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(ctx)
			}()
		})

	slog.Info("[Sovereign Relay] Listening", "addr", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("relay server error: %v", err)
	}

	return nil
}

// AddToBlocklist explicitly revokes a JWT sub ID.
func (s *RelayServer) AddToBlocklist(subID string) {
	s.blockMu.Lock()
	defer s.blockMu.Unlock()
	s.blocklist[subID] = struct{}{}
}

// RemoveFromBlocklist removes a JWT sub ID from the blocklist.
func (s *RelayServer) RemoveFromBlocklist(subID string) {
	s.blockMu.Lock()
	defer s.blockMu.Unlock()
	delete(s.blocklist, subID)
}

// IsBlocked returns true if the JWT sub ID is explicitly revoked.
func (s *RelayServer) IsBlocked(subID string) bool {
	s.blockMu.RLock()
	defer s.blockMu.RUnlock()
	_, exists := s.blocklist[subID]
	return exists
}

// Allow checks if the client ID has not exceeded the rate limit (5 req/sec).
func (s *RelayServer) Allow(clientID string) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	now := time.Now()
	rl, exists := s.limiters[clientID]
	if !exists {
		s.limiters[clientID] = &rateLimit{
			tokens:     MaxRateLimitTokens - 1,
			lastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(rl.lastRefill).Seconds()
	tokensToAdd := int(elapsed * RateLimitRefillSec)
	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > MaxRateLimitTokens {
			rl.tokens = MaxRateLimitTokens
		}
		// Adjust lastRefill based on exactly how many tokens we added in time
		addedDuration := time.Duration((float64(tokensToAdd) / RateLimitRefillSec) * float64(time.Second))
		rl.lastRefill = rl.lastRefill.Add(addedDuration)
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// BufferPayload temporarily buffers a payload for a disconnected client.
func (s *RelayServer) BufferPayload(clientID string, payload []byte) {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()

	s.buffers[clientID] = append(s.buffers[clientID], BufferedPayload{
		Data:      payload,
		ExpiresAt: time.Now().Add(BufferTTL),
	})
}

// GetBufferedPayloads retrieves and removes all valid payloads for a client.
func (s *RelayServer) GetBufferedPayloads(clientID string) [][]byte {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()

	payloads, exists := s.buffers[clientID]
	if !exists {
		return nil
	}

	var valid [][]byte
	now := time.Now()

	for _, p := range payloads {
		if now.Before(p.ExpiresAt) {
			valid = append(valid, p.Data)
		}
	}

	delete(s.buffers, clientID)
	return valid
}

func (s *RelayServer) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.cleanupBuffers()
		s.cleanupLimiters()
	}
}

func (s *RelayServer) cleanupBuffers() {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()

	now := time.Now()
	for clientID, payloads := range s.buffers {
		var valid []BufferedPayload
		for _, p := range payloads {
			if now.Before(p.ExpiresAt) {
				valid = append(valid, p)
			}
		}
		if len(valid) == 0 {
			delete(s.buffers, clientID)
		} else {
			s.buffers[clientID] = valid
		}
	}
}

func (s *RelayServer) cleanupLimiters() {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	now := time.Now()
	for clientID, rl := range s.limiters {
		if now.Sub(rl.lastRefill) > time.Minute {
			delete(s.limiters, clientID)
		}
	}
}
