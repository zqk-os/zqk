package community

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	defaultBFTRelayMaxHops          = uint32(8)
	maxBFTRelayHopCeiling           = uint32(64)
	defaultBFTRelayTTL              = 30 * time.Second
	defaultBFTRelaySeenCap          = 4096
	bftRelayDomainOrigin            = "zqk-bft-relay-origin-v1"
	bftRelayDomainHop               = "zqk-bft-relay-hop-v1"
	bftRelayForwardGoroutineName    = "bft_relay_forward"
	bftRelayForwardGoroutinePurpose = "concurrent peer packet forwarding"
	errBFTRelayBlankMsgID           = "relay message ID cannot be blank"
)

var (
	// ErrMalformedRelayEnvelope indicates a BFT relay envelope failed structural or field validation.
	ErrMalformedRelayEnvelope = errors.New("malformed BFT relay envelope")

	// ErrInvalidRelayConfig indicates a BFT relay node configuration parameter is invalid.
	ErrInvalidRelayConfig = errors.New("invalid BFT relay configuration")

	// ErrInvalidRelaySignature indicates an origin or intermediate peer Ed25519 signature failed verification.
	ErrInvalidRelaySignature = errors.New("invalid BFT relay signature")

	// ErrContentHashMismatch indicates the envelope payload does not match its declared SHA-256 digest.
	ErrContentHashMismatch = errors.New("relay payload SHA-256 hash mismatch")

	// ErrHopLimitExceeded indicates the relay packet exceeded its configured or envelope hop ceiling.
	ErrHopLimitExceeded = errors.New("relay hop limit exceeded")

	// ErrRelayExpired indicates the relay envelope TTL has elapsed.
	ErrRelayExpired = errors.New("relay envelope TTL expired")

	// ErrUntrustedPeer indicates the origin or intermediate relay peer is not registered as trusted.
	ErrUntrustedPeer = errors.New("untrusted or unknown relay peer")

	// ErrReplayedSequence indicates a stale or replayed monotonic sequence number from an origin peer.
	ErrReplayedSequence = errors.New("replayed or stale sequence number")
)

// BFTRelayConfig parameterizes a Byzantine fault tolerant relay node.
type BFTRelayConfig struct {
	LocalPeerID  string
	ClusterID    string
	PrivateKey   ed25519.PrivateKey
	MaxHops      uint32
	DefaultTTL   time.Duration
	MaxSeenCache int
}

// Validate enforces static floor invariants on the BFT relay node configuration.
func (c BFTRelayConfig) Validate() error {
	if strutil.IsBlank(c.LocalPeerID) {
		return ErrBlankPeerID
	}
	if strutil.IsBlank(c.ClusterID) {
		return ErrBlankClusterID
	}
	if len(c.PrivateKey) != ed25519.PrivateKeySize {
		return ErrInvalidRelayConfig
	}
	if c.MaxHops == 0 || c.MaxHops > maxBFTRelayHopCeiling {
		return ErrInvalidRelayConfig
	}
	if c.DefaultTTL <= 0 || c.MaxSeenCache <= 0 {
		return ErrInvalidRelayConfig
	}
	return nil
}

// RelayAttestation records a cryptographic hop signature appended by an intermediate relay peer.
type RelayAttestation struct {
	PeerID    string
	HopIndex  uint32
	Timestamp time.Time
	Signature []byte
}

// BFTRelayEnvelope carries a signed broadcast payload and its hop attestation chain.
type BFTRelayEnvelope struct {
	MessageID       string
	ClusterID       string
	OriginPeerID    string
	Sequence        uint64
	HopCount        uint32
	MaxHops         uint32
	CreatedAt       time.Time
	ExpiresAt       time.Time
	Payload         []byte
	PayloadHash     string
	OriginSignature []byte
	Attestations    []RelayAttestation
}

// ComputePayloadHash returns the hex-encoded SHA-256 digest of the envelope payload.
func (e BFTRelayEnvelope) ComputePayloadHash() string {
	sum := sha256.Sum256(e.Payload)
	return hex.EncodeToString(sum[:])
}

