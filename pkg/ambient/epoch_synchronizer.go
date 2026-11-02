package ambient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Default configuration parameters for epoch synchronization and barrier rendezvous.
const (
	DefaultEpochTimeout    = 5 * time.Second
	DefaultMinParticipants = 1
	DefaultMaxEpochHistory = 1024
)

// EpochState represents the distributed synchronization lifecycle phase of an epoch boundary.
type EpochState string

const (
	EpochStateUninitialized EpochState = "uninitialized"
	EpochStateActive        EpochState = "active"
	EpochStateSynchronizing EpochState = "synchronizing"
	EpochStateCommitted     EpochState = "committed"
	EpochStateAborted       EpochState = "aborted"

	// EpochStatusBarriered aliases EpochStateSynchronizing for barrier convergence.
	EpochStatusBarriered EpochState = "synchronizing"
	// EpochStatusActive aliases EpochStateActive.
	EpochStatusActive EpochState = "active"
	// EpochStatusCommitted aliases EpochStateCommitted.
	EpochStatusCommitted EpochState = "committed"
	// EpochStatusAborted aliases EpochStateAborted.
	EpochStatusAborted EpochState = "aborted"
	// EpochStatusPending aliases EpochStateUninitialized.
	EpochStatusPending EpochState = "uninitialized"
)

// IsValid validates whether the EpochState is an authorized lifecycle phase.
func (s EpochState) IsValid() bool {
	switch s {
	case EpochStateUninitialized,
		EpochStateActive,
		EpochStateSynchronizing,
		EpochStateCommitted,
		EpochStateAborted:
		return true
	default:
		return false
	}
}

// String returns the string representation of the EpochState.
func (s EpochState) String() string {
	return string(s)
}

// EpochStatus is a type alias for EpochState.
type EpochStatus = EpochState

// ChecksumAlgorithm denotes cryptographic or polynomial hashing algorithms for state snapshots.
type ChecksumAlgorithm string

const (
	ChecksumSHA256 ChecksumAlgorithm = "sha256"
	ChecksumCRC32  ChecksumAlgorithm = "crc32"

	// ChecksumAlgorithmSHA256 aliases ChecksumSHA256.
	ChecksumAlgorithmSHA256 ChecksumAlgorithm = "sha256"
	// ChecksumAlgorithmCRC32 aliases ChecksumCRC32.
	ChecksumAlgorithmCRC32 ChecksumAlgorithm = "crc32"
)

// IsValid validates whether the ChecksumAlgorithm is a supported hashing scheme.
func (a ChecksumAlgorithm) IsValid() bool {
	switch a {
	case ChecksumSHA256, ChecksumCRC32:
		return true
	default:
		return false
	}
}

// String returns the string representation of the ChecksumAlgorithm.
func (a ChecksumAlgorithm) String() string {
	return string(a)
}

// Domain sentinel errors for epoch synchronizer lifecycle and barrier validation.
var (
	ErrNilEpochBoundary           = errfmt.Errorf("epoch boundary cannot be nil")
	ErrNilCheckpointSnapshot      = errfmt.Errorf("swarm checkpoint snapshot cannot be nil")
	ErrInvalidEpochID             = errfmt.Errorf("invalid epoch id")
	ErrInvalidCheckpointID        = errfmt.Errorf("invalid checkpoint id")
	ErrInvalidEpochState          = errfmt.Errorf("invalid epoch state")
	ErrInvalidChecksumAlgorithm   = errfmt.Errorf("invalid checksum algorithm")
	ErrInvalidMonotonicTransition = errfmt.Errorf("invalid monotonic epoch transition")
	ErrCorruptPayload             = errfmt.Errorf("corrupt checkpoint payload")
	ErrEmptyPayload               = ErrCorruptPayload
	ErrStaleEpoch                 = errfmt.Errorf("stale or retroactive epoch transition rejected")
	ErrChecksumMismatch           = errfmt.Errorf("state checksum mismatch detected")
	ErrBarrierTimeout             = errfmt.Errorf("epoch barrier synchronization timed out")
	ErrEpochTimedOut              = ErrBarrierTimeout
	ErrParticipantNotRegistered   = errfmt.Errorf("participant not registered in epoch")
	ErrAgentNotRegistered         = ErrParticipantNotRegistered
	ErrNilEpochSynchronizer       = errfmt.Errorf("epoch synchronizer receiver is nil")
	ErrEmptyAgentID               = errfmt.Errorf("agent id cannot be empty")
	ErrEmptyParticipantID         = ErrEmptyAgentID
	ErrEmptyStateHash             = errfmt.Errorf("state hash cannot be empty")
	ErrAgentAlreadyRegistered     = errfmt.Errorf("agent already registered")
	ErrEpochAlreadyStarted        = errfmt.Errorf("active epoch already in progress")
	ErrEpochNotActive             = errfmt.Errorf("epoch is not active")
	ErrEpochMismatch              = errfmt.Errorf("epoch number mismatch")
	ErrEpochBarrierNotMet         = errfmt.Errorf("barrier participants not met")
	ErrParticipantAlreadyArrived  = errfmt.Errorf("participant already arrived at barrier")
	ErrMinParticipantsNotMet      = errfmt.Errorf("registered participants below minimum")
)

