package community

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errDPBlankEnvelopeIDMsg = "envelope ID cannot be blank"
	laplaceMantissaBits     = 53
	laplaceMantissaShift    = 64 - laplaceMantissaBits
	laplaceMantissaScale    = float64(uint64(1) << laplaceMantissaBits)
)

var (
	// ErrInvalidPrivacyParameter indicates a differential privacy parameter is non-positive, out of bounds, or non-finite.
	ErrInvalidPrivacyParameter = errors.New("invalid differential privacy parameter")

	// ErrPrivacyBudgetExhausted indicates the requested query exceeds remaining privacy budget.
	ErrPrivacyBudgetExhausted = errors.New("privacy budget exhausted")

	// ErrMalformedGossipEnvelope indicates a gossip privacy envelope failed structural or commitment validation.
	ErrMalformedGossipEnvelope = errors.New("malformed gossip privacy envelope")
)

// DifferentialPrivacyGossipConfig parameterizes the privacy-preserving gossip synchronization engine.
type DifferentialPrivacyGossipConfig struct {
	LocalPeerID string
	ClusterID   string
	Epsilon     float64
	Delta       float64
	Sensitivity float64
	TotalBudget float64
}

// Validate enforces static floor invariants on differential privacy and cluster identity parameters.
func (c DifferentialPrivacyGossipConfig) Validate() error {
	if err := strutil.ErrorOnBlank(c.LocalPeerID, errGossipBlankLocalPeerID); err != nil {
		return ErrBlankPeerID
	}
	if err := strutil.ErrorOnBlank(c.ClusterID, errGossipBlankClusterIDMsg); err != nil {
		return ErrBlankClusterID
	}
	if !isPositiveFinite(c.Epsilon) || !isPositiveFinite(c.Sensitivity) || !isPositiveFinite(c.TotalBudget) {
		return ErrInvalidPrivacyParameter
	}
	if !isFinite(c.Delta) || c.Delta <= 0 || c.Delta >= 1 {
		return ErrInvalidPrivacyParameter
	}
	return nil
}

// GossipPrivacyEnvelope carries a differentially private additive state contribution from a peer.
type GossipPrivacyEnvelope struct {
	EnvelopeID         string
	SenderPeerID       string
	ClusterID          string
	Epoch              uint64
	PerturbedAggregate float64
	SampleCount        uint64
	EpsilonSpent       float64
	Commitment         string
}

// ComputeCommitment computes the deterministic SHA-256 commitment over the envelope's canonical fields.
func (e GossipPrivacyEnvelope) ComputeCommitment() (string, error) {
	h := sha256.New()
	if err := writeHashString(h, e.EnvelopeID); err != nil {
		return "", err
	}
	if err := writeHashString(h, e.SenderPeerID); err != nil {
		return "", err
	}
	if err := writeHashString(h, e.ClusterID); err != nil {
		return "", err
	}
	if err := writeHashUint64(h, e.Epoch); err != nil {
		return "", err
	}
	if err := writeHashUint64(h, math.Float64bits(e.PerturbedAggregate)); err != nil {
		return "", err
	}
	if err := writeHashUint64(h, e.SampleCount); err != nil {
		return "", err
	}
	if err := writeHashUint64(h, math.Float64bits(e.EpsilonSpent)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Validate verifies schema invariants and cryptographic commitment integrity for the gossip envelope.
func (e GossipPrivacyEnvelope) Validate() error {
	if err := strutil.ErrorOnBlank(e.EnvelopeID, errDPBlankEnvelopeIDMsg); err != nil {
		return ErrMalformedGossipEnvelope
	}
	if err := strutil.ErrorOnBlank(e.SenderPeerID, errGossipBlankPeerIDMsg); err != nil {
		return ErrMalformedGossipEnvelope
	}
	if err := strutil.ErrorOnBlank(e.ClusterID, errGossipBlankClusterIDMsg); err != nil {
		return ErrMalformedGossipEnvelope
	}
	if e.Epoch == 0 || e.SampleCount == 0 {
		return ErrMalformedGossipEnvelope
	}
	if !isFinite(e.PerturbedAggregate) || !isPositiveFinite(e.EpsilonSpent) {
		return ErrMalformedGossipEnvelope
	}
	expected, err := e.ComputeCommitment()
	if err != nil || e.Commitment != expected {
		return ErrMalformedGossipEnvelope
	}
	return nil
}

// ClusterPrivacyAggregate represents the homomorphically combined differentially private cluster state.
type ClusterPrivacyAggregate struct {
	ClusterID         string
	PeerCount         int
	TotalPerturbedSum float64
	TotalSampleCount  uint64
	TotalEpsilonSpent float64
	MaxEpoch          uint64
}

// DifferentialPrivacyGossipEngine manages calibrated Laplace perturbation, privacy budget accounting,
// and homomorphic additive gossip aggregation across cluster peers.
type DifferentialPrivacyGossipEngine struct {
	mu             sync.RWMutex
	cfg            DifferentialPrivacyGossipConfig
	consumedBudget float64
	localEpoch     uint64
	envelopes      map[string]GossipPrivacyEnvelope
}

// NewDifferentialPrivacyGossipEngine validates configuration and initializes a privacy-preserving gossip engine.
func NewDifferentialPrivacyGossipEngine(cfg DifferentialPrivacyGossipConfig) (*DifferentialPrivacyGossipEngine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DifferentialPrivacyGossipEngine{
		cfg:       cfg,
		envelopes: make(map[string]GossipPrivacyEnvelope),
	}, nil
}

// RemainingBudget returns the unspent epsilon privacy budget.
func (e *DifferentialPrivacyGossipEngine) RemainingBudget() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rem := e.cfg.TotalBudget - e.consumedBudget
	if rem < 0 {
		return 0
	}
	return rem
}

// ConsumedBudget returns the cumulative epsilon privacy budget spent so far.
func (e *DifferentialPrivacyGossipEngine) ConsumedBudget() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.consumedBudget
}

