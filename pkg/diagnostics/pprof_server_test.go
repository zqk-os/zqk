package diagnostics

import (
	"net/http"
	"testing"
)

func TestBuildPprofServer_Timeouts(t *testing.T) {
	srv := BuildPprofServer("localhost:6060", http.DefaultServeMux)
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
