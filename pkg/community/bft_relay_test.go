package community

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testBFTRelayClusterID = "cluster-bft-alpha"
	testBFTForeignClus    = "cluster-bft-foreign"
	testBFTOriginID       = "peer-origin-01"
	testBFTRelayAID       = "peer-relay-a"
	testBFTRelayBID       = "peer-relay-b"
	testBFTLeafCID        = "peer-leaf-c"
	testBFTMsgID          = "msg-bft-001"
	testBFTPayload        = "bft-consensus-checkpoint-state"
)

type bftTestPeer struct {
	id   string
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
	node *BFTRelayNode
}

func newTestBFTPeer(t *testing.T, peerID, clusterID string, maxHops uint32) bftTestPeer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	node, err := NewBFTRelayNode(BFTRelayConfig{
		LocalPeerID:  peerID,
		ClusterID:    clusterID,
		PrivateKey:   priv,
		MaxHops:      maxHops,
		DefaultTTL:   defaultBFTRelayTTL,
		MaxSeenCache: defaultBFTRelaySeenCap,
	})
	require.NoError(t, err)
	return bftTestPeer{id: peerID, pub: pub, priv: priv, node: node}
}

func registerPeerMesh(t *testing.T, peers ...bftTestPeer) {
	t.Helper()
	for _, dst := range peers {
		for _, src := range peers {
			if dst.id == src.id {
				continue
			}
			require.NoError(t, dst.node.RegisterPeer(src.id, testBFTRelayClusterID, src.pub))
		}
	}
}

func TestBFTRelay_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	baseCfg := BFTRelayConfig{
		LocalPeerID:  testBFTOriginID,
		ClusterID:    testBFTRelayClusterID,
		PrivateKey:   priv,
		MaxHops:      defaultBFTRelayMaxHops,
		DefaultTTL:   defaultBFTRelayTTL,
		MaxSeenCache: defaultBFTRelaySeenCap,
	}

	cfgCases := []struct {
		name    string
		mutate  func(*BFTRelayConfig)
		wantErr error
	}{
		{"blank peer id", func(c *BFTRelayConfig) { c.LocalPeerID = " " }, ErrBlankPeerID},
		{"blank cluster id", func(c *BFTRelayConfig) { c.ClusterID = "" }, ErrBlankClusterID},
		{"invalid private key", func(c *BFTRelayConfig) { c.PrivateKey = []byte("short") }, ErrInvalidRelayConfig},
		{"zero max hops", func(c *BFTRelayConfig) { c.MaxHops = 0 }, ErrInvalidRelayConfig},
		{"excessive max hops", func(c *BFTRelayConfig) { c.MaxHops = maxBFTRelayHopCeiling + 1 }, ErrInvalidRelayConfig},
		{"non-positive ttl", func(c *BFTRelayConfig) { c.DefaultTTL = 0 }, ErrInvalidRelayConfig},
		{"non-positive cache", func(c *BFTRelayConfig) { c.MaxSeenCache = 0 }, ErrInvalidRelayConfig},
	}
	for _, tc := range cfgCases {
		cfg := baseCfg
		tc.mutate(&cfg)
		_, cfgErr := NewBFTRelayNode(cfg)
		assert.ErrorIs(t, cfgErr, tc.wantErr, tc.name)
	}

	origin := newTestBFTPeer(t, testBFTOriginID, testBFTRelayClusterID, defaultBFTRelayMaxHops)
	validEnv, err := origin.node.CreateEnvelope(testBFTMsgID, 1, []byte(testBFTPayload))
	require.NoError(t, err)
	require.NoError(t, validEnv.ValidateStatic())

	envCases := []struct {
		name    string
		mutate  func(*BFTRelayEnvelope)
		wantErr error
	}{
		{"blank message id", func(e *BFTRelayEnvelope) { e.MessageID = "" }, ErrMalformedRelayEnvelope},
		{"blank cluster id", func(e *BFTRelayEnvelope) { e.ClusterID = " " }, ErrMalformedRelayEnvelope},
		{"blank origin id", func(e *BFTRelayEnvelope) { e.OriginPeerID = "" }, ErrMalformedRelayEnvelope},
		{"zero sequence", func(e *BFTRelayEnvelope) { e.Sequence = 0 }, ErrMalformedRelayEnvelope},
		{"excessive max hops", func(e *BFTRelayEnvelope) { e.MaxHops = maxBFTRelayHopCeiling + 1 }, ErrMalformedRelayEnvelope},
		{"inverted timestamps", func(e *BFTRelayEnvelope) { e.ExpiresAt = e.CreatedAt.Add(-time.Second) }, ErrMalformedRelayEnvelope},
		{"short signature", func(e *BFTRelayEnvelope) { e.OriginSignature = []byte("bad") }, ErrMalformedRelayEnvelope},
		{"attestation count mismatch", func(e *BFTRelayEnvelope) { e.HopCount = 2 }, ErrMalformedRelayEnvelope},
		{"hash mismatch", func(e *BFTRelayEnvelope) { e.PayloadHash = "0000" }, ErrContentHashMismatch},
	}
	for _, tc := range envCases {
		env := validEnv.Clone()
		tc.mutate(&env)
		assert.ErrorIs(t, env.ValidateStatic(), tc.wantErr, tc.name)
	}

	assert.ErrorIs(t, origin.node.RegisterPeer(testBFTRelayAID, testBFTForeignClus, origin.pub), ErrClusterMismatch)
	assert.ErrorIs(t, origin.node.ConnectPeer(testBFTRelayAID, func(context.Context, string, BFTRelayEnvelope) error { return nil }), ErrUntrustedPeer)
}