// OriginSigningBytes returns the canonical byte slice signed by the origin peer.
func (e BFTRelayEnvelope) OriginSigningBytes() []byte {
	var buf []byte
	buf = appendLengthPrefixed(buf, []byte(bftRelayDomainOrigin))
	buf = appendLengthPrefixed(buf, []byte(e.MessageID))
	buf = appendLengthPrefixed(buf, []byte(e.ClusterID))
	buf = appendLengthPrefixed(buf, []byte(e.OriginPeerID))
	buf = binary.BigEndian.AppendUint64(buf, e.Sequence)
	buf = binary.BigEndian.AppendUint32(buf, e.MaxHops)
	buf = binary.BigEndian.AppendUint64(buf, uint64(e.CreatedAt.UTC().UnixNano()))
	buf = binary.BigEndian.AppendUint64(buf, uint64(e.ExpiresAt.UTC().UnixNano()))
	buf = appendLengthPrefixed(buf, []byte(e.PayloadHash))
	return buf
}

// HopSigningBytes returns the canonical byte slice signed by an intermediate relay peer.
func (e BFTRelayEnvelope) HopSigningBytes(hopIndex uint32, peerID string, ts time.Time) []byte {
	var buf []byte
	buf = appendLengthPrefixed(buf, []byte(bftRelayDomainHop))
	buf = appendLengthPrefixed(buf, []byte(e.MessageID))
	buf = appendLengthPrefixed(buf, []byte(e.ClusterID))
	buf = appendLengthPrefixed(buf, []byte(e.PayloadHash))
	buf = binary.BigEndian.AppendUint32(buf, hopIndex)
	buf = appendLengthPrefixed(buf, []byte(peerID))
	buf = binary.BigEndian.AppendUint64(buf, uint64(ts.UTC().UnixNano()))
	return buf
}

// ValidateStatic enforces structural, hash, and bound invariants on the relay envelope.
func (e BFTRelayEnvelope) ValidateStatic() error {
	if strutil.IsBlank(e.MessageID) || strutil.IsBlank(e.ClusterID) || strutil.IsBlank(e.OriginPeerID) {
		return ErrMalformedRelayEnvelope
	}
	if e.Sequence == 0 || e.MaxHops == 0 || e.MaxHops > maxBFTRelayHopCeiling {
		return ErrMalformedRelayEnvelope
	}
	if e.CreatedAt.IsZero() || e.ExpiresAt.IsZero() || e.ExpiresAt.Before(e.CreatedAt) {
		return ErrMalformedRelayEnvelope
	}
	if len(e.OriginSignature) != ed25519.SignatureSize {
		return ErrMalformedRelayEnvelope
	}
	if len(e.Attestations) != int(e.HopCount) {
		return ErrMalformedRelayEnvelope
	}
	if len(e.PayloadHash) != sha256.Size*2 || e.PayloadHash != e.ComputePayloadHash() {
		return ErrContentHashMismatch
	}
	return nil
}

// Clone returns a deep copy of the envelope safe for concurrent forwarding.
func (e BFTRelayEnvelope) Clone() BFTRelayEnvelope {
	out := e
	out.Payload = append([]byte(nil), e.Payload...)
	out.OriginSignature = append([]byte(nil), e.OriginSignature...)
	if len(e.Attestations) > 0 {
		out.Attestations = make([]RelayAttestation, len(e.Attestations))
		for i, att := range e.Attestations {
			att.Signature = append([]byte(nil), att.Signature...)
			out.Attestations[i] = att
		}
	}
	return out
}

// RelayForwardFunc delivers a validated relay envelope to a connected downstream peer.
type RelayForwardFunc func(ctx context.Context, peerID string, env BFTRelayEnvelope) error

// RelayOutcome summarizes the disposition of a processed relay envelope.
type RelayOutcome struct {
	MessageID     string
	Accepted      bool
	Deduplicated  bool
	ForwardedTo   []string
	ForwardErrors map[string]error
	HopCount      uint32
}

