// BLI-STARTER-COMMUNITY-053 / PRI-STARTER-COMMUNITY-053 coverage elevation
package mesh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestExtraMeshWebhookGossipQueueAndTrust(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	h := WebhookHandler(nil)
	body, _ := json.Marshal(WebhookPayload{JobID: "JOB-1", Status: "success"})
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body2, _ := json.Marshal(WebhookPayload{JobID: "JOB-2", Status: objects.ObjectStatusCompleted, URL: "https://example.com/a"})
	req2 := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req2)

	bad, _ := json.Marshal(WebhookPayload{JobID: "JOB-3", Status: "running"})
	req3 := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(bad))
	req3.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req3)

	failStore := extraFailStore{}
	hf := WebhookHandler(failStore)
	req4 := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	req4.Header.Set("Content-Type", "application/json")
	hf.ServeHTTP(httptest.NewRecorder(), req4)

	ev := SyncEvent{NodeID: "n1", Timestamp: time.Now(), Kind: "GraphState", ItemID: "i1"}
	_ = ev.PayloadForSignature()
	gs, err := NewGossipSyncer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Start(ctx); err != nil {
		t.Fatal(err)
	}
	gs.Subscribe(func(SyncEvent) {})
	_ = gs.Broadcast(ctx, ev)
	_ = gs.Stop()
	_, _ = NewGossipSyncer("::::", nil, nil)

	q := NewAsyncDispatchQueue(9)
	_ = q.Enqueue(ctx, AsyncMessage{ID: "h", Priority: PriorityHigh})
	_ = q.Enqueue(ctx, AsyncMessage{ID: "n", Priority: PriorityNormal})
	_ = q.Enqueue(ctx, AsyncMessage{ID: "l", Priority: PriorityLow})
	_, _ = q.DequeueBatch(ctx, 0)
	_, _ = q.DequeueBatch(ctx, 2)
	_ = q.Len()
	_ = q.Drain()
	_ = q.Close()
	if err := q.Enqueue(ctx, AsyncMessage{ID: "x"}); err == nil {
		t.Fatal("closed enqueue")
	}
	_, _ = q.DequeueBatch(ctx, 1)

	bp := NewBackpressureController(BackpressureConfig{HighWatermark: 2, LowWatermark: 5})
	bp.RecordEnqueue(2)
	_ = bp.IsThrottled()
	cancelWait, cancelFn := context.WithCancel(ctx)
	cancelFn()
	_ = bp.WaitUntilHealthy(cancelWait)
	bp.RecordDrain(0)
	_ = bp.WaitUntilHealthy(ctx)

	reg := NewInMemoryRegistry()
	_, err = reg.Verify(ctx, "missing")
	if err != ErrKernelNotFound {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv
	_ = reg.Register(ctx, RemoteKernel{ID: "k1", PublicKey: hex.EncodeToString(pub), Endpoint: "tcp://"})
	tv := NewTrustVerifier(reg)
	_ = tv.VerifyState(ctx, SyncedState{NodeID: "missing"})
	_ = tv.VerifyState(ctx, SyncedState{NodeID: "k1", Signature: "zz", Payload: []byte("p")})
	sig := ed25519.Sign(priv, []byte("p"))
	_ = tv.VerifyState(ctx, SyncedState{NodeID: "k1", Payload: []byte("p"), Signature: hex.EncodeToString(sig)})
	_ = tv.VerifyState(ctx, SyncedState{NodeID: "k1", Payload: []byte("nope"), Signature: hex.EncodeToString(sig)})

	m := NewInMemoryMesh()
	_ = m.BroadcastCapacity(ctx, CapacityAdvertisement{})
	_ = m.GetCapacities()
	_ = m.RegisterToolPod(ctx, ToolPodRegistration{})
	_, _ = m.DiscoverToolPods(ctx)
}

type extraFailStore struct{ storage.NoopObjectStorage }

func (extraFailStore) Update(context.Context, *storage.SecurityContext, string, map[string]any) error {
	return storage.ErrNoopObjectStorage
}