// ParticipantArrival captures an individual agent arrival record at the barrier.
type ParticipantArrival struct {
	AgentID   string    `json:"agent_id"`
	StateHash string    `json:"state_hash"`
	ArrivedAt time.Time `json:"arrived_at"`
}

// EpochBoundary defines the consensus synchronization coordination point for an epoch.
type EpochBoundary struct {
	EpochID          string                         `json:"epoch_id"`
	EpochNumber      uint64                         `json:"epoch_number"`
	State            EpochState                     `json:"state"`
	Status           EpochState                     `json:"status,omitempty"`
	StartedAt        time.Time                      `json:"started_at"`
	CompletedAt      time.Time                      `json:"completed_at,omitempty"`
	BarrieredAt      time.Time                      `json:"barriered_at,omitempty"`
	CommittedAt      time.Time                      `json:"committed_at,omitempty"`
	SnapshotHash     string                         `json:"snapshot_hash,omitempty"`
	ParticipantCount int                            `json:"participant_count"`
	Arrivals         map[string]*ParticipantArrival `json:"arrivals,omitempty"`
	Metadata         map[string]string              `json:"metadata,omitempty"`
}

// Validate checks whether the epoch boundary satisfies static floor invariants.
func (b *EpochBoundary) Validate() error {
	if b == nil {
		return ErrNilEpochBoundary
	}
	if strings.TrimSpace(b.EpochID) == "" {
		return ErrInvalidEpochID
	}
	if !b.State.IsValid() {
		return ErrInvalidEpochState
	}
	return nil
}

// Clone creates a detached shallow copy of the epoch boundary.
func (b *EpochBoundary) Clone() *EpochBoundary {
	if b == nil {
		return nil
	}
	dst := *b
	if len(b.Arrivals) > 0 {
		dst.Arrivals = make(map[string]*ParticipantArrival, len(b.Arrivals))
		for k, v := range b.Arrivals {
			arr := *v
			dst.Arrivals[k] = &arr
		}
	}
	if len(b.Metadata) > 0 {
		dst.Metadata = make(map[string]string, len(b.Metadata))
		for k, v := range b.Metadata {
			dst.Metadata[k] = v
		}
	}
	return &dst
}

// Serialize encodes the EpochBoundary into a deterministic JSON byte payload.
func (b *EpochBoundary) Serialize() ([]byte, error) {
	if b == nil {
		return nil, ErrNilEpochBoundary
	}
	return json.Marshal(b)
}

// DeserializeEpochBoundary decodes a serialized JSON payload into an EpochBoundary.
func DeserializeEpochBoundary(data []byte) (*EpochBoundary, error) {
	if len(data) == 0 {
		return nil, ErrEmptyPayload
	}
	var boundary EpochBoundary
	if err := json.Unmarshal(data, &boundary); err != nil {
		return nil, err
	}
	if err := boundary.Validate(); err != nil {
		return nil, err
	}
	return &boundary, nil
}

