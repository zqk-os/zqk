package community

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func newTestClusterEndpoint(clusterID, addr, cipher string, now time.Time) ClusterEndpoint {
	return ClusterEndpoint{
		ClusterID: clusterID,
		Address:   addr,
		Manifest: TLSManifest{
			CertFingerprint:   "sha256:cert-" + clusterID,
			RootCAFingerprint: "sha256:root-ca-mesh",
			CipherSuite:       cipher,
			MutualTLS:         true,
			ExpiresAt:         now.Add(24 * time.Hour),
		},
	}
}

func newTestEpistemicBridge(t *testing.T, clusterID, addr string, nowFn func() time.Time) *EpistemicStateBridge {
	t.Helper()
	ep := newTestClusterEndpoint(clusterID, addr, CipherTLS_AES_256_GCM_SHA384, nowFn())
	bridge, err := NewEpistemicStateBridge(BridgeConfig{
		LocalClusterID: clusterID,
		LocalEndpoint:  ep,
		SharedSecret:   []byte("epistemic-bridge-shared-secret-key"),
		SessionTTL:     10 * time.Minute,
		MaxClockSkew:   30 * time.Second,
		NowFunc:        nowFn,
	})
	if err != nil {
		t.Fatalf("failed to create test epistemic bridge for %s: %v", clusterID, err)
	}
	return bridge
}

func assertErrorIs(t *testing.T, got, want error, context string) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("%s: expected error %v, got %v", context, want, got)
	}
}

func setupPairedTestBridges(t *testing.T, nowFn func() time.Time) (*EpistemicStateBridge, *EpistemicStateBridge, ClusterEndpoint, ClusterEndpoint, *HandshakeSession) {
	t.Helper()
	now := nowFn()
	bridgeA := newTestEpistemicBridge(t, "cluster-alpha", "10.0.1.10:8443", nowFn)
	bridgeB := newTestEpistemicBridge(t, "cluster-beta", "10.0.2.20:8443", nowFn)

	epA := newTestClusterEndpoint("cluster-alpha", "10.0.1.10:8443", CipherTLS_AES_256_GCM_SHA384, now)
	epB := newTestClusterEndpoint("cluster-beta", "10.0.2.20:8443", CipherTLS_AES_256_GCM_SHA384, now)

	if err := bridgeA.RegisterCluster(epB); err != nil {
		t.Fatalf("bridgeA failed to register cluster-beta: %v", err)
	}
	if err := bridgeB.RegisterCluster(epA); err != nil {
		t.Fatalf("bridgeB failed to register cluster-alpha: %v", err)
	}

	sess, err := bridgeA.NegotiateHandshake(HandshakeRequest{
		RemoteCluster: epB,
		Nonce:         "nonce-init-001",
		Timestamp:     now,
	})
	if err != nil {
		t.Fatalf("NegotiateHandshake failed: %v", err)
	}

	peerSess := *sess
	peerSess.RemoteClusterID = "cluster-alpha"
	peerSess.RemoteFingerprint = epA.Manifest.CertFingerprint
	if err := bridgeB.AcceptPeerSession(peerSess); err != nil {
		t.Fatalf("AcceptPeerSession failed: %v", err)
	}
	return bridgeA, bridgeB, epA, epB, sess
}

