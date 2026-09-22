// BLI-STARTER-COMMUNITY-046 / PRI-STARTER-COMMUNITY-046 coverage elevation
package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
)

func TestExtraAdaptersValidateAndSend(t *testing.T) {
	ctx := context.Background()
	msg := types.Message{
		EventType: "job.done",
		Source:    "scheduler",
		Timestamp: time.Now().UTC(),
		Payload:   map[string]any{"ok": true},
		Metadata:  map[string]string{"job_id": "SCH-1"},
	}

	httpAd := NewHTTPAdapter(nil)
	if httpAd.Name() != "webhook" {
		t.Fatal(httpAd.Name())
	}
	if err := httpAd.Validate(types.Action{}); err == nil {
		t.Fatal("empty endpoint")
	}
	if err := httpAd.Validate(types.Action{Endpoint: "ftp://x"}); err == nil {
		t.Fatal("bad scheme")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	act := types.Action{Endpoint: srv.URL, Auth: &types.AuthConfig{Type: "bearer", Credentials: "tok", Headers: map[string]string{"X-A": "1"}}}
	if err := httpAd.Validate(act); err != nil {
		t.Fatal(err)
	}
	if err := httpAd.Send(ctx, msg, act); err != nil {
		t.Fatal(err)
	}
	httpAd.applyAuth(httptest.NewRequest(http.MethodPost, srv.URL, nil), &types.AuthConfig{Type: "basic", Credentials: "u:p"})
	httpAd.applyAuth(httptest.NewRequest(http.MethodPost, srv.URL, nil), &types.AuthConfig{Type: "api_key", Credentials: "k"})
	httpAd.applyAuth(httptest.NewRequest(http.MethodPost, srv.URL, nil), &types.AuthConfig{Type: "jwt", Credentials: "j"})
	httpAd.applyAuth(httptest.NewRequest(http.MethodPost, srv.URL, nil), &types.AuthConfig{Type: "unknown", Credentials: "x"})
	t.Setenv("TDE_TOKEN", "envtok")
	_ = httpAd.resolveCredentials("$TDE_TOKEN")
	_ = httpAd.resolveCredentials("secret:foo")
	_ = httpAd.resolveCredentials("plain")
	_ = httpAd.toHeaderCase("job_id")

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("no"))
	}))
	t.Cleanup(failSrv.Close)
	if err := httpAd.Send(ctx, msg, types.Action{Endpoint: failSrv.URL}); err == nil {
		t.Fatal("non-2xx")
	}

	ev := NewEventAdapter(nil)
	if ev.Name() != "event" {
		t.Fatal(ev.Name())
	}
	if err := ev.Validate(types.Action{}); err == nil {
		t.Fatal("event endpoint")
	}
	if err := ev.Validate(types.Action{Endpoint: "evt"}); err != nil {
		t.Fatal(err)
	}
	if err := ev.Send(ctx, msg, types.Action{Endpoint: "evt"}); err != nil {
		t.Fatal(err)
	}

	cmd := NewCommandAdapter(nil)
	if cmd.Name() != "command" {
		t.Fatal(cmd.Name())
	}
	if err := cmd.Validate(types.Action{}); err == nil {
		t.Fatal("cmd endpoint")
	}
	if err := cmd.Send(ctx, msg, types.Action{Endpoint: "true", Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Send(ctx, msg, types.Action{Endpoint: "false"}); err == nil {
		t.Fatal("false cmd")
	}
	if err := cmd.Send(ctx, msg, types.Action{Endpoint: ""}); err == nil {
		t.Fatal("empty argv")
	}
}