// SwarmCheckpointSnapshot records the immutable distributed state across swarm agents.
type SwarmCheckpointSnapshot struct {
	CheckpointID      string            `json:"checkpoint_id"`
	EpochNumber       uint64            `json:"epoch_number"`
	EpochID           uint64            `json:"epoch_id,omitempty"`
	Timestamp         time.Time         `json:"timestamp"`
	CommittedAt       time.Time         `json:"committed_at,omitempty"`
	StateChecksum     string            `json:"state_checksum"`
	AggregateChecksum string            `json:"aggregate_checksum,omitempty"`
	Checksum          string            `json:"checksum,omitempty"`
	Algorithm         ChecksumAlgorithm `json:"algorithm"`
	AgentStates       map[string]string `json:"agent_states,omitempty"`
	ParticipantHashes map[string]string `json:"participant_hashes,omitempty"`
	StateData         map[string]string `json:"state_data,omitempty"`
	IntentOffsets     map[string]uint64 `json:"intent_offsets,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// EpochCheckpoint is an alias for SwarmCheckpointSnapshot.
type EpochCheckpoint = SwarmCheckpointSnapshot

// Validate checks whether the swarm checkpoint snapshot satisfies static floor invariants.
func (s *SwarmCheckpointSnapshot) Validate() error {
	if s == nil {
		return ErrNilCheckpointSnapshot
	}
	if strings.TrimSpace(s.CheckpointID) == "" {
		return ErrInvalidCheckpointID
	}
	if !s.Algorithm.IsValid() {
		return ErrInvalidChecksumAlgorithm
	}
	return nil
}

// Clone creates a detached shallow copy of the checkpoint snapshot.
func (s *SwarmCheckpointSnapshot) Clone() *SwarmCheckpointSnapshot {
	if s == nil {
		return nil
	}
	dst := *s
	if len(s.AgentStates) > 0 {
		dst.AgentStates = make(map[string]string, len(s.AgentStates))
		for k, v := range s.AgentStates {
			dst.AgentStates[k] = v
		}
		dst.ParticipantHashes = dst.AgentStates
		dst.StateData = dst.AgentStates
	}
	if len(s.IntentOffsets) > 0 {
		dst.IntentOffsets = make(map[string]uint64, len(s.IntentOffsets))
		for k, v := range s.IntentOffsets {
			dst.IntentOffsets[k] = v
		}
	}
	if len(s.Metadata) > 0 {
		dst.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			dst.Metadata[k] = v
		}
	}
	return &dst
}

// Serialize encodes the SwarmCheckpointSnapshot into a deterministic JSON byte payload.
func (s *SwarmCheckpointSnapshot) Serialize() ([]byte, error) {
	if s == nil {
		return nil, ErrNilCheckpointSnapshot
	}
	return json.Marshal(s)
}

// DeserializeCheckpointSnapshot decodes a serialized JSON payload into a SwarmCheckpointSnapshot.
func DeserializeCheckpointSnapshot(data []byte) (*SwarmCheckpointSnapshot, error) {
	if len(data) == 0 {
		return nil, ErrEmptyPayload
	}
	var snapshot SwarmCheckpointSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// ValidateMonotonicTransition validates that an epoch boundary transition is strictly sequential.
func ValidateMonotonicTransition(from, to uint64) error {
	if to <= from || to != from+1 {
		return ErrInvalidMonotonicTransition
	}
	return nil
}

// ComputeStateChecksum calculates a formatted digest using the specified algorithm.
func ComputeStateChecksum(data []byte, algo ChecksumAlgorithm) (string, error) {
	switch algo {
	case ChecksumSHA256:
		digest := sha256.Sum256(data)
		return hex.EncodeToString(digest[:]), nil
	case ChecksumCRC32:
		crcVal := crc32.ChecksumIEEE(data)
		return fmt.Sprintf("%08x", crcVal), nil
	default:
		return "", ErrInvalidChecksumAlgorithm
	}
}

// ComputeChecksum calculates the deterministic SHA-256 hash of checkpoint state key-value entries.
func ComputeChecksum(data map[string]string) string {
	raw, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// MarshalCheckpoint serializes an EpochCheckpoint into a byte stream with computed checksum.
func MarshalCheckpoint(cp EpochCheckpoint) ([]byte, error) {
	if cp.StateData == nil {
		cp.StateData = make(map[string]string)
	}
	if cp.AgentStates == nil {
		cp.AgentStates = cp.StateData
	}
	if cp.Algorithm == "" {
		cp.Algorithm = ChecksumSHA256
	}
	checksum := ComputeChecksum(cp.StateData)
	cp.Checksum = checksum
	cp.StateChecksum = checksum
	cp.AggregateChecksum = checksum
	if cp.CheckpointID == "" {
		cp.CheckpointID = fmt.Sprintf("chk-%d", cp.EpochID)
	}
	if cp.EpochNumber == 0 && cp.EpochID > 0 {
		cp.EpochNumber = cp.EpochID
	}
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now().UTC()
	}
	return json.Marshal(cp)
}

// UnmarshalCheckpoint deserializes raw bytes into an EpochCheckpoint, verifying integrity.
func UnmarshalCheckpoint(data []byte) (*EpochCheckpoint, error) {
	if len(data) == 0 {
		return nil, ErrCorruptPayload
	}
	var cp EpochCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, ErrCorruptPayload
	}
	if cp.StateData == nil && cp.AgentStates != nil {
		cp.StateData = cp.AgentStates
	}
	if cp.StateData == nil {
		return nil, ErrCorruptPayload
	}
	expected := ComputeChecksum(cp.StateData)
	chk := cp.StateChecksum
	if chk == "" {
		chk = cp.Checksum
	}
	if chk == "" || chk != expected {
		return nil, ErrChecksumMismatch
	}
	if cp.Checksum != "" && cp.Checksum != expected {
		return nil, ErrChecksumMismatch
	}
	if cp.StateChecksum != "" && cp.StateChecksum != expected {
		return nil, ErrChecksumMismatch
	}
	if cp.AggregateChecksum != "" && cp.AggregateChecksum != expected {
		return nil, ErrChecksumMismatch
	}
	if cp.EpochNumber == 0 && cp.EpochID > 0 {
		cp.EpochNumber = cp.EpochID
	}
	return &cp, nil
}

// EpochSynchronizerConfig defines operational thresholds for multi-agent synchronization.
type EpochSynchronizerConfig struct {
	EpochTimeout    time.Duration     `json:"epoch_timeout"`
	MinParticipants int               `json:"min_participants"`
	ChecksumAlgo    ChecksumAlgorithm `json:"checksum_algo"`
}

// DefaultEpochSynchronizerConfig returns production defaults for epoch synchronization.
func DefaultEpochSynchronizerConfig() EpochSynchronizerConfig {
	return EpochSynchronizerConfig{
		EpochTimeout:    DefaultEpochTimeout,
		MinParticipants: DefaultMinParticipants,
		ChecksumAlgo:    ChecksumAlgorithmSHA256,
	}
}

// EpochSynchronizer coordinates multi-agent barrier synchronization and epoch snapshotting.
type EpochSynchronizer struct {
	mu           sync.RWMutex
	config       EpochSynchronizerConfig
	participants map[string]bool
	currentEpoch *EpochBoundary
	history      []*SwarmCheckpointSnapshot
	barrierCh    chan struct{}
}

// NewEpochSynchronizer initializes an EpochSynchronizer with configuration defaults.
func NewEpochSynchronizer(config EpochSynchronizerConfig) *EpochSynchronizer {
	if config.EpochTimeout <= 0 {
		config.EpochTimeout = DefaultEpochTimeout
	}
	if config.MinParticipants <= 0 {
		config.MinParticipants = DefaultMinParticipants
	}
	if config.ChecksumAlgo == "" {
		config.ChecksumAlgo = ChecksumAlgorithmSHA256
	}

	return &EpochSynchronizer{
		config:       config,
		participants: make(map[string]bool),
		history:      make([]*SwarmCheckpointSnapshot, 0, DefaultMaxEpochHistory),
	}
}

// NewEpochSynchronizerWithParticipants initializes an EpochSynchronizer with registered agents.
func NewEpochSynchronizerWithParticipants(participants []string, config ...EpochSynchronizerConfig) *EpochSynchronizer {
	var cfg EpochSynchronizerConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	syncer := NewEpochSynchronizer(cfg)
	for _, p := range participants {
		if err := syncer.RegisterParticipant(p); err != nil {
			continue
		}
	}
	return syncer
}

func (s *EpochSynchronizer) withLock(fn func() error) error {
	if s == nil {
		return ErrNilEpochSynchronizer
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn()
}

func (s *EpochSynchronizer) withRLock(fn func()) error {
	if s == nil {
		return ErrNilEpochSynchronizer
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn()
	return nil
}

func (s *EpochSynchronizer) validateAgentID(agentID string) (string, error) {
	if s == nil {
		return "", ErrNilEpochSynchronizer
	}
	clean := strings.TrimSpace(agentID)
	if clean == "" {
		return "", ErrEmptyAgentID
	}
	return clean, nil
}

func (s *EpochSynchronizer) mutateParticipant(agentID string, mutate func(cleanID string) error) error {
	cleanID, err := s.validateAgentID(agentID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return mutate(cleanID)
}

// RegisterParticipant registers an agent ID for barrier tracking.
func (s *EpochSynchronizer) RegisterParticipant(agentID string) error {
	return s.mutateParticipant(agentID, func(cleanID string) error {
		if s.participants[cleanID] {
			return ErrAgentAlreadyRegistered
		}
		s.participants[cleanID] = true
		return nil
	})
}

// DeregisterParticipant removes an agent ID from barrier tracking.
func (s *EpochSynchronizer) DeregisterParticipant(agentID string) error {
	return s.mutateParticipant(agentID, func(cleanID string) error {
		if !s.participants[cleanID] {
			return ErrParticipantNotRegistered
		}
		delete(s.participants, cleanID)
		return nil
	})
}

// StartEpoch initiates a new epoch cycle and stages the barrier rendezvous.
func (s *EpochSynchronizer) StartEpoch(ctx context.Context, epochNumber uint64) (*EpochBoundary, error) {
	var boundary *EpochBoundary
	err := s.withLock(func() error {
		if len(s.participants) < s.config.MinParticipants {
			return ErrMinParticipantsNotMet
		}
		if s.currentEpoch != nil {
			if s.currentEpoch.State == EpochStateActive || s.currentEpoch.State == EpochStateSynchronizing {
				return ErrEpochAlreadyStarted
			}
			if epochNumber <= s.currentEpoch.EpochNumber {
				return ErrStaleEpoch
			}
			if transErr := ValidateMonotonicTransition(s.currentEpoch.EpochNumber, epochNumber); transErr != nil {
				return transErr
			}
		}
		now := time.Now().UTC()
		boundary = &EpochBoundary{
			EpochID:          fmt.Sprintf("epoch-%d-%d", epochNumber, now.UnixNano()),
			EpochNumber:      epochNumber,
			State:            EpochStateActive,
			Status:           EpochStateActive,
			StartedAt:        now,
			ParticipantCount: len(s.participants),
			Arrivals:         make(map[string]*ParticipantArrival),
			Metadata:         make(map[string]string),
		}
		s.currentEpoch = boundary
		s.barrierCh = make(chan struct{})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return boundary.Clone(), nil
}

// ArriveAtBarrier records a participant arrival and blocks until all participants arrive or timeout.
func (s *EpochSynchronizer) ArriveAtBarrier(
	ctx context.Context,
	agentID string,
	epochNumber uint64,
	stateHash string,
) (*EpochBoundary, error) {
	cleanID, err := s.validateAgentID(agentID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(stateHash) == "" {
		return nil, ErrEmptyStateHash
	}

	barrierCh, errRecord := s.registerBarrierArrival(cleanID, epochNumber, stateHash)
	if errRecord != nil {
		return nil, errRecord
	}

	return s.awaitBarrierRelease(ctx, barrierCh)
}

func (s *EpochSynchronizer) registerBarrierArrival(agentID string, epochNumber uint64, stateHash string) (chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.participants[agentID] {
		return nil, ErrParticipantNotRegistered
	}
	if s.currentEpoch == nil || (s.currentEpoch.State != EpochStateActive && s.currentEpoch.State != EpochStateSynchronizing) {
		return nil, ErrEpochNotActive
	}
	if epochNumber < s.currentEpoch.EpochNumber {
		return nil, ErrStaleEpoch
	}
	if s.currentEpoch.EpochNumber != epochNumber {
		return nil, ErrEpochMismatch
	}
	if _, exists := s.currentEpoch.Arrivals[agentID]; exists {
		return nil, ErrParticipantAlreadyArrived
	}

	now := time.Now().UTC()
	s.currentEpoch.Arrivals[agentID] = &ParticipantArrival{
		AgentID:   agentID,
		StateHash: stateHash,
		ArrivedAt: now,
	}

	barrierCh := s.barrierCh
	if len(s.currentEpoch.Arrivals) >= len(s.participants) {
		s.currentEpoch.State = EpochStateSynchronizing
		s.currentEpoch.Status = EpochStatusBarriered
		s.currentEpoch.BarrieredAt = now
		s.currentEpoch.CompletedAt = now
		close(barrierCh)
	}

	return barrierCh, nil
}

func (s *EpochSynchronizer) awaitBarrierRelease(ctx context.Context, barrierCh chan struct{}) (*EpochBoundary, error) {
	timeout := s.config.EpochTimeout
	if timeout <= 0 {
		timeout = DefaultEpochTimeout
	}

	select {
	case <-barrierCh:
		return s.CurrentEpoch(), nil
	case <-ctx.Done():
		return nil, errors.Join(ErrBarrierTimeout, ctx.Err())
	case <-time.After(timeout):
		return nil, ErrBarrierTimeout
	}
}

func (s *EpochSynchronizer) finalizeEpochCommit(epochNumber uint64) (*SwarmCheckpointSnapshot, error) {
	if s.currentEpoch == nil || s.currentEpoch.EpochNumber != epochNumber {
		return nil, ErrEpochMismatch
	}
	if s.currentEpoch.State != EpochStateSynchronizing && s.currentEpoch.Status != EpochStatusBarriered {
		return nil, ErrEpochBarrierNotMet
	}

	keys := make([]string, 0, len(s.currentEpoch.Arrivals))
	for k := range s.currentEpoch.Arrivals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	participantHashes := make(map[string]string, len(keys))
	var digestBuffer strings.Builder
	for _, k := range keys {
		arr := s.currentEpoch.Arrivals[k]
		participantHashes[k] = arr.StateHash
		digestBuffer.WriteString(k)
		digestBuffer.WriteString("=")
		digestBuffer.WriteString(arr.StateHash)
		digestBuffer.WriteString(";")
	}

	algo := s.config.ChecksumAlgo
	if algo == "" {
		algo = ChecksumSHA256
	}
	aggregateChecksum, err := ComputeStateChecksum([]byte(digestBuffer.String()), algo)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	s.currentEpoch.State = EpochStateCommitted
	s.currentEpoch.Status = EpochStatusCommitted
	s.currentEpoch.CommittedAt = now
	s.currentEpoch.CompletedAt = now
	s.currentEpoch.SnapshotHash = aggregateChecksum

	snapshot := &SwarmCheckpointSnapshot{
		CheckpointID:      fmt.Sprintf("chk-%d-%d", epochNumber, now.UnixNano()),
		EpochNumber:       epochNumber,
		EpochID:           epochNumber,
		Timestamp:         now,
		CommittedAt:       now,
		StateChecksum:     aggregateChecksum,
		AggregateChecksum: aggregateChecksum,
		Checksum:          aggregateChecksum,
		Algorithm:         algo,
		AgentStates:       participantHashes,
		ParticipantHashes: participantHashes,
		StateData:         participantHashes,
		IntentOffsets:     make(map[string]uint64),
		Metadata:          make(map[string]string),
	}
	s.history = append(s.history, snapshot)
	return snapshot.Clone(), nil
}

// CommitEpoch calculates aggregate state checksum and records an immutable checkpoint snapshot.
func (s *EpochSynchronizer) CommitEpoch(ctx context.Context, epochNumber uint64) (*SwarmCheckpointSnapshot, error) {
	var snapshot *SwarmCheckpointSnapshot
	err := s.withLock(func() error {
		snap, errCommit := s.finalizeEpochCommit(epochNumber)
		if errCommit != nil {
			return errCommit
		}
		snapshot = snap
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// CurrentEpoch returns a detached snapshot of the current active or barriered epoch.
func (s *EpochSynchronizer) CurrentEpoch() *EpochBoundary {
	var cur *EpochBoundary
	if err := s.withRLock(func() {
		if s.currentEpoch != nil {
			cur = s.currentEpoch.Clone()
		}
	}); err != nil {
		return nil
	}
	return cur
}

// GetHistory returns a copy of historical committed checkpoint snapshots.
func (s *EpochSynchronizer) GetHistory() []*SwarmCheckpointSnapshot {
	var result []*SwarmCheckpointSnapshot
	if err := s.withRLock(func() {
		result = make([]*SwarmCheckpointSnapshot, len(s.history))
		for i, h := range s.history {
			result[i] = h.Clone()
		}
	}); err != nil {
		return []*SwarmCheckpointSnapshot{}
	}
	return result
}

// AdvanceEpoch manually advances the epoch counter monotonically for testing and testing setup.
func (s *EpochSynchronizer) AdvanceEpoch(targetEpoch uint64) error {
	return s.withLock(func() error {
		cur := uint64(0)
		if s.currentEpoch != nil {
			cur = s.currentEpoch.EpochNumber
		}
		if targetEpoch == 0 || targetEpoch <= cur {
			return ErrStaleEpoch
		}

		now := time.Now().UTC()
		s.currentEpoch = &EpochBoundary{
			EpochID:          fmt.Sprintf("epoch-%d-%d", targetEpoch, now.UnixNano()),
			EpochNumber:      targetEpoch,
			State:            EpochStateActive,
			Status:           EpochStateActive,
			StartedAt:        now,
			ParticipantCount: len(s.participants),
			Arrivals:         make(map[string]*ParticipantArrival),
			Metadata:         make(map[string]string),
		}
		s.barrierCh = make(chan struct{})
		return nil
	})
}

// Arrive signals participant arrival at the given epoch barrier and waits for rendezvous.
func (s *EpochSynchronizer) Arrive(ctx context.Context, participantID string, epoch uint64) error {
	cleanID, err := s.validateAgentID(participantID)
	if err != nil {
		return err
	}

	s.mu.RLock()
	if !s.participants[cleanID] {
		s.mu.RUnlock()
		return ErrParticipantNotRegistered
	}
	curEpoch := uint64(0)
	if s.currentEpoch != nil {
		curEpoch = s.currentEpoch.EpochNumber
	}
	if epoch == 0 || epoch < curEpoch {
		s.mu.RUnlock()
		return ErrStaleEpoch
	}
	s.mu.RUnlock()

	stateHash := fmt.Sprintf("state-%s-%d", cleanID, epoch)
	_, errBarrier := s.ArriveAtBarrier(ctx, cleanID, epoch, stateHash)
	return errBarrier
}

// CommitCheckpoint verifies and records a serialized checkpoint, advancing history.
func (s *EpochSynchronizer) CommitCheckpoint(epoch uint64, rawPayload []byte) (*EpochCheckpoint, error) {
	if s == nil {
		return nil, ErrNilEpochSynchronizer
	}
	cp, err := UnmarshalCheckpoint(rawPayload)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	curEpoch := uint64(0)
	if s.currentEpoch != nil {
		curEpoch = s.currentEpoch.EpochNumber
	}
	if epoch == 0 || epoch < curEpoch || cp.EpochNumber < curEpoch {
		return nil, ErrStaleEpoch
	}

	s.history = append(s.history, cp)
	return cp.Clone(), nil
}

// LatestCheckpoint returns the most recently committed checkpoint in history, or nil if none.
func (s *EpochSynchronizer) LatestCheckpoint() *SwarmCheckpointSnapshot {
	var latest *SwarmCheckpointSnapshot
	if err := s.withRLock(func() {
		if len(s.history) > 0 {
			latest = s.history[len(s.history)-1].Clone()
		}
	}); err != nil {
		return nil
	}
	return latest
}

// History returns a copy of historical checkpoints as values.
func (s *EpochSynchronizer) History() []SwarmCheckpointSnapshot {
	var result []SwarmCheckpointSnapshot
	if err := s.withRLock(func() {
		result = make([]SwarmCheckpointSnapshot, len(s.history))
		for i, h := range s.history {
			result[i] = *h.Clone()
		}
	}); err != nil {
		return []SwarmCheckpointSnapshot{}
	}
	return result
}

// RecoverFromSnapshot unmarshals a checkpoint payload, falling back to the provided checkpoint if corrupt.
func (s *EpochSynchronizer) RecoverFromSnapshot(rawPayload []byte, fallback *SwarmCheckpointSnapshot) (*SwarmCheckpointSnapshot, error) {
	if s == nil {
		return nil, ErrNilEpochSynchronizer
	}
	cp, err := UnmarshalCheckpoint(rawPayload)
	if err != nil {
		if fallback != nil {
			return fallback.Clone(), err
		}
		return nil, err
	}
	return cp.Clone(), nil
}

// Close unblocks all pending barrier waiters and marks the synchronizer terminated.
func (s *EpochSynchronizer) Close() error {
	return s.withLock(func() error {
		if s.barrierCh != nil {
			select {
			case <-s.barrierCh:
			default:
				close(s.barrierCh)
			}
		}
		return nil
	})
}