func TestBFTRelay_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	origin := newTestBFTPeer(t, testBFTOriginID, testBFTRelayClusterID, 4)
	relayA := newTestBFTPeer(t, testBFTRelayAID, testBFTRelayClusterID, 4)
	relayB := newTestBFTPeer(t, testBFTRelayBID, testBFTRelayClusterID, 4)
	leafC := newTestBFTPeer(t, testBFTLeafCID, testBFTRelayClusterID, 4)
	registerPeerMesh(t, origin, relayA, relayB, leafC)

	ctx := context.Background()
	var (
		leafMu       sync.Mutex
		leafReceived []BFTRelayEnvelope
	)
	require.NoError(t, leafC.node.ConnectPeer(relayB.id, func(context.Context, string, BFTRelayEnvelope) error { return nil }))
	require.NoError(t, relayB.node.ConnectPeer(leafC.id, func(c context.Context, _ string, env BFTRelayEnvelope) error {
		out, fwdErr := leafC.node.ReceiveAndForward(c, env)
		if fwdErr != nil {
			return fwdErr
		}
		if out.Accepted {
			leafMu.Lock()
			leafReceived = append(leafReceived, env)
			leafMu.Unlock()
		}
		return nil
	}))
	require.NoError(t, relayA.node.ConnectPeer(relayB.id, func(c context.Context, _ string, env BFTRelayEnvelope) error {
		_, fwdErr := relayB.node.ReceiveAndForward(c, env)
		return fwdErr
	}))

	env, err := origin.node.CreateEnvelope(testBFTMsgID, 1, []byte(testBFTPayload))
	require.NoError(t, err)

	outcome, err := relayA.node.ReceiveAndForward(ctx, env)
	require.NoError(t, err)
	assert.True(t, outcome.Accepted)
	assert.False(t, outcome.Deduplicated)
	assert.Equal(t, uint32(1), outcome.HopCount)
	assert.Equal(t, []string{relayB.id}, outcome.ForwardedTo)

	leafMu.Lock()
	require.Len(t, leafReceived, 1)
	assert.Equal(t, uint32(2), leafReceived[0].HopCount)
	assert.Len(t, leafReceived[0].Attestations, 2)
	leafMu.Unlock()

	// Duplicate broadcast of the same message ID must be deduplicated without forwarding.
	dupOutcome, err := relayA.node.ReceiveAndForward(ctx, env)
	require.NoError(t, err)
	assert.True(t, dupOutcome.Deduplicated)
	assert.False(t, dupOutcome.Accepted)
	assert.Empty(t, dupOutcome.ForwardedTo)

	// Concurrent stress: 8 workers sending unique and overlapping envelopes into relayA.
	wg := goroutinelabels.StartTestGoroutines(t, 8, "bft_relay_stress", "concurrent relay and dedup", func(id int) {
		for seq := 10; seq < 16; seq++ {
			msgID := fmt.Sprintf("stress-%d-%d", id%2, seq)
			pkt, mkErr := origin.node.CreateEnvelope(msgID, uint64(id*100+seq), []byte(testBFTPayload))
			assert.NoError(t, mkErr)
			_, rcvErr := relayA.node.ReceiveAndForward(ctx, pkt)
			if rcvErr != nil {
				assert.ErrorIs(t, rcvErr, ErrReplayedSequence)
			}
		}
	})
	wg.Wait()

	stats := relayA.node.Stats()
	assert.GreaterOrEqual(t, stats.AcceptedCount, uint64(2))
	assert.GreaterOrEqual(t, stats.DeduplicatedCount, uint64(1))
}