// BFTRelayStats reports thread-safe operational telemetry for a relay node.
type BFTRelayStats struct {
	AcceptedCount      uint64
	DeduplicatedCount  uint64
	ForwardedCount     uint64
	DroppedCount       uint64
	TrustedPeerCount   int
	ConnectedPeerCount int
	SeenCacheSize      int
}

// BFTRelayNode validates, deduplicates, attests, and forwards BFT relay envelopes across peers.
type BFTRelayNode struct {
	mu             sync.RWMutex
	cfg            BFTRelayConfig
	trustedPeers   map[string]ed25519.PublicKey
	connectedPeers map[string]RelayForwardFunc
	seenMessages   map[string]time.Time
	seenOrder      []string
	lastSequence   map[string]uint64
	stats          BFTRelayStats
	nowFn          func() time.Time
}

// NewBFTRelayNode constructs a BFT relay node after validating its configuration.
func NewBFTRelayNode(cfg BFTRelayConfig) (*BFTRelayNode, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pub, ok := cfg.PrivateKey.Public().(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		return nil, ErrInvalidRelayConfig
	}
	trusted := map[string]ed25519.PublicKey{
		cfg.LocalPeerID: append(ed25519.PublicKey(nil), pub...),
	}
	return &BFTRelayNode{
		cfg:            cfg,
		trustedPeers:   trusted,
		connectedPeers: make(map[string]RelayForwardFunc),
		seenMessages:   make(map[string]time.Time),
		lastSequence:   make(map[string]uint64),
		nowFn:          time.Now,
	}, nil
}

// SetClock overrides the node's time source for deterministic testing.
func (n *BFTRelayNode) SetClock(fn func() time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if fn != nil {
		n.nowFn = fn
	}
}

