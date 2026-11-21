package community

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	// CipherTLS_AES_256_GCM_SHA384 is the preferred TLS 1.3 AEAD cipher suite.
	CipherTLS_AES_256_GCM_SHA384 = "TLS_AES_256_GCM_SHA384"

	// CipherTLS_CHACHA20_POLY1305_SHA256 is the ChaCha20-Poly1305 TLS 1.3 cipher suite.
	CipherTLS_CHACHA20_POLY1305_SHA256 = "TLS_CHACHA20_POLY1305_SHA256"

	// CipherTLS_AES_128_GCM_SHA256 is the AES-128-GCM TLS 1.3 cipher suite.
	CipherTLS_AES_128_GCM_SHA256 = "TLS_AES_128_GCM_SHA256"

	defaultBridgeSessionTTL   = 15 * time.Minute
	defaultBridgeMaxClockSkew = 30 * time.Second
)

var (
	// ErrBlankEndpoint indicates an empty or whitespace cluster endpoint address.
	ErrBlankEndpoint = errors.New("cluster endpoint address cannot be blank")

	// ErrBlankCertFingerprint indicates an empty or whitespace TLS certificate fingerprint.
	ErrBlankCertFingerprint = errors.New("TLS certificate fingerprint cannot be blank")

	// ErrUnsupportedCipherSuite indicates a cipher suite outside the allowed TLS 1.3 set.
	ErrUnsupportedCipherSuite = errors.New("unsupported TLS cipher suite")

	// ErrInvalidTLSManifest indicates an invalid or expired mutual TLS manifest.
	ErrInvalidTLSManifest = errors.New("invalid or expired TLS manifest")

	// ErrBlankSessionID indicates an empty or whitespace handshake session identifier.
	ErrBlankSessionID = errors.New("session ID cannot be blank")

	// ErrBlankNonce indicates an empty or whitespace handshake nonce.
	ErrBlankNonce = errors.New("handshake nonce cannot be blank")

	// ErrBlankSharedSecret indicates a missing HMAC shared secret.
	ErrBlankSharedSecret = errors.New("bridge shared secret cannot be empty")

	// ErrSpoofedCluster indicates an unregistered cluster or mismatched endpoint identity.
	ErrSpoofedCluster = errors.New("spoofed or unregistered cluster endpoint")

	// ErrSessionNotFound indicates an unknown handshake session identifier.
	ErrSessionNotFound = errors.New("handshake session not found")

	// ErrSessionExpired indicates a handshake session whose TTL has elapsed.
	ErrSessionExpired = errors.New("handshake session expired")

	// ErrInvalidSessionToken indicates a failed HMAC session token verification.
	ErrInvalidSessionToken = errors.New("invalid handshake session token")

	// ErrReplayedNonce indicates a duplicate handshake nonce submission.
	ErrReplayedNonce = errors.New("replayed handshake nonce")

	// ErrReplayedFrame indicates a duplicate or already-processed delta sequence.
	ErrReplayedFrame = errors.New("replayed state delta frame")

	// ErrClockSkewExceeded indicates a timestamp outside the allowed clock skew window.
	ErrClockSkewExceeded = errors.New("timestamp exceeds maximum allowed clock skew")

	// ErrNonMonotonicSequence indicates an out-of-order or zero delta sequence number.
	ErrNonMonotonicSequence = errors.New("non-monotonic delta sequence number")

	// ErrCASDigestMismatch indicates a payload whose SHA-256 digest does not match CASDigest.
	ErrCASDigestMismatch = errors.New("CAS digest mismatch")

	// ErrBlankDeltaPayload indicates an empty state delta payload.
	ErrBlankDeltaPayload = errors.New("state delta payload cannot be empty")
)

var allowedCipherSuites = map[string]struct{}{
	CipherTLS_AES_256_GCM_SHA384:       {},
	CipherTLS_CHACHA20_POLY1305_SHA256: {},
	CipherTLS_AES_128_GCM_SHA256:       {},
}

// TLSManifest describes the mutual TLS identity and cryptographic parameters of a cluster.
type TLSManifest struct {
	CertFingerprint   string
	RootCAFingerprint string
	CipherSuite       string
	MutualTLS         bool
	ExpiresAt         time.Time
}

// ClusterEndpoint describes a remote or local cluster network endpoint and its TLS manifest.
type ClusterEndpoint struct {
	ClusterID string
	Address   string
	Manifest  TLSManifest
}

// HandshakeRequest initiates a mutually authenticated cross-cluster epistemic bridge session.
type HandshakeRequest struct {
	RemoteCluster ClusterEndpoint
	Nonce         string
	Timestamp     time.Time
}

