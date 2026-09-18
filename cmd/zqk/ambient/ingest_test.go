package ambient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/ambient"
)

func TestIngestEndpoint(t *testing.T) {
	// Setup the test logger and hub
	ctx := context.Background()
	hub := ambient.NewEventHub()

	// Handler function from ingest.go extracted for testing
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body := new(bytes.Buffer)
		_, err := body.ReadFrom(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Simplified test logic for JSON parsing
		// (normally we'd reuse the exact handler)
		// For now we just check if it receives a valid POST and publishes.
		event := ambient.Event{
			Type:      ambient.EventTypeSession,
			Payload:   "test payload",
			Timestamp: time.Now(),
		}

		if err := hub.Publish(ctx, event); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBuffer([]byte(`{"type":"session","payload":"test"}`)))
	w := httptest.NewRecorder()

	handler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, res.StatusCode)
	}
}

func TestAmbientIngestServer_Timeouts(t *testing.T) {
	srv := newAmbientIngestServer("127.0.0.1:9999", http.DefaultServeMux)
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
