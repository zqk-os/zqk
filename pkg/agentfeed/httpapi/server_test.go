package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// writeStubWakeScripts installs a no-op shell wake membrane so peer_wake.attempted can be true
// under a TempDir project root (production scripts are not copied into the fixture).
func writeStubWakeScripts(t *testing.T, root string) {
	t.Helper()
	dir := paths.ScriptsDirPath(root)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	for _, name := range []string{"wake-agy.sh", "wake-peer-tpm-02.sh"} {
		p := filepath.Join(dir, name)
		if err := fileutil.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), paths.DirPerm755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestServer_SteerAckPending(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	prev := agentfeed.PeerAckAwaitFire
	agentfeed.PeerAckAwaitFire = func(a agentfeed.PeerAckAwait) error { return nil }
	t.Cleanup(func() { agentfeed.PeerAckAwaitFire = prev })
	writeStubWakeScripts(t, root)
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeNotify,
		FeedID:        "AGF-http",
	}); err != nil {
		t.Fatalf("WriteAgentChatChannelConfig: %v", err)
	}

	srv, err := New(Config{ProjectRoot: root, Token: "secret", SkipMCPProbe: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := srv.Handler()

	// Unauthorized
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedSteer, bytes.NewBufferString(`{"message":"x"}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}

	// Health (no auth)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathHealth, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("health: %d", rr.Code)
	}

	// Steer
	body, _ := json.Marshal(map[string]any{
		"message":               "ATTN peer: via http",
		objects.FieldKeyAgentID: "coord",
		"to_agent_id":           "worker-1",
		"await_peer_ack":        true,
	})
	req := httptest.NewRequest(http.MethodPost, pathFeedSteer, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("steer: %d %s", rr.Code, rr.Body.String())
	}
	var steerOut map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &steerOut); err != nil {
		t.Fatal(err)
	}
	eventID, _ := steerOut["event_id"].(string)
	if eventID == "" {
		t.Fatalf("missing event_id: %v", steerOut)
	}
	if steerOut["sender"] != agentfeed.FeedSenderHTTPAPI {
		t.Fatalf("sender=%v", steerOut["sender"])
	}
	if wakeInfo, ok := steerOut["peer_wake"].(map[string]any); !ok || wakeInfo["attempted"] != true {
		t.Fatalf("expected peer_wake with attempted=true in steer output, got: %v", steerOut["peer_wake"])
	}

	// Pending for worker
	req = httptest.NewRequest(http.MethodGet, pathFeedPending+"?agent_id=worker-1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pending: %d %s", rr.Code, rr.Body.String())
	}

	// Ack
	ackBody, _ := json.Marshal(map[string]any{
		"in_reply_to":              eventID,
		objects.FieldKeyAgentID:    "worker-1",
		objects.FieldKeyPersonaRef: "PER-TEST",
		objects.FieldKeySummary:    "got it",
	})
	req = httptest.NewRequest(http.MethodPost, pathFeedAck, bytes.NewReader(ackBody))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ack: %d %s", rr.Code, rr.Body.String())
	}
}

func TestServer_Wake(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStubWakeScripts(t, root)
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeNotify,
		FeedID:        "AGF-wake",
	}); err != nil {
		t.Fatalf("WriteAgentChatChannelConfig: %v", err)
	}

	srv, err := New(Config{ProjectRoot: root, Token: "secret", SkipMCPProbe: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := srv.Handler()

	// 401 without auth
	wakeBody, _ := json.Marshal(map[string]any{
		"message":               "wake peer",
		objects.FieldKeyAgentID: "coord",
	})
	req := httptest.NewRequest(http.MethodPost, pathFeedWake, bytes.NewReader(wakeBody))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without auth, got %d", rr.Code)
	}

	// 200 with auth
	req = httptest.NewRequest(http.MethodPost, pathFeedWake, bytes.NewReader(wakeBody))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("wake: want 200, got %d %s", rr.Code, rr.Body.String())
	}
	var wakeOut map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &wakeOut); err != nil {
		t.Fatal(err)
	}
	if wakeInfo, ok := wakeOut["peer_wake"].(map[string]any); !ok || wakeInfo["attempted"] != true {
		t.Fatalf("expected peer_wake with attempted=true in wake output, got: %v", wakeOut["peer_wake"])
	}
}

func TestServer_OpenAPI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv, err := New(Config{ProjectRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, pathOpenAPI, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi: want 200, got %d %s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("json unmarshal openapi: %v", err)
	}
	if res["openapi"] != "3.0.3" {
		t.Errorf("want openapi 3.0.3, got %v", res["openapi"])
	}
}

func TestServer_RequiresProjectRoot(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestServer_TLSConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv, err := New(Config{
		ProjectRoot: root,
		TLSCertFile: "/tmp/cert.pem",
		TLSKeyFile:  "/tmp/key.pem",
	})
	if err != nil {
		t.Fatalf("New TLS: %v", err)
	}
	if srv.cfg.TLSCertFile != "/tmp/cert.pem" || srv.cfg.TLSKeyFile != "/tmp/key.pem" {
		t.Errorf("TLS config not retained: %v", srv.cfg)
	}
}

func TestServer_Timeouts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv, err := New(Config{ProjectRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	httpSrv := srv.HTTPServer()
	if httpSrv == nil {
		t.Fatal("expected non-nil http.Server")
	}
	if httpSrv.ReadHeaderTimeout <= 0 {
		t.Errorf("expected ReadHeaderTimeout > 0, got %v", httpSrv.ReadHeaderTimeout)
	}
	if httpSrv.ReadTimeout <= 0 {
		t.Errorf("expected ReadTimeout > 0, got %v", httpSrv.ReadTimeout)
	}
	if httpSrv.WriteTimeout <= 0 {
		t.Errorf("expected WriteTimeout > 0, got %v", httpSrv.WriteTimeout)
	}
	if httpSrv.IdleTimeout <= 0 {
		t.Errorf("expected IdleTimeout > 0, got %v", httpSrv.IdleTimeout)
	}
}

func TestServer_NonLoopback_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Non-loopback without token/TLS must fail
	if _, err := New(Config{ProjectRoot: root, ListenAddr: "0.0.0.0:8787"}); err == nil {
		t.Fatal("expected error binding to 0.0.0.0 without token and TLS (F-SEC-005)")
	}
	// Non-loopback with token and TLS succeeds
	if _, err := New(Config{
		ProjectRoot: root,
		ListenAddr:  "0.0.0.0:8787",
		Token:       "secret",
		TLSCertFile: "/tmp/cert.pem",
		TLSKeyFile:  "/tmp/key.pem",
	}); err != nil {
		t.Fatalf("expected success with token and TLS on 0.0.0.0: %v", err)
	}
}