// HandshakeSession represents an established, token-bound cross-cluster bridge session.
type HandshakeSession struct {
	SessionID         string
	LocalClusterID    string
	RemoteClusterID   string
	RemoteFingerprint string
	CipherSuite       string
	SessionToken      string
	EstablishedAt     time.Time
	ExpiresAt         time.Time
	LastSendSeq       uint64
	LastRecvSeq       uint64
}

// StateDeltaFrame carries a sequence-tracked, CAS-verified epistemic state delta.
type StateDeltaFrame struct {
	SessionID     string
	SourceCluster string
	TargetCluster string
	Sequence      uint64
	CASDigest     string
	Payload       []byte
	SessionToken  string
	Timestamp     time.Time
}

// BridgeConfig configures an EpistemicStateBridge instance.
type BridgeConfig struct {
	LocalClusterID string
	LocalEndpoint  ClusterEndpoint
	SharedSecret   []byte
	SessionTTL     time.Duration
	MaxClockSkew   time.Duration
	NowFunc        func() time.Time
}

// EpistemicStateBridge manages mutual TLS validation, session handshakes,
// and replay-protected bidirectional state delta exchange across clusters.
type EpistemicStateBridge struct {
	mu                 sync.RWMutex
	localClusterID     string
	localEndpoint      ClusterEndpoint
	sharedSecret       []byte
	sessionTTL         time.Duration
	maxClockSkew       time.Duration
	nowFunc            func() time.Time
	registeredClusters map[string]ClusterEndpoint
	sessions           map[string]*HandshakeSession
	seenNonces         map[string]time.Time
	pendingDeltas      map[string][]StateDeltaFrame
}

type blankFieldCheck struct {
	value string
	label string
	err   error
}

func validateRequiredFields(checks []blankFieldCheck) error {
	for _, c := range checks {
		if err := strutil.ErrorOnBlank(c.value, "%s cannot be blank", c.label); err != nil {
			return c.err
		}
	}
	return nil
}

// ValidateTLSManifest verifies that a TLSManifest satisfies mutual TLS,
// allowed cipher suite, non-blank fingerprint, and expiration invariants.
func ValidateTLSManifest(m TLSManifest, now time.Time) error {
	if err := validateRequiredFields([]blankFieldCheck{
		{value: m.CertFingerprint, label: "certificate fingerprint", err: ErrBlankCertFingerprint},
		{value: m.RootCAFingerprint, label: "root CA fingerprint", err: ErrBlankCertFingerprint},
		{value: m.CipherSuite, label: "cipher suite", err: ErrUnsupportedCipherSuite},
	}); err != nil {
		return err
	}
	if !m.MutualTLS {
		return fmt.Errorf("%w: mutual TLS must be enabled", ErrInvalidTLSManifest)
	}
	if _, ok := allowedCipherSuites[m.CipherSuite]; !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedCipherSuite, m.CipherSuite)
	}
	if m.ExpiresAt.IsZero() || now.After(m.ExpiresAt) {
		return fmt.Errorf("%w: manifest expired or missing expiry", ErrInvalidTLSManifest)
	}
	return nil
}

// ValidateClusterEndpoint validates a ClusterEndpoint descriptor and its TLSManifest.
func ValidateClusterEndpoint(ep ClusterEndpoint, now time.Time) error {
	if err := validateRequiredFields([]blankFieldCheck{
		{value: ep.ClusterID, label: "cluster ID", err: ErrBlankClusterID},
		{value: ep.Address, label: "endpoint address", err: ErrBlankEndpoint},
	}); err != nil {
		return err
	}
	return ValidateTLSManifest(ep.Manifest, now)
}

// ComputeCASDigest returns the canonical SHA-256 content-addressed digest for a payload.
func ComputeCASDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// NewEpistemicStateBridge initializes a validated EpistemicStateBridge.
func NewEpistemicStateBridge(cfg BridgeConfig) (*EpistemicStateBridge, error) {
	nowFn := cfg.NowFunc
	if nowFn == nil {
		nowFn = time.Now
	}
	if err := strutil.ErrorOnBlank(cfg.LocalClusterID, "local cluster ID cannot be blank"); err != nil {
		return nil, ErrBlankClusterID
	}
	if len(cfg.SharedSecret) == 0 {
		return nil, ErrBlankSharedSecret
	}
	if err := ValidateClusterEndpoint(cfg.LocalEndpoint, nowFn()); err != nil {
		return nil, err
	}
	if cfg.LocalEndpoint.ClusterID != cfg.LocalClusterID {
		return nil, fmt.Errorf("%w: local endpoint cluster ID mismatch", ErrSpoofedCluster)
	}

	ttl := cfg.SessionTTL
	if ttl <= 0 {
		ttl = defaultBridgeSessionTTL
	}
	skew := cfg.MaxClockSkew
	if skew <= 0 {
		skew = defaultBridgeMaxClockSkew
	}

	return &EpistemicStateBridge{
		localClusterID:     cfg.LocalClusterID,
		localEndpoint:      cfg.LocalEndpoint,
		sharedSecret:       bytes.Clone(cfg.SharedSecret),
		sessionTTL:         ttl,
		maxClockSkew:       skew,
		nowFunc:            nowFn,
		registeredClusters: make(map[string]ClusterEndpoint),
		sessions:           make(map[string]*HandshakeSession),
		seenNonces:         make(map[string]time.Time),
		pendingDeltas:      make(map[string][]StateDeltaFrame),
	}, nil
}

