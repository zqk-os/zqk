package agentdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDeliverer_Names(t *testing.T) {
	t.Parallel()

	if got, want := (HTTPDeliverer{}).Name(), "http"; got != want {
		t.Fatalf("HTTPDeliverer.Name: %q want %q", got, want)
	}
	if got, want := (NoopDeliverer{}).Name(), "noop"; got != want {
		t.Fatalf("NoopDeliverer.Name: %q want %q", got, want)
	}
	if got, want := (Composite{}).Name(), "composite"; got != want {
		t.Fatalf("Composite.Name: %q want %q", got, want)
	}
}

func TestComposite_Deliver(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "a.md")
	c := Composite{Steps: []Deliverer{
		NoopDeliverer{},
		NoopDeliverer{},
	}}
	_, err := c.Deliver(context.Background(), Prompt{Markdown: []byte("x"), DestPath: out})
	if err != nil {
		t.Fatal(err)
	}
}

func TestComposite_NilStep(t *testing.T) {
	t.Parallel()
	c := Composite{Steps: []Deliverer{nil}}
	_, err := c.Deliver(context.Background(), Prompt{Markdown: []byte("x")})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestHTTPDeliverer_Deliver(t *testing.T) {
	t.Parallel()
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type: %q", ct)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	d := HTTPDeliverer{URL: srv.URL}
	res, err := d.Deliver(context.Background(), Prompt{
		Markdown:  []byte("# hi"),
		SessionID: "CVS-1",
		Format:    "agent-prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.DeliveredTo, "http:") {
		t.Fatalf("DeliveredTo: %q", res.DeliveredTo)
	}
	if res.HTTPStatusCode != http.StatusNoContent {
		t.Fatalf("status: %d", res.HTTPStatusCode)
	}
	var payload struct {
		Markdown             string `json:"markdown"`
		ConvergenceSessionID string `json:"convergence_session_id"`
	}
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Markdown != "# hi" || payload.ConvergenceSessionID != "CVS-1" {
		t.Fatalf("payload: %+v", payload)
	}
}

func TestHTTPDeliverer_EmptyURL(t *testing.T) {
	t.Parallel()
	_, err := (HTTPDeliverer{}).Deliver(context.Background(), Prompt{Markdown: []byte("x")})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "empty URL") {
		t.Fatalf("err: %v", err)
	}
}

func TestHTTPDeliverer_WhitespaceOnlyURL(t *testing.T) {
	t.Parallel()
	d := HTTPDeliverer{URL: "  \t "}
	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("x")})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "empty URL") {
		t.Fatalf("err: %v", err)
	}
}

func TestHTTPDeliverer_UnsupportedScheme(t *testing.T) {
	t.Parallel()
	d := HTTPDeliverer{URL: "ftp://example.com/hook"}
	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("x")})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("err: %v", err)
	}
}

func TestHTTPDeliverer_MissingScheme(t *testing.T) {
	t.Parallel()
	d := HTTPDeliverer{URL: "example.com/path"}
	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("x")})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "missing scheme") {
		t.Fatalf("err: %v", err)
	}
}

func TestHTTPDeliverer_Non2xxReturnsErrorAndStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("no"))
	}))
	defer srv.Close()

	d := HTTPDeliverer{URL: srv.URL}
	res, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("x"), SessionID: "s"})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("err: %v", err)
	}
	if res.HTTPStatusCode != http.StatusBadGateway {
		t.Fatalf("HTTPStatusCode: %d", res.HTTPStatusCode)
	}
	if res.DeliveredTo == "" {
		t.Fatal("want DeliveredTo set on error path")
	}
}

func TestHTTPDeliverer_CustomClient(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := HTTPDeliverer{URL: srv.URL, Client: srv.Client()}
	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("body"), SessionID: "id", Format: "agent-prompt"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPDeliverer_URLTrimSpace(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := HTTPDeliverer{URL: "  " + srv.URL + "  "}
	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("z")})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPDeliverer_ContextCancel(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(block)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d := HTTPDeliverer{URL: srv.URL}
	_, err := d.Deliver(ctx, Prompt{Markdown: []byte("x")})
	if err == nil {
		t.Fatal("want error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestHTTPDeliverer_Bearer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := HTTPDeliverer{URL: srv.URL, BearerToken: "tok"}
	_, err := d.Deliver(context.Background(), Prompt{Markdown: []byte("x"), SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppendDeliveryAuditJSONL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec := &DeliveryAuditRecord{
		ConvergenceSessionID: "CVS-x",
		Format:               "agent-prompt",
		DeliverMode:          "none",
		PrimaryDestination:   "stdout",
		DeliveredTo:          []string{"stdout"},
		MarkdownBytes:        3,
	}
	AppendDeliveryAuditJSONL(dir, rec)
	path := AgentPromptDeliveriesJSONLPath(dir)
	b, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got DeliveryAuditRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &got); err != nil {
		t.Fatal(err)
	}
	if got.EventType != EventTypeAgentPromptDelivery {
		t.Fatalf("EventType: %v", got.EventType)
	}
	if got.ConvergenceSessionID != "CVS-x" {
		t.Fatalf("ConvergenceSessionID: %v", got.ConvergenceSessionID)
	}
	if got.MarkdownBytes != 3 {
		t.Fatalf("MarkdownBytes: %v", got.MarkdownBytes)
	}
}

func TestAppendDeliveryAuditJSONL_NilRecord(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	AppendDeliveryAuditJSONL(dir, nil)
	path := AgentPromptDeliveriesJSONLPath(dir)
	if _, err := fileutil.Stat(path); !fileutil.IsNotExist(err) {
		t.Fatalf("expected no file for nil record, stat err=%v", err)
	}
}

func TestAppendDeliveryAuditJSONL_AppendsSecondLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	AppendDeliveryAuditJSONL(dir, &DeliveryAuditRecord{
		ConvergenceSessionID: "a",
		Format:               "agent-prompt",
		DeliverMode:          "none",
		PrimaryDestination:   "stdout",
		DeliveredTo:          []string{"stdout"},
		MarkdownBytes:        1,
	})
	AppendDeliveryAuditJSONL(dir, &DeliveryAuditRecord{
		ConvergenceSessionID: "b",
		Format:               "agent-prompt",
		DeliverMode:          "none",
		PrimaryDestination:   "stdout",
		DeliveredTo:          []string{"stdout"},
		MarkdownBytes:        2,
	})
	path := AgentPromptDeliveriesJSONLPath(dir)
	b, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), string(b))
	}
	var first, second DeliveryAuditRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if first.ConvergenceSessionID != "a" || second.ConvergenceSessionID != "b" {
		t.Fatalf("ids: %v %v", first.ConvergenceSessionID, second.ConvergenceSessionID)
	}
}

func TestAgentPromptDeliveriesJSONLPath_EmptyRoot(t *testing.T) {
	t.Parallel()
	p := AgentPromptDeliveriesJSONLPath("")
	if !strings.Contains(p, AgentPromptDeliveriesJSONLFile) {
		t.Fatalf("path: %s", p)
	}
}
