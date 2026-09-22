// BLI-STARTER-COMMUNITY-037 / PRI-STARTER-COMMUNITY-037 coverage elevation
package federation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type stubTransport struct {
	resp *HandshakeResponse
	err  error
}

func (s stubTransport) SendHandshake(context.Context, string, HandshakeRequest) (*HandshakeResponse, error) {
	return s.resp, s.err
}
func (s stubTransport) SendHeartbeat(context.Context, string, string) error { return nil }
func (s stubTransport) ExecuteTool(context.Context, string, string, map[string]any) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func TestExtraIdentityAndHandshakeMap(t *testing.T) {
	mgr := NewIdentityManager(t.TempDir())
	id, err := mgr.GetKernelID()
	if err != nil || !strings.HasPrefix(id, KernelIDPrefix+"-") {
		t.Fatalf("kernel id = %q err=%v", id, err)
	}
	pub, err := mgr.GetPublicKey()
	if err != nil || !strings.HasPrefix(pub, PublicKeyPrefix+"-") {
		t.Fatalf("public key = %q err=%v", pub, err)
	}

	old := globalIdentityManagerProvider
	t.Cleanup(func() { globalIdentityManagerProvider = old })
	RegisterIdentityManagerProvider(func() *IdentityManager { return mgr })
	if GetIdentityManager() != mgr {
		t.Fatal("registered provider not used")
	}
	globalIdentityManagerProvider = nil
	if GetIdentityManager() == nil {
		t.Fatal("nil provider fallback")
	}

	state := &RemoteKernelState{
		ID:               "peer-1",
		Endpoint:         "http://peer",
		PublicKey:        "PUB-X",
		SharedNamespaces: []string{"ns"},
		Capabilities:     []Capability{{ID: "cap-1", Kind: "resource", Name: "r"}},
		LastHandshake:    time.Unix(0, 0).UTC(),
	}
	m := state.ToMap()
	if m[objects.FieldKeyID] != "REM-peer-1" {
		t.Fatalf("id = %v", m[objects.FieldKeyID])
	}
	state.ID = "REM-already"
	if state.ToMap()[objects.FieldKeyID] != "REM-already" {
		t.Fatal("existing REM prefix rewritten")
	}
}

func TestExtraTransportAndMCPHandshaker(t *testing.T) {
	old := defaultTransport
	t.Cleanup(func() { defaultTransport = old })

	SetDefaultTransport(stubTransport{resp: &HandshakeResponse{Accepted: true, KernelID: "k"}})
	if GetDefaultTransport() == nil {
		t.Fatal("default transport nil")
	}
	SetDefaultTransport(nil)
	if _, ok := GetDefaultTransport().(*LocalCLITransport); !ok {
		t.Fatal("expected LocalCLITransport fallback")
	}

	tr := NewLocalCLITransport("")
	if tr.BinaryPath != zqkenv.Bin().Get() {
		t.Fatalf("empty path = %q", tr.BinaryPath)
	}
	if err := tr.SendHeartbeat(context.Background(), "ep", "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.ExecuteTool(context.Background(), "", "unsupported", nil); err == nil {
		t.Fatal("expected unsupported tool")
	}
	_, err := tr.ExecuteTool(context.Background(), t.TempDir(), "object_list", map[string]any{
		objects.FieldKeyKind: "backlog_item",
		"filters":            map[string]any{"status": "planned"},
	})
	if err == nil {
		t.Fatal("expected missing binary failure for object_list")
	}
	_, err = tr.ExecuteTool(context.Background(), "", "object_read", map[string]any{objects.FieldKeyID: "BLI-1"})
	if err == nil {
		t.Fatal("expected missing binary failure for object_read")
	}
	tr.BinaryPath = t.TempDir() + "/no-such-zqk"
	if _, err := tr.SendHandshake(context.Background(), "ep", HandshakeRequest{KernelID: "k"}); err == nil {
		t.Fatal("expected handshake exec failure")
	}

	dir := t.TempDir()
	okBin := filepath.Join(dir, "zqk-ok")
	badBin := filepath.Join(dir, "zqk-bad")
	writeExec(t, okBin, "#!/bin/sh\necho '{\"accepted\":true,\"kernel_id\":\"k\",\"public_key\":\"p\"}'\n")
	writeExec(t, badBin, "#!/bin/sh\necho not-json\n")
	okTr := NewLocalCLITransport(okBin)
	hs, err := okTr.SendHandshake(context.Background(), dir, HandshakeRequest{KernelID: "k"})
	if err != nil || hs == nil || !hs.Accepted {
		t.Fatalf("handshake ok = %+v %v", hs, err)
	}
	raw, err := okTr.ExecuteTool(context.Background(), dir, "object_list", map[string]any{objects.FieldKeyKind: "backlog_item"})
	if err != nil || len(raw) == 0 {
		t.Fatalf("object_list = %s %v", raw, err)
	}
	raw, err = okTr.ExecuteTool(context.Background(), dir, "object_read", map[string]any{objects.FieldKeyID: "BLI-1"})
	if err != nil || len(raw) == 0 {
		t.Fatalf("object_read = %s %v", raw, err)
	}
	if _, err := NewLocalCLITransport(badBin).SendHandshake(context.Background(), dir, HandshakeRequest{}); err == nil {
		t.Fatal("expected handshake json parse failure")
	}

	h := NewMCPHandshaker("local", "pub", nil)
	if h.Transport == nil {
		t.Fatal("nil transport should default")
	}
	h = NewMCPHandshaker("local", "pub", stubTransport{resp: &HandshakeResponse{Accepted: true, Message: "ok"}})
	resp, err := h.Initiate(context.Background(), "ep", HandshakeRequest{ProtocolVersion: "1"})
	if err != nil || !resp.Accepted {
		t.Fatalf("initiate = %+v %v", resp, err)
	}
	if _, err := h.Accept(context.Background(), HandshakeRequest{}); err == nil {
		t.Fatal("expected Accept CLI placeholder error")
	}
}