// TestEpistemicBridge_StaticFloorInvariant verifies endpoint descriptor validation,
// mutual TLS manifest checks, and allowed cipher suite enforcement.
func TestEpistemicBridge_StaticFloorInvariant(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	invalidEndpoints := []struct {
		name string
		ep   ClusterEndpoint
		want error
	}{
		{
			name: "blank cluster ID",
			ep:   newTestClusterEndpoint("   ", "10.0.0.1:8443", CipherTLS_AES_256_GCM_SHA384, now),
			want: ErrBlankClusterID,
		},
		{
			name: "blank address",
			ep:   newTestClusterEndpoint("cluster-a", "\t", CipherTLS_AES_256_GCM_SHA384, now),
			want: ErrBlankEndpoint,
		},
		{
			name: "blank cert fingerprint",
			ep: func() ClusterEndpoint {
				e := newTestClusterEndpoint("cluster-a", "10.0.0.1:8443", CipherTLS_AES_256_GCM_SHA384, now)
				e.Manifest.CertFingerprint = ""
				return e
			}(),
			want: ErrBlankCertFingerprint,
		},
		{
			name: "unsupported cipher suite",
			ep:   newTestClusterEndpoint("cluster-a", "10.0.0.1:8443", "TLS_RSA_WITH_AES_128_CBC_SHA", now),
			want: ErrUnsupportedCipherSuite,
		},
		{
			name: "mutual TLS disabled",
			ep: func() ClusterEndpoint {
				e := newTestClusterEndpoint("cluster-a", "10.0.0.1:8443", CipherTLS_AES_256_GCM_SHA384, now)
				e.Manifest.MutualTLS = false
				return e
			}(),
			want: ErrInvalidTLSManifest,
		},
		{
			name: "expired manifest",
			ep: func() ClusterEndpoint {
				e := newTestClusterEndpoint("cluster-a", "10.0.0.1:8443", CipherTLS_AES_256_GCM_SHA384, now)
				e.Manifest.ExpiresAt = now.Add(-1 * time.Second)
				return e
			}(),
			want: ErrInvalidTLSManifest,
		},
	}

	for _, tc := range invalidEndpoints {
		err := ValidateClusterEndpoint(tc.ep, now)
		assertErrorIs(t, err, tc.want, tc.name)
	}

	allowed := []string{
		CipherTLS_AES_256_GCM_SHA384,
		CipherTLS_CHACHA20_POLY1305_SHA256,
		CipherTLS_AES_128_GCM_SHA256,
	}
	for _, cipher := range allowed {
		ep := newTestClusterEndpoint("cluster-ok", "10.0.0.2:8443", cipher, now)
		if err := ValidateClusterEndpoint(ep, now); err != nil {
			t.Fatalf("expected cipher %s to be valid, got %v", cipher, err)
		}
	}

	// Verify bridge config validation rejects empty shared secret.
	_, err := NewEpistemicStateBridge(BridgeConfig{
		LocalClusterID: "cluster-ok",
		LocalEndpoint:  newTestClusterEndpoint("cluster-ok", "10.0.0.2:8443", CipherTLS_AES_256_GCM_SHA384, now),
		SharedSecret:   nil,
	})
	assertErrorIs(t, err, ErrBlankSharedSecret, "empty shared secret")
}

// TestEpistemicBridge_OperationalDynamicProof verifies mutually authenticated
// session negotiation and bidirectional, CAS-verified delta exchange under concurrency.
func TestEpistemicBridge_OperationalDynamicProof(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }

	bridgeA, bridgeB, _, _, sess := setupPairedTestBridges(t, nowFn)

	// Send 3 deltas from A -> B and 2 deltas from B -> A.
	for i := 1; i <= 3; i++ {
		payload := []byte(fmt.Sprintf("delta-a-to-b-%d", i))
		frame, err := bridgeA.SendDelta(sess.SessionID, payload, ComputeCASDigest(payload))
		if err != nil {
			t.Fatalf("SendDelta A->B %d failed: %v", i, err)
		}
		if frame.Sequence != uint64(i) {
			t.Fatalf("expected sequence %d, got %d", i, frame.Sequence)
		}
		if err := bridgeB.ReceiveDelta(frame); err != nil {
			t.Fatalf("ReceiveDelta A->B %d failed: %v", i, err)
		}
	}

	for i := 1; i <= 2; i++ {
		payload := []byte(fmt.Sprintf("delta-b-to-a-%d", i))
		frame, err := bridgeB.SendDelta(sess.SessionID, payload, ComputeCASDigest(payload))
		if err != nil {
			t.Fatalf("SendDelta B->A %d failed: %v", i, err)
		}
		if err := bridgeA.ReceiveDelta(frame); err != nil {
			t.Fatalf("ReceiveDelta B->A %d failed: %v", i, err)
		}
	}

	pendingB, err := bridgeB.PendingDeltas(sess.SessionID)
	if err != nil {
		t.Fatalf("PendingDeltas on bridgeB failed: %v", err)
	}
	if len(pendingB) != 3 {
		t.Fatalf("expected 3 pending deltas on bridgeB, got %d", len(pendingB))
	}

	pendingA, err := bridgeA.PendingDeltas(sess.SessionID)
	if err != nil {
		t.Fatalf("PendingDeltas on bridgeA failed: %v", err)
	}
	if len(pendingA) != 2 {
		t.Fatalf("expected 2 pending deltas on bridgeA, got %d", len(pendingA))
	}

	// Concurrent handshake & delta stress test using goroutinelabels.StartTestGoroutines.
	var seqErrCount atomic.Int64
	var wgMu sync.Mutex
	var sentFrames []StateDeltaFrame
	wg := goroutinelabels.StartTestGoroutines(t, 4, "epistemic_bridge_worker", "concurrent delta send", func(id int) {
		payload := []byte(fmt.Sprintf("concurrent-payload-%d", id))
		f, sendErr := bridgeA.SendDelta(sess.SessionID, payload, ComputeCASDigest(payload))
		if sendErr != nil {
			seqErrCount.Add(1)
			return
		}
		wgMu.Lock()
		sentFrames = append(sentFrames, f)
		wgMu.Unlock()
	})
	wg.Wait()

	if seqErrCount.Load() != 0 || len(sentFrames) != 4 {
		t.Fatalf("expected 4 concurrent sends with 0 errors, got %d frames, %d errors", len(sentFrames), seqErrCount.Load())
	}
}

