package scheduler

import (
	"net/http"
	"testing"
	"time"
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