// PerturbAndPublish injects calibrated Laplace noise into rawAggregate, consumes epsilonQuery from the
// privacy budget, and records/returns a committed GossipPrivacyEnvelope.
func (e *DifferentialPrivacyGossipEngine) PerturbAndPublish(
	rawAggregate float64,
	sampleCount uint64,
	epsilonQuery float64,
) (GossipPrivacyEnvelope, error) {
	if !isFinite(rawAggregate) || !isPositiveFinite(epsilonQuery) || sampleCount == 0 {
		return GossipPrivacyEnvelope{}, ErrInvalidPrivacyParameter
	}

	noise, err := sampleLaplaceNoise(e.cfg.Sensitivity / epsilonQuery)
	if err != nil {
		return GossipPrivacyEnvelope{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.consumedBudget+epsilonQuery > e.cfg.TotalBudget {
		return GossipPrivacyEnvelope{}, ErrPrivacyBudgetExhausted
	}

	e.localEpoch++
	e.consumedBudget += epsilonQuery

	env := GossipPrivacyEnvelope{
		EnvelopeID:         fmt.Sprintf("%s-%s-%d", e.cfg.ClusterID, e.cfg.LocalPeerID, e.localEpoch),
		SenderPeerID:       e.cfg.LocalPeerID,
		ClusterID:          e.cfg.ClusterID,
		Epoch:              e.localEpoch,
		PerturbedAggregate: rawAggregate + noise,
		SampleCount:        sampleCount,
		EpsilonSpent:       epsilonQuery,
	}
	commitment, err := env.ComputeCommitment()
	if err != nil {
		return GossipPrivacyEnvelope{}, err
	}
	env.Commitment = commitment
	e.envelopes[e.cfg.LocalPeerID] = env

	return env, nil
}

// MergeEnvelope validates and merges an incoming peer envelope into the local homomorphic state.
func (e *DifferentialPrivacyGossipEngine) MergeEnvelope(env GossipPrivacyEnvelope) error {
	if err := env.Validate(); err != nil {
		return err
	}
	if env.ClusterID != e.cfg.ClusterID {
		return ErrClusterMismatch
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if existing, ok := e.envelopes[env.SenderPeerID]; ok && env.Epoch <= existing.Epoch {
		return ErrStaleEpoch
	}
	e.envelopes[env.SenderPeerID] = env
	return nil
}

// AggregateClusterState computes the homomorphic additive aggregate across all merged peer envelopes.
func (e *DifferentialPrivacyGossipEngine) AggregateClusterState() ClusterPrivacyAggregate {
	e.mu.RLock()
	defer e.mu.RUnlock()

	agg := ClusterPrivacyAggregate{
		ClusterID: e.cfg.ClusterID,
		PeerCount: len(e.envelopes),
	}
	for _, env := range e.envelopes {
		agg.TotalPerturbedSum += env.PerturbedAggregate
		agg.TotalSampleCount += env.SampleCount
		agg.TotalEpsilonSpent += env.EpsilonSpent
		if env.Epoch > agg.MaxEpoch {
			agg.MaxEpoch = env.Epoch
		}
	}
	return agg
}

func sampleLaplaceNoise(scale float64) (float64, error) {
	if !isPositiveFinite(scale) {
		return 0, ErrInvalidPrivacyParameter
	}
	var raw uint64
	if err := binary.Read(rand.Reader, binary.BigEndian, &raw); err != nil {
		return 0, err
	}
	// Map 53-bit integer into (0, 1) open interval to keep math.Log finite.
	u := (float64(raw>>laplaceMantissaShift) + 0.5) / laplaceMantissaScale
	centered := u - 0.5
	return -scale * math.Copysign(1.0, centered) * math.Log(1.0-2.0*math.Abs(centered)), nil
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func isPositiveFinite(v float64) bool {
	return isFinite(v) && v > 0
}

func writeHashString(h hash.Hash, s string) error {
	if err := writeHashUint64(h, uint64(len(s))); err != nil {
		return err
	}
	_, err := h.Write([]byte(s))
	return err
}

func writeHashUint64(h hash.Hash, v uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	_, err := h.Write(buf[:])
	return err
}