// RegisterPeer adds a trusted peer public key belonging to the same cluster.
func (n *BFTRelayNode) RegisterPeer(peerID, clusterID string, pubKey ed25519.PublicKey) error {
	if strutil.IsBlank(peerID) {
		return ErrBlankPeerID
	}
	if strutil.IsBlank(clusterID) {
		return ErrBlankClusterID
	}
	if clusterID != n.cfg.ClusterID {
		return ErrClusterMismatch
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return ErrInvalidRelayConfig
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	n.trustedPeers[peerID] = append(ed25519.PublicKey(nil), pubKey...)
	return nil
}

// RevokePeer removes a peer from the trusted key registry and active forwarding links.
func (n *BFTRelayNode) RevokePeer(peerID string) error {
	if strutil.IsBlank(peerID) {
		return ErrBlankPeerID
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.trustedPeers, peerID)
	delete(n.connectedPeers, peerID)
	return nil
}

// ConnectPeer binds a forwarding callback to an already-trusted peer.
func (n *BFTRelayNode) ConnectPeer(peerID string, forwardFn RelayForwardFunc) error {
	if strutil.IsBlank(peerID) {
		return ErrBlankPeerID
	}
	if forwardFn == nil {
		return ErrInvalidRelayConfig
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.trustedPeers[peerID]; !ok {
		return ErrUntrustedPeer
	}
	n.connectedPeers[peerID] = forwardFn
	return nil
}

// CreateEnvelope mints and signs a new BFT relay envelope originated by this node.
func (n *BFTRelayNode) CreateEnvelope(messageID string, seq uint64, payload []byte) (BFTRelayEnvelope, error) {
	if strutil.IsBlank(messageID) {
		return BFTRelayEnvelope{}, fmt.Errorf("%w: %s", ErrMalformedRelayEnvelope, errBFTRelayBlankMsgID)
	}
	if seq == 0 {
		return BFTRelayEnvelope{}, ErrMalformedRelayEnvelope
	}
	n.mu.RLock()
	now := n.nowFn().UTC()
	cfg := n.cfg
	n.mu.RUnlock()

	env := BFTRelayEnvelope{
		MessageID:    messageID,
		ClusterID:    cfg.ClusterID,
		OriginPeerID: cfg.LocalPeerID,
		Sequence:     seq,
		HopCount:     0,
		MaxHops:      cfg.MaxHops,
		CreatedAt:    now,
		ExpiresAt:    now.Add(cfg.DefaultTTL),
		Payload:      append([]byte(nil), payload...),
	}
	env.PayloadHash = env.ComputePayloadHash()
	env.OriginSignature = ed25519.Sign(cfg.PrivateKey, env.OriginSigningBytes())
	return env, nil
}

// ReceiveAndForward verifies a relay envelope, deduplicates seen IDs, appends a hop attestation,
// and concurrently forwards the envelope to eligible connected peers.
func (n *BFTRelayNode) ReceiveAndForward(ctx context.Context, env BFTRelayEnvelope) (RelayOutcome, error) {
	if err := env.ValidateStatic(); err != nil {
		n.incrementDropped()
		return RelayOutcome{}, err
	}
	if env.ClusterID != n.cfg.ClusterID {
		n.incrementDropped()
		return RelayOutcome{}, ErrClusterMismatch
	}

	outcome, forwardedEnv, targets, err := n.validateAndStage(env)
	if err != nil || outcome.Deduplicated {
		return outcome, err
	}

	forwarded, fwdErrs := n.forwardConcurrently(ctx, forwardedEnv, targets)
	n.mu.Lock()
	n.stats.ForwardedCount += uint64(len(forwarded))
	n.mu.Unlock()

	outcome.ForwardedTo = forwarded
	outcome.ForwardErrors = fwdErrs
	return outcome, nil
}

// Stats returns a thread-safe snapshot of node telemetry and registry sizes.
func (n *BFTRelayNode) Stats() BFTRelayStats {
	n.mu.RLock()
	defer n.mu.RUnlock()
	snap := n.stats
	snap.TrustedPeerCount = len(n.trustedPeers)
	snap.ConnectedPeerCount = len(n.connectedPeers)
	snap.SeenCacheSize = len(n.seenMessages)
	return snap
}

func (n *BFTRelayNode) validateAndStage(env BFTRelayEnvelope) (RelayOutcome, BFTRelayEnvelope, map[string]RelayForwardFunc, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := n.nowFn().UTC()
	if now.After(env.ExpiresAt) {
		n.stats.DroppedCount++
		return RelayOutcome{}, BFTRelayEnvelope{}, nil, ErrRelayExpired
	}
	if env.HopCount >= env.MaxHops || env.HopCount >= n.cfg.MaxHops {
		n.stats.DroppedCount++
		return RelayOutcome{}, BFTRelayEnvelope{}, nil, ErrHopLimitExceeded
	}
	if err := n.verifySignaturesLocked(env); err != nil {
		n.stats.DroppedCount++
		return RelayOutcome{}, BFTRelayEnvelope{}, nil, err
	}
	if _, seen := n.seenMessages[env.MessageID]; seen {
		n.stats.DeduplicatedCount++
		return RelayOutcome{
			MessageID:    env.MessageID,
			Deduplicated: true,
			HopCount:     env.HopCount,
		}, BFTRelayEnvelope{}, nil, nil
	}
	if lastSeq, ok := n.lastSequence[env.OriginPeerID]; ok && env.Sequence <= lastSeq {
		n.stats.DroppedCount++
		return RelayOutcome{}, BFTRelayEnvelope{}, nil, ErrReplayedSequence
	}

	n.lastSequence[env.OriginPeerID] = env.Sequence
	n.recordSeenLocked(env.MessageID, now)
	n.stats.AcceptedCount++

	forwardedEnv := n.appendAttestationLocked(env, now)
	targets := n.selectTargetsLocked(env)
	return RelayOutcome{
		MessageID: env.MessageID,
		Accepted:  true,
		HopCount:  forwardedEnv.HopCount,
	}, forwardedEnv, targets, nil
}

func (n *BFTRelayNode) verifySignaturesLocked(env BFTRelayEnvelope) error {
	originPub, ok := n.trustedPeers[env.OriginPeerID]
	if !ok {
		return ErrUntrustedPeer
	}
	if !ed25519.Verify(originPub, env.OriginSigningBytes(), env.OriginSignature) {
		return ErrInvalidRelaySignature
	}
	for i, att := range env.Attestations {
		if att.HopIndex != uint32(i+1) || strutil.IsBlank(att.PeerID) || len(att.Signature) != ed25519.SignatureSize {
			return ErrMalformedRelayEnvelope
		}
		peerPub, trusted := n.trustedPeers[att.PeerID]
		if !trusted {
			return ErrUntrustedPeer
		}
		if !ed25519.Verify(peerPub, env.HopSigningBytes(att.HopIndex, att.PeerID, att.Timestamp), att.Signature) {
			return ErrInvalidRelaySignature
		}
	}
	return nil
}

func (n *BFTRelayNode) recordSeenLocked(messageID string, now time.Time) {
	if len(n.seenOrder) >= n.cfg.MaxSeenCache {
		oldest := n.seenOrder[0]
		n.seenOrder = n.seenOrder[1:]
		delete(n.seenMessages, oldest)
	}
	n.seenMessages[messageID] = now
	n.seenOrder = append(n.seenOrder, messageID)
}

func (n *BFTRelayNode) appendAttestationLocked(env BFTRelayEnvelope, now time.Time) BFTRelayEnvelope {
	next := env.Clone()
	next.HopCount++
	att := RelayAttestation{
		PeerID:    n.cfg.LocalPeerID,
		HopIndex:  next.HopCount,
		Timestamp: now,
	}
	att.Signature = ed25519.Sign(n.cfg.PrivateKey, next.HopSigningBytes(att.HopIndex, att.PeerID, att.Timestamp))
	next.Attestations = append(next.Attestations, att)
	return next
}

func (n *BFTRelayNode) selectTargetsLocked(env BFTRelayEnvelope) map[string]RelayForwardFunc {
	visited := make(map[string]struct{}, len(env.Attestations)+2)
	visited[n.cfg.LocalPeerID] = struct{}{}
	visited[env.OriginPeerID] = struct{}{}
	for _, att := range env.Attestations {
		visited[att.PeerID] = struct{}{}
	}
	targets := make(map[string]RelayForwardFunc, len(n.connectedPeers))
	for peerID, fn := range n.connectedPeers {
		if _, skip := visited[peerID]; !skip {
			targets[peerID] = fn
		}
	}
	return targets
}

func (n *BFTRelayNode) forwardConcurrently(ctx context.Context, env BFTRelayEnvelope, targets map[string]RelayForwardFunc) ([]string, map[string]error) {
	if len(targets) == 0 {
		return nil, nil
	}
	var (
		wg        sync.WaitGroup
		resMu     sync.Mutex
		forwarded = make([]string, 0, len(targets))
		fwdErrs   map[string]error
	)
	for peerID, fn := range targets {
		pid := peerID
		sendFn := fn
		pkt := env.Clone()
		goroutinelabels.NewGoroutine(bftRelayForwardGoroutineName, bftRelayForwardGoroutinePurpose).
			WithContext(ctx).
			WithWaitGroup(&wg).
			Start(func() error {
				err := sendFn(ctx, pid, pkt)
				resMu.Lock()
				defer resMu.Unlock()
				if err != nil {
					if fwdErrs == nil {
						fwdErrs = make(map[string]error)
					}
					fwdErrs[pid] = err
					return nil
				}
				forwarded = append(forwarded, pid)
				return nil
			})
	}
	wg.Wait()
	sort.Strings(forwarded)
	return forwarded, fwdErrs
}

func (n *BFTRelayNode) incrementDropped() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stats.DroppedCount++
}

func appendLengthPrefixed(dst, item []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(item)))
	return append(dst, item...)
}