// RegisterCluster registers a trusted peer cluster endpoint for mutual handshakes.
func (b *EpistemicStateBridge) RegisterCluster(ep ClusterEndpoint) error {
	now := b.nowFunc()
	if err := ValidateClusterEndpoint(ep, now); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.registeredClusters[ep.ClusterID] = ep
	return nil
}

// NegotiateHandshake validates a peer handshake request against registered cluster
// descriptors, clock skew bounds, and replay fences, returning a token-bound session.
func (b *EpistemicStateBridge) NegotiateHandshake(req HandshakeRequest) (*HandshakeSession, error) {
	now := b.nowFunc()
	if err := ValidateClusterEndpoint(req.RemoteCluster, now); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(req.Nonce, "handshake nonce cannot be blank"); err != nil {
		return nil, ErrBlankNonce
	}
	if err := b.checkClockSkew(now, req.Timestamp); err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	reg, ok := b.registeredClusters[req.RemoteCluster.ClusterID]
	if !ok || reg.Address != req.RemoteCluster.Address ||
		reg.Manifest.CertFingerprint != req.RemoteCluster.Manifest.CertFingerprint ||
		reg.Manifest.RootCAFingerprint != req.RemoteCluster.Manifest.RootCAFingerprint {
		return nil, ErrSpoofedCluster
	}

	if _, replayed := b.seenNonces[req.Nonce]; replayed {
		return nil, ErrReplayedNonce
	}
	b.seenNonces[req.Nonce] = now

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	expiresAt := now.Add(b.sessionTTL)
	token := b.computeSessionToken(sessionID, req.RemoteCluster.ClusterID, req.RemoteCluster.Manifest.CertFingerprint, req.Nonce, expiresAt)

	sess := &HandshakeSession{
		SessionID:         sessionID,
		LocalClusterID:    b.localClusterID,
		RemoteClusterID:   req.RemoteCluster.ClusterID,
		RemoteFingerprint: req.RemoteCluster.Manifest.CertFingerprint,
		CipherSuite:       req.RemoteCluster.Manifest.CipherSuite,
		SessionToken:      token,
		EstablishedAt:     now,
		ExpiresAt:         expiresAt,
	}
	b.sessions[sessionID] = sess

	copied := *sess
	return &copied, nil
}

// AcceptPeerSession registers a mutually negotiated session initiated by a peer bridge
// using the same shared secret so both sides can exchange verified delta frames.
func (b *EpistemicStateBridge) AcceptPeerSession(sess HandshakeSession) error {
	if err := validateRequiredFields([]blankFieldCheck{
		{value: sess.SessionID, label: "session ID", err: ErrBlankSessionID},
		{value: sess.RemoteClusterID, label: "remote cluster ID", err: ErrBlankClusterID},
		{value: sess.RemoteFingerprint, label: "remote fingerprint", err: ErrBlankCertFingerprint},
		{value: sess.SessionToken, label: "session token", err: ErrInvalidSessionToken},
	}); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.nowFunc()
	if now.After(sess.ExpiresAt) {
		return ErrSessionExpired
	}
	reg, ok := b.registeredClusters[sess.RemoteClusterID]
	if !ok || reg.Manifest.CertFingerprint != sess.RemoteFingerprint {
		return ErrSpoofedCluster
	}

	copied := sess
	copied.LocalClusterID = b.localClusterID
	b.sessions[sess.SessionID] = &copied
	return nil
}

