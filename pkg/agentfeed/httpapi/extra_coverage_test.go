// BLI-STARTER-COMMUNITY-029 / PRI-STARTER-COMMUNITY-029 coverage elevation
package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_DefaultListenAddr(t *testing.T) {
	t.Parallel()
	srv, err := New(Config{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if srv.cfg.ListenAddr != "127.0.0.1:8787" {
		t.Fatalf("ListenAddr=%q", srv.cfg.ListenAddr)
	}
}

func TestServer_MethodNotAllowedAndValidation(t *testing.T) {
	t.Parallel()
	srv, err := New(Config{ProjectRoot: t.TempDir(), SkipMCPProbe: true})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathHealth, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health POST %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathOpenAPI, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("openapi POST %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedSteer, bytes.NewBufferString(`{"message":""}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty steer %d %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedSteer, bytes.NewBufferString(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathFeedSteer, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("steer GET %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathFeedPending, nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("pending no agent %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathFeedPending+"?agent_id=a&limit=-1", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad limit %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedPending, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("pending POST %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedWake, bytes.NewBufferString(`{"message":"  "}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty wake %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathFeedWake, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wake GET %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedAck, bytes.NewBufferString(`{"in_reply_to":"x"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("incomplete ack %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathFeedAck, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ack GET %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, pathFeedPending+"?agent_id=a&limit=0", nil))
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Fatalf("limit 0: %d %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedSteer, bytes.NewBufferString(`{"message":"x","nope":true}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown field %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, pathFeedSteer, bytes.NewBufferString(`{"message":"hi","to_agent_id":"worker"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("hourglass %d %s", rr.Code, rr.Body.String())
	}
}

func TestServer_ListenAndServeCancel(t *testing.T) {
	t.Parallel()
	srv, err := New(Config{ProjectRoot: t.TempDir(), ListenAddr: "127.0.0.1:0", SkipMCPProbe: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := srv.ListenAndServe(ctx); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}
}