// TestEpistemicBridge_NegativeAdversarialBoundary verifies rejection of spoofed clusters,
// replayed nonces/frames, expired sessions, forged tokens, CAS mismatches, and clock skew.
func TestEpistemicBridge_NegativeAdversarialBoundary(t *testing.T) {
	currentTime := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return currentTime }

	bridgeA, bridgeB, _, epB, sess := setupPairedTestBridges(t, nowFn)

	// 1. Spoofed / unregistered cluster announcement.
	spoofedEp := newTestClusterEndpoint("cluster-rogue", "10.9.9.9:8443", CipherTLS_AES_256_GCM_SHA384, currentTime)
	_, err := bridgeA.NegotiateHandshake(HandshakeRequest{
		RemoteCluster: spoofedEp,
		Nonce:         "nonce-rogue-1",
		Timestamp:     currentTime,
	})
	assertErrorIs(t, err, ErrSpoofedCluster, "unregistered cluster handshake")

	// Registered cluster ID with mismatched cert fingerprint.
	tamperedEp := epB
	tamperedEp.Manifest.CertFingerprint = "sha256:forged-cert"
	_, err = bridgeA.NegotiateHandshake(HandshakeRequest{
		RemoteCluster: tamperedEp,
		Nonce:         "nonce-rogue-2",
		Timestamp:     currentTime,
	})
	assertErrorIs(t, err, ErrSpoofedCluster, "fingerprint mismatch handshake")

	// 2. Replayed handshake nonce.
	_, err = bridgeA.NegotiateHandshake(HandshakeRequest{
		RemoteCluster: epB,
		Nonce:         "nonce-init-001",
		Timestamp:     currentTime,
	})
	assertErrorIs(t, err, ErrReplayedNonce, "replayed handshake nonce")

	// 3. Clock skew exceeded on handshake.
	_, err = bridgeA.NegotiateHandshake(HandshakeRequest{
		RemoteCluster: epB,
		Nonce:         "nonce-skew-1",
		Timestamp:     currentTime.Add(2 * time.Minute),
	})
	assertErrorIs(t, err, ErrClockSkewExceeded, "handshake clock skew exceeded")

	// 4. Send valid frame 1 and test adversarial mutations on ReceiveDelta.
	payload := []byte("state-delta-1")
	frame1, err := bridgeA.SendDelta(sess.SessionID, payload, ComputeCASDigest(payload))
	if err != nil {
		t.Fatalf("SendDelta failed: %v", err)
	}
	if err := bridgeB.ReceiveDelta(frame1); err != nil {
		t.Fatalf("ReceiveDelta frame1 failed: %v", err)
	}

	// Replayed delta frame.
	assertErrorIs(t, bridgeB.ReceiveDelta(frame1), ErrReplayedFrame, "replayed delta frame")

	// Forged session token on frame 2.
	payload2 := []byte("state-delta-2")
	frame2, err := bridgeA.SendDelta(sess.SessionID, payload2, ComputeCASDigest(payload2))
	if err != nil {
		t.Fatalf("SendDelta frame2 failed: %v", err)
	}
	forgedTokenFrame := frame2
	forgedTokenFrame.SessionToken = "forged-hmac-token"
	assertErrorIs(t, bridgeB.ReceiveDelta(forgedTokenFrame), ErrInvalidSessionToken, "forged session token")

	// CAS digest mismatch on frame 2.
	corruptedFrame := frame2
	corruptedFrame.Payload = []byte("tampered-payload")
	assertErrorIs(t, bridgeB.ReceiveDelta(corruptedFrame), ErrCASDigestMismatch, "CAS digest mismatch")

	// Non-monotonic sequence (skipping sequence 2 -> sending 3).
	skippedFrame := frame2
	skippedFrame.Sequence = 3
	assertErrorIs(t, bridgeB.ReceiveDelta(skippedFrame), ErrNonMonotonicSequence, "skipped sequence")

	// Clock skew exceeded on delta frame.
	skewedFrame := frame2
	skewedFrame.Timestamp = currentTime.Add(-2 * time.Minute)
	assertErrorIs(t, bridgeB.ReceiveDelta(skewedFrame), ErrClockSkewExceeded, "delta clock skew exceeded")

	// 5. Expired handshake session.
	currentTime = currentTime.Add(15 * time.Minute)
	assertErrorIs(t, bridgeB.ReceiveDelta(frame2), ErrSessionExpired, "expired session delta")
}