// SendDelta constructs a monotonically sequenced, CAS-verified StateDeltaFrame for an active session.
func (b *EpistemicStateBridge) SendDelta(sessionID string, payload []byte, casDigest string) (StateDeltaFrame, error) {
	if len(payload) == 0 {
		return StateDeltaFrame{}, ErrBlankDeltaPayload
	}
	if err := strutil.ErrorOnBlank(casDigest, "CAS digest cannot be blank"); err != nil {
		return StateDeltaFrame{}, ErrCASDigestMismatch
	}
	if ComputeCASDigest(payload) != casDigest {
		return StateDeltaFrame{}, ErrCASDigestMismatch
	}

	var frame StateDeltaFrame
	err := b.withLockedSession(sessionID, func(sess *HandshakeSession, now time.Time) error {
		sess.LastSendSeq++
		frame = StateDeltaFrame{
			SessionID:     sess.SessionID,
			SourceCluster: b.localClusterID,
			TargetCluster: sess.RemoteClusterID,
			Sequence:      sess.LastSendSeq,
			CASDigest:     casDigest,
			Payload:       bytes.Clone(payload),
			SessionToken:  sess.SessionToken,
			Timestamp:     now,
		}
		return nil
	})
	if err != nil {
		return StateDeltaFrame{}, err
	}
	return frame, nil
}

// ReceiveDelta validates an incoming StateDeltaFrame against session token binding,
// clock skew, replay/monotonicity invariants, and CAS digest integrity.
func (b *EpistemicStateBridge) ReceiveDelta(frame StateDeltaFrame) error {
	if len(frame.Payload) == 0 {
		return ErrBlankDeltaPayload
	}
	if err := validateRequiredFields([]blankFieldCheck{
		{value: frame.SessionID, label: "session ID", err: ErrBlankSessionID},
		{value: frame.SourceCluster, label: "source cluster", err: ErrBlankClusterID},
		{value: frame.TargetCluster, label: "target cluster", err: ErrBlankClusterID},
		{value: frame.CASDigest, label: "CAS digest", err: ErrCASDigestMismatch},
		{value: frame.SessionToken, label: "session token", err: ErrInvalidSessionToken},
	}); err != nil {
		return err
	}

	return b.withLockedSession(frame.SessionID, func(sess *HandshakeSession, now time.Time) error {
		if err := b.checkClockSkew(now, frame.Timestamp); err != nil {
			return err
		}
		if frame.TargetCluster != b.localClusterID || frame.SourceCluster != sess.RemoteClusterID {
			return ErrSpoofedCluster
		}
		if !hmac.Equal([]byte(frame.SessionToken), []byte(sess.SessionToken)) {
			return ErrInvalidSessionToken
		}
		if frame.Sequence == 0 {
			return ErrNonMonotonicSequence
		}
		if frame.Sequence <= sess.LastRecvSeq {
			return ErrReplayedFrame
		}
		if frame.Sequence != sess.LastRecvSeq+1 {
			return ErrNonMonotonicSequence
		}
		if ComputeCASDigest(frame.Payload) != frame.CASDigest {
			return ErrCASDigestMismatch
		}

		sess.LastRecvSeq = frame.Sequence
		stored := frame
		stored.Payload = bytes.Clone(frame.Payload)
		b.pendingDeltas[frame.SessionID] = append(b.pendingDeltas[frame.SessionID], stored)
		return nil
	})
}

// PendingDeltas returns a copy of all verified inbound delta frames for the given session.
func (b *EpistemicStateBridge) PendingDeltas(sessionID string) ([]StateDeltaFrame, error) {
	var out []StateDeltaFrame
	err := b.withLockedSession(sessionID, func(_ *HandshakeSession, _ time.Time) error {
		frames := b.pendingDeltas[sessionID]
		out = make([]StateDeltaFrame, len(frames))
		for i, f := range frames {
			out[i] = f
			out[i].Payload = bytes.Clone(f.Payload)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (b *EpistemicStateBridge) withLockedSession(sessionID string, fn func(sess *HandshakeSession, now time.Time) error) error {
	if err := strutil.ErrorOnBlank(sessionID, "session ID cannot be blank"); err != nil {
		return ErrBlankSessionID
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	sess, ok := b.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	now := b.nowFunc()
	if now.After(sess.ExpiresAt) {
		return ErrSessionExpired
	}
	return fn(sess, now)
}

func (b *EpistemicStateBridge) checkClockSkew(now, ts time.Time) error {
	if ts.IsZero() {
		return ErrClockSkewExceeded
	}
	diff := now.Sub(ts)
	if diff < 0 {
		diff = -diff
	}
	if diff > b.maxClockSkew {
		return ErrClockSkewExceeded
	}
	return nil
}

func (b *EpistemicStateBridge) computeSessionToken(sessionID, remoteClusterID, fingerprint, nonce string, expiresAt time.Time) string {
	mac := hmac.New(sha256.New, b.sharedSecret)
	fmt.Fprintf(mac, "%s|%s|%s|%s|%s|%d", sessionID, b.localClusterID, remoteClusterID, fingerprint, nonce, expiresAt.UnixNano())
	return hex.EncodeToString(mac.Sum(nil))
}

func generateSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return "sess-" + hex.EncodeToString(buf), nil
}
