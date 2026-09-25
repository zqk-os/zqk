package scheduler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

func TestCallbackListener_Constants(t *testing.T) {
	if defaultCallbackListenerPort != 8080 {
		t.Errorf("expected port 8080, got %d", defaultCallbackListenerPort)
	}
	if defaultCallbackListenerBasePath != "/callbacks" {
		t.Errorf("expected basePath '/callbacks', got %s", defaultCallbackListenerBasePath)
	}
	if defaultCallbackListenerIdleTimeout != 5*time.Minute {
		t.Errorf("expected 5m idle timeout, got %v", defaultCallbackListenerIdleTimeout)
	}
	if callbackServerReadHeaderTimeout != 5*time.Second {
		t.Errorf("expected 5s read header timeout, got %v", callbackServerReadHeaderTimeout)
	}
}

func TestCallbackListenerHandler_BuildHTTPServer(t *testing.T) {
	handler := &CallbackListenerHandler{}
	dummyHandler := http.NewServeMux()
	srv := handler.BuildHTTPServer("127.0.0.1:8080", dummyHandler)

	if srv.Addr != "127.0.0.1:8080" {
		t.Errorf("expected addr '127.0.0.1:8080', got %s", srv.Addr)
	}
	if srv.ReadHeaderTimeout != callbackServerReadHeaderTimeout {
		t.Errorf("expected ReadHeaderTimeout %v, got %v", callbackServerReadHeaderTimeout, srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != callbackServerReadTimeout {
		t.Errorf("expected ReadTimeout %v, got %v", callbackServerReadTimeout, srv.ReadTimeout)
	}
	if srv.WriteTimeout != callbackServerWriteTimeout {
		t.Errorf("expected WriteTimeout %v, got %v", callbackServerWriteTimeout, srv.WriteTimeout)
	}
	if srv.IdleTimeout != callbackServerIdleTimeout {
		t.Errorf("expected IdleTimeout %v, got %v", callbackServerIdleTimeout, srv.IdleTimeout)
	}
}

type testAuthHook struct {
	authenticated bool
	subject       string
	permissions   []string
	err           error
}

func (m *testAuthHook) Authenticate(ctx context.Context, r *http.Request) (bool, string, []string, error) {
	return m.authenticated, m.subject, m.permissions, m.err
}

func TestCallbackListenerAuth(t *testing.T) {
	job := &ScheduledJob{ID: "test-job"}

	t.Run("nil auth hook allows requests (testing mode)", func(t *testing.T) {
		handler := &CallbackListenerHandler{
			authHook: nil,
			logger:   logging.GetLoggerFromProfile("system"),
		}
		req := httptest.NewRequest("POST", "/callbacks/trigger", bytes.NewBufferString(`{"key":"value"}`))
		rr := httptest.NewRecorder()

		handler.handleCallback(rr, req, "trigger", job)
		if rr.Code == http.StatusUnauthorized {
			t.Errorf("expected request to proceed with nil auth hook, got 401")
		}
	})

	t.Run("unauthenticated request returns 401 Unauthorized", func(t *testing.T) {
		handler := &CallbackListenerHandler{
			authHook: &testAuthHook{authenticated: false},
			logger:   logging.GetLoggerFromProfile("system"),
		}
		req := httptest.NewRequest("POST", "/callbacks/trigger", bytes.NewBufferString(`{"key":"value"}`))
		rr := httptest.NewRecorder()

		handler.handleCallback(rr, req, "trigger", job)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("authenticated request succeeds", func(t *testing.T) {
		handler := &CallbackListenerHandler{
			authHook: &testAuthHook{authenticated: true, subject: "test-agent"},
			logger:   logging.GetLoggerFromProfile("system"),
		}
		req := httptest.NewRequest("POST", "/callbacks/trigger", bytes.NewBufferString(`{"key":"value"}`))
		rr := httptest.NewRecorder()

		handler.handleCallback(rr, req, "trigger", job)
		if rr.Code == http.StatusUnauthorized {
			t.Errorf("expected authenticated request not to return 401, got %d", rr.Code)
		}
	})
}