func TestBFTRelay_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	origin := newTestBFTPeer(t, testBFTOriginID, testBFTRelayClusterID, 2)
	relayA := newTestBFTPeer(t, testBFTRelayAID, testBFTRelayClusterID, 2)
	rogue := newTestBFTPeer(t, "peer-rogue-99", testBFTRelayClusterID, 2)
	registerPeerMesh(t, origin, relayA)

	now := time.Now().UTC()
	relayA.node.SetClock(func() time.Time { return now })
	origin.node.SetClock(func() time.Time { return now })

	ctx := context.Background()
	seq1, err := origin.node.CreateEnvelope("msg-adv-1", 10, []byte(testBFTPayload))
	require.NoError(t, err)
	_, err = relayA.node.ReceiveAndForward(ctx, seq1)
	require.NoError(t, err)

	advCases := []struct {
		name    string
		build   func() BFTRelayEnvelope
		wantErr error
	}{
		{
			name: "forged origin signature",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-forged", 11, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				e.OriginSignature[0] ^= 0xff
				return e
			},
			wantErr: ErrInvalidRelaySignature,
		},
		{
			name: "forged intermediate attestation",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-hop-forged", 12, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				e.HopCount = 1
				e.Attestations = []RelayAttestation{{
					PeerID:    origin.id,
					HopIndex:  1,
					Timestamp: now,
					Signature: make([]byte, ed25519.SignatureSize),
				}}
				return e
			},
			wantErr: ErrInvalidRelaySignature,
		},
		{
			name: "altered payload hash mismatch",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-tamper", 13, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				e.Payload = []byte("tampered-payload-bytes")
				return e
			},
			wantErr: ErrContentHashMismatch,
		},
		{
			name: "expired ttl",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-expired", 14, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				e.CreatedAt = now.Add(-time.Minute)
				e.ExpiresAt = now.Add(-time.Millisecond)
				e.OriginSignature = ed25519.Sign(origin.priv, e.OriginSigningBytes())
				return e
			},
			wantErr: ErrRelayExpired,
		},
		{
			name: "hop limit exceeded",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-hops", 15, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				e.HopCount = 2
				e.Attestations = []RelayAttestation{
					{PeerID: origin.id, HopIndex: 1, Timestamp: now, Signature: make([]byte, ed25519.SignatureSize)},
					{PeerID: origin.id, HopIndex: 2, Timestamp: now, Signature: make([]byte, ed25519.SignatureSize)},
				}
				return e
			},
			wantErr: ErrHopLimitExceeded,
		},
		{
			name: "untrusted origin peer",
			build: func() BFTRelayEnvelope {
				e, mkErr := rogue.node.CreateEnvelope("msg-adv-rogue", 1, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				return e
			},
			wantErr: ErrUntrustedPeer,
		},
		{
			name: "foreign cluster mismatch",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-foreign", 16, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				e.ClusterID = testBFTForeignClus
				return e
			},
			wantErr: ErrClusterMismatch,
		},
		{
			name: "replayed stale sequence number",
			build: func() BFTRelayEnvelope {
				e, mkErr := origin.node.CreateEnvelope("msg-adv-replay", 9, []byte(testBFTPayload))
				require.NoError(t, mkErr)
				return e
			},
			wantErr: ErrReplayedSequence,
		},
	}

	for _, tc := range advCases {
		_, advErr := relayA.node.ReceiveAndForward(ctx, tc.build())
		assert.ErrorIs(t, advErr, tc.wantErr, tc.name)
	}

	require.NoError(t, relayA.node.RevokePeer(origin.id))
	revokedEnv, err := origin.node.CreateEnvelope("msg-adv-revoked", 99, []byte(testBFTPayload))
	require.NoError(t, err)
	_, err = relayA.node.ReceiveAndForward(ctx, revokedEnv)
	assert.ErrorIs(t, err, ErrUntrustedPeer)
}
