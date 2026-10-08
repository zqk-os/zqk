package ambient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestIngestEndpoint(t *testing.T) {
	ctx := context.Background()
	hub := ambient.NewEventHub()
	logger := logging.GetLoggerFromContext(ctx)
	handler := newAmbientIngestHandler(ctx, hub, logger)

	// 1. Method Not Allowed
	req := httptest.NewRequest(http.MethodGet, "/ingest", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)

	// 2. Bad JSON
	req = httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBuffer([]byte(`{invalid-json`)))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)

	// 3. Valid JSON with type and timestamp
	payloadWithTS := `{"type":"session","payload":"test-data","timestamp":"2026-10-07T20:00:00Z"}`
	req = httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBuffer([]byte(payloadWithTS)))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Result().StatusCode)

	// 4. Valid JSON with empty type and without timestamp
	payloadEmptyType := `{"payload":"fallback-data"}`
	req = httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewBuffer([]byte(payloadEmptyType)))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Result().StatusCode)
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