func TestExtraQueryEngineBranches(t *testing.T) {
	ctx := context.Background()
	spine := &inMemSpine{}
	signer, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	engine := NewFederatedQueryEngine("kernel-local", spine, signer)
	if err := engine.Start(ctx); err != nil {
		t.Fatal(err)
	}

	_ = QueryRequest{ID: "1", SourceID: "s", Kind: "k", Query: "q"}.DataToSign()
	_ = QueryResponse{RequestID: "1", Status: "ok", Result: map[string]any{"a": 1}}.DataToSign()

	if _, err := engine.ResponseListener(ctx, "missing"); err == nil {
		t.Fatal("expected missing pending")
	}

	unsigned := NewFederatedQueryEngine("kernel-local", spine, nil)
	if got := unsigned.signPulse("m"); got != "unsigned" {
		t.Fatalf("nil signer pulse = %s", got)
	}

	engine.RegisterKernel("kernel-peer", "trustedkey-abcdefgh")
	req := QueryRequest{
		ID: "QRY-mismatch", Query: "q", Kind: "k", SourceID: "kernel-peer",
		PublicKey: "otherkey-ijklmnop", Signature: "nope",
	}
	payload, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	if err := engine.HandleQueryRequest(ctx, infrastructure.Event{ObjectID: "QRY-mismatch", Payload: m}); err != nil {
		t.Fatal(err)
	}

	if err := spine.Publish(ctx, infrastructure.Event{
		Kind: "federated_query", Op: "request",
		Payload: map[string]any{"source_id": "kernel-local"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := spine.Publish(ctx, infrastructure.Event{Kind: "federated_query", Op: "noop"}); err != nil {
		t.Fatal(err)
	}
	if err := spine.Publish(ctx, infrastructure.Event{Kind: "federated_query", Op: "response", ObjectID: "no-pending"}); err != nil {
		t.Fatal(err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	engine.pending["QRY-wait"] = make(chan *QueryResponse, 1)
	if _, err := engine.ResponseListener(canceled, "QRY-wait"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled listener = %v", err)
	}

	if err := engine.SyncOntology(ctx, map[string]string{"L1": "h"}); err != nil {
		t.Fatal(err)
	}

	failing := NewFederatedQueryEngine("kernel-local", &mockSpine{}, failSigner{})
	if _, err := failing.RouteQuery(ctx, "k", "q"); err == nil {
		t.Fatal("expected RouteQuery sign failure")
	}
	if got := failing.signPulse("m"); got != "unsigned" {
		t.Fatalf("failing signPulse = %s", got)
	}
	okSigner, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	reqOK := QueryRequest{ID: "QRY-ok", Query: "q", Kind: "k", SourceID: "kernel-peer", PublicKey: okSigner.PublicKey()}
	sig, err := okSigner.Sign(reqOK.DataToSign())
	if err != nil {
		t.Fatal(err)
	}
	reqOK.Signature = sig
	payloadOK, _ := json.Marshal(reqOK)
	var mOK map[string]any
	_ = json.Unmarshal(payloadOK, &mOK)
	failing.RegisterKernel("kernel-peer", okSigner.PublicKey())
	if err := failing.HandleQueryRequest(ctx, infrastructure.Event{ObjectID: "QRY-ok", Payload: mOK}); err == nil {
		t.Fatal("expected response sign failure")
	}
}

type failSigner struct{}

func (failSigner) Sign([]byte) (string, error) { return "", errors.New("nope") }
func (failSigner) PublicKey() string           { return "pk-fail-signer" }

func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
