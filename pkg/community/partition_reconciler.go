package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	// PartitionStateHealthy indicates all validators are converged on a single branch.
	PartitionStateHealthy PartitionState = "healthy"
	// PartitionStatePartitioned indicates a minority or isolated cluster segment was detected.
	PartitionStatePartitioned PartitionState = "partitioned"
	// PartitionStateSplitBrain indicates multiple divergent branches exist for the active epoch.
	PartitionStateSplitBrain PartitionState = "split_brain"
	// PartitionStateReconciling indicates partition heal arbitration is actively running.
	PartitionStateReconciling PartitionState = "reconciling"

	errPartitionBlankNodeMsg      = "local node ID cannot be blank"
	errPartitionBlankBranchMsg    = "branch ID cannot be blank"
	errPartitionInvalidTotalMsg   = "total validators must be positive"
	errPartitionInvalidQuorumMsg  = "quorum threshold must be a strict majority within total validators"
	errPartitionEmptyBranchesMsg  = "at least one partition branch is required"
	errPartitionDuplicateVoterMsg = "duplicate or blank voter ID in branch"
	errPartitionEmptyClockMsg     = "branch vector clock cannot be empty"
)

var (
	// ErrInvalidPartitionConfig indicates a partition reconciler configuration violates static invariants.
	ErrInvalidPartitionConfig = errors.New("invalid partition reconciler configuration")

	// ErrSubQuorumBranch indicates no branch in the heal set satisfies the strict majority quorum threshold.
	ErrSubQuorumBranch = errors.New("sub-quorum branch rejected: strict majority required")

	// ErrAmbiguousQuorum indicates multiple distinct branches simultaneously claim majority quorum.
	ErrAmbiguousQuorum = errors.New("ambiguous quorum: multiple majority branches detected")

	// ErrForgedBranchDigest indicates a branch state hash or digest failed deterministic SHA-256 verification.
	ErrForgedBranchDigest = errors.New("forged or mismatched branch state digest")

	// ErrNonMonotonicVectorClock indicates a branch vector clock regressed or contains invalid entries.
	ErrNonMonotonicVectorClock = errors.New("non-monotonic or invalid vector clock")

	// ErrStalePartitionRollback indicates an attempt to reconcile or apply a stale epoch or rolled-back state.
	ErrStalePartitionRollback = errors.New("stale partition rollback rejected")
)

// PartitionState represents the convergence state of a multi-cluster partition reconciler.
type PartitionState string

// IsValid reports whether the partition state is a recognized lifecycle value.
func (s PartitionState) IsValid() bool {
	switch s {
	case PartitionStateHealthy, PartitionStatePartitioned, PartitionStateSplitBrain, PartitionStateReconciling:
		return true
	default:
		return false
	}
}

// StrictMajorityQuorum returns the minimum validator count required for a strict majority ((total/2)+1).
func StrictMajorityQuorum(totalValidators int) int {
	if totalValidators <= 0 {
		return 0
	}
	return (totalValidators / 2) + 1
}

// PartitionReconcilerConfig parameterizes cluster identity, validator count, and quorum thresholds.
type PartitionReconcilerConfig struct {
	ClusterID       string
	LocalNodeID     string
	TotalValidators int
	QuorumThreshold int
}

// Validate enforces static floor invariants on cluster identity and quorum thresholds.
func (c PartitionReconcilerConfig) Validate() error {
	if strutil.IsBlank(c.ClusterID) {
		return ErrBlankClusterID
	}
	if strutil.IsBlank(c.LocalNodeID) {
		return fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionBlankNodeMsg)
	}
	if c.TotalValidators <= 0 {
		return fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionInvalidTotalMsg)
	}
	minMajority := StrictMajorityQuorum(c.TotalValidators)
	threshold := c.effectiveQuorum()
	if threshold < minMajority || threshold > c.TotalValidators {
		return fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionInvalidQuorumMsg)
	}
	return nil
}

func (c PartitionReconcilerConfig) effectiveQuorum() int {
	if c.QuorumThreshold == 0 {
		return StrictMajorityQuorum(c.TotalValidators)
	}
	return c.QuorumThreshold
}

// PartitionVectorClock maps node identifiers to monotonically increasing logical counters.
type PartitionVectorClock map[string]uint64

// Clone returns a deep copy of the vector clock.
func (vc PartitionVectorClock) Clone() PartitionVectorClock {
	if vc == nil {
		return nil
	}
	out := make(PartitionVectorClock, len(vc))
	for k, v := range vc {
		out[k] = v
	}
	return out
}

// Validate checks that the vector clock is non-empty and has no blank node IDs or zero counters.
func (vc PartitionVectorClock) Validate() error {
	if len(vc) == 0 {
		return fmt.Errorf("%w: %s", ErrNonMonotonicVectorClock, errPartitionEmptyClockMsg)
	}
	for nodeID, counter := range vc {
		if strutil.IsBlank(nodeID) || counter == 0 {
			return ErrNonMonotonicVectorClock
		}
	}
	return nil
}

// DominatesOrEqual reports whether vc is element-wise >= other for all keys in other.
func (vc PartitionVectorClock) DominatesOrEqual(other PartitionVectorClock) bool {
	for nodeID, otherVal := range other {
		if vc[nodeID] < otherVal {
			return false
		}
	}
	return true
}

// Merge returns a new PartitionVectorClock containing the element-wise maximum of vc and other.
func (vc PartitionVectorClock) Merge(other PartitionVectorClock) PartitionVectorClock {
	merged := vc.Clone()
	if merged == nil {
		merged = make(PartitionVectorClock, len(other))
	}
	for nodeID, val := range other {
		if val > merged[nodeID] {
			merged[nodeID] = val
		}
	}
	return merged
}

// StateDelta represents a key-value state mutation recorded on a partition branch.
type StateDelta struct {
	Key        string
	Value      string
	Version    uint64
	OriginNode string
}

// PartitionBranch captures a divergent or converged cluster branch with quorum votes and causal history.
type PartitionBranch struct {
	BranchID             string
	ClusterID            string
	Epoch                uint64
	PartitionVectorClock PartitionVectorClock
	VoterIDs             []string
	Deltas               []StateDelta
	StateHash            string
	BranchDigest         string
}

// ComputeStateHash computes the deterministic SHA-256 digest of the branch's sorted state deltas.
func (b PartitionBranch) ComputeStateHash() (string, error) {
	h := sha256.New()
	sorted := sortedDeltas(b.Deltas)
	if err := writeHashUint64(h, uint64(len(sorted))); err != nil {
		return "", err
	}
	for _, d := range sorted {
		if err := writeDeltaToHash(h, d); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeBranchDigest computes the deterministic SHA-256 digest over all canonical branch fields.
func (b PartitionBranch) ComputeBranchDigest() (string, error) {
	h := sha256.New()
	if err := writeHashStrings(h, b.BranchID, b.ClusterID, b.StateHash); err != nil {
		return "", err
	}
	if err := writeHashUint64(h, b.Epoch); err != nil {
		return "", err
	}
	if err := writeVectorClockToHash(h, b.PartitionVectorClock); err != nil {
		return "", err
	}
	if err := writeSortedStringsToHash(h, b.VoterIDs); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Seal computes and populates StateHash and BranchDigest on the branch.
func (b *PartitionBranch) Seal() error {
	stateHash, err := b.ComputeStateHash()
	if err != nil {
		return err
	}
	b.StateHash = stateHash
	digest, err := b.ComputeBranchDigest()
	if err != nil {
		return err
	}
	b.BranchDigest = digest
	return nil
}

// BranchHistoryEntry records an audited branch observation in the reconciler log.
type BranchHistoryEntry struct {
	BranchID             string
	Epoch                uint64
	VoterCount           int
	PartitionVectorClock PartitionVectorClock
	StateHash            string
	BranchDigest         string
	StateAtEntry         PartitionState
}

// ReconciliationReport summarizes the outcome of split-brain arbitration and state unification.
type ReconciliationReport struct {
	WinningBranchID string
	WinningEpoch    uint64
	NewEpoch        uint64
	MergedDeltaKeys []string
	UnifiedClock    PartitionVectorClock
	UnifiedState    map[string]string
	PartitionState  PartitionState
}

// PartitionReconciler detects multi-cluster partitions, logs branch history, and arbitrates split-brain heals.
type PartitionReconciler struct {
	mu              sync.RWMutex
	config          PartitionReconcilerConfig
	quorumThreshold int
	partitionState  PartitionState
	currentEpoch    uint64
	unifiedClock    PartitionVectorClock
	state           map[string]string
	history         []BranchHistoryEntry
	activeBranches  map[string]PartitionBranch
}

// NewPartitionReconciler constructs a validated PartitionReconciler.
func NewPartitionReconciler(cfg PartitionReconcilerConfig) (*PartitionReconciler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &PartitionReconciler{
		config:          cfg,
		quorumThreshold: cfg.effectiveQuorum(),
		partitionState:  PartitionStateHealthy,
		currentEpoch:    1,
		unifiedClock:    make(PartitionVectorClock),
		state:           make(map[string]string),
		history:         make([]BranchHistoryEntry, 0),
		activeBranches:  make(map[string]PartitionBranch),
	}, nil
}

// State returns the current partition state, epoch, and a defensive copy of the unified vector clock.
func (r *PartitionReconciler) State() (PartitionState, uint64, PartitionVectorClock) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.partitionState, r.currentEpoch, r.unifiedClock.Clone()
}

// SnapshotState returns a defensive copy of the unified key-value state.
func (r *PartitionReconciler) SnapshotState() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneStringMap(r.state)
}

// History returns a defensive copy of the branch history log.
func (r *PartitionReconciler) History() []BranchHistoryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]BranchHistoryEntry, len(r.history))
	for i, entry := range r.history {
		entry.PartitionVectorClock = entry.PartitionVectorClock.Clone()
		out[i] = entry
	}
	return out
}

// RecordBranch validates and logs a partition branch, updating the detector's partition state.
func (r *PartitionReconciler) RecordBranch(branch PartitionBranch) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.validateBranchLocked(branch); err != nil {
		return err
	}
	if prev, exists := r.activeBranches[branch.BranchID]; exists {
		if !branch.PartitionVectorClock.DominatesOrEqual(prev.PartitionVectorClock) {
			return ErrNonMonotonicVectorClock
		}
	}

	r.activeBranches[branch.BranchID] = cloneBranch(branch)
	r.updateDetectorStateLocked(branch)
	r.history = append(r.history, BranchHistoryEntry{
		BranchID:             branch.BranchID,
		Epoch:                branch.Epoch,
		VoterCount:           len(branch.VoterIDs),
		PartitionVectorClock: branch.PartitionVectorClock.Clone(),
		StateHash:            branch.StateHash,
		BranchDigest:         branch.BranchDigest,
		StateAtEntry:         r.partitionState,
	})
	return nil
}

// Reconcile arbitrates a set of partition branches upon network heal, selecting the unique majority branch
// and merging non-conflicting deltas without data loss.
func (r *PartitionReconciler) Reconcile(branches []PartitionBranch) (ReconciliationReport, error) {
	if len(branches) == 0 {
		return ReconciliationReport{}, fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionEmptyBranchesMsg)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	majorityBranches, err := r.validateAndPartitionBranchesLocked(branches)
	if err != nil {
		return ReconciliationReport{}, err
	}
	if len(majorityBranches) == 0 {
		return ReconciliationReport{}, ErrSubQuorumBranch
	}
	if len(majorityBranches) > 1 {
		return ReconciliationReport{}, ErrAmbiguousQuorum
	}

	winner := majorityBranches[0]
	nextState, mergedKeys, nextClock := r.unifyBranchesLocked(winner, branches)
	nextEpoch := maxEpoch(branches) + 1

	r.state = nextState
	r.unifiedClock = nextClock
	r.currentEpoch = nextEpoch
	r.partitionState = PartitionStateHealthy
	r.activeBranches = make(map[string]PartitionBranch)
	r.history = append(r.history, BranchHistoryEntry{
		BranchID:             winner.BranchID,
		Epoch:                winner.Epoch,
		VoterCount:           len(winner.VoterIDs),
		PartitionVectorClock: nextClock.Clone(),
		StateHash:            winner.StateHash,
		BranchDigest:         winner.BranchDigest,
		StateAtEntry:         PartitionStateHealthy,
	})

	return ReconciliationReport{
		WinningBranchID: winner.BranchID,
		WinningEpoch:    winner.Epoch,
		NewEpoch:        nextEpoch,
		MergedDeltaKeys: mergedKeys,
		UnifiedClock:    nextClock.Clone(),
		UnifiedState:    cloneStringMap(nextState),
		PartitionState:  PartitionStateHealthy,
	}, nil
}

func (r *PartitionReconciler) validateAndPartitionBranchesLocked(branches []PartitionBranch) ([]PartitionBranch, error) {
	majority := make([]PartitionBranch, 0, len(branches))
	for _, branch := range branches {
		if err := r.validateBranchLocked(branch); err != nil {
			return nil, err
		}
		if len(branch.VoterIDs) >= r.quorumThreshold {
			majority = append(majority, branch)
		}
	}
	return majority, nil
}

func (r *PartitionReconciler) validateBranchLocked(branch PartitionBranch) error {
	if strutil.IsBlank(branch.BranchID) {
		return fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionBlankBranchMsg)
	}
	if strutil.IsBlank(branch.ClusterID) {
		return ErrBlankClusterID
	}
	if branch.ClusterID != r.config.ClusterID {
		return ErrClusterMismatch
	}
	if branch.Epoch < r.currentEpoch {
		return ErrStalePartitionRollback
	}
	if err := branch.PartitionVectorClock.Validate(); err != nil {
		return err
	}
	if !branch.PartitionVectorClock.DominatesOrEqual(r.unifiedClock) {
		return ErrStalePartitionRollback
	}
	if err := validateUniqueVoters(branch.VoterIDs); err != nil {
		return err
	}
	return verifyBranchDigests(branch)
}

func (r *PartitionReconciler) updateDetectorStateLocked(branch PartitionBranch) {
	if len(r.activeBranches) > 1 {
		r.partitionState = PartitionStateSplitBrain
		return
	}
	if len(branch.VoterIDs) < r.quorumThreshold {
		r.partitionState = PartitionStatePartitioned
		return
	}
	r.partitionState = PartitionStateHealthy
}

func (r *PartitionReconciler) unifyBranchesLocked(winner PartitionBranch, branches []PartitionBranch) (map[string]string, []string, PartitionVectorClock) {
	nextState := cloneStringMap(r.state)
	winnerKeys := make(map[string]struct{}, len(winner.Deltas))
	mergedKeySet := make(map[string]struct{})

	for _, d := range sortedDeltas(winner.Deltas) {
		nextState[d.Key] = d.Value
		winnerKeys[d.Key] = struct{}{}
		mergedKeySet[d.Key] = struct{}{}
	}

	minorityApplied := make(map[string]uint64)
	for _, branch := range branches {
		if branch.BranchID == winner.BranchID {
			continue
		}
		for _, d := range sortedDeltas(branch.Deltas) {
			if _, ownedByWinner := winnerKeys[d.Key]; ownedByWinner {
				continue
			}
			prevVer, seen := minorityApplied[d.Key]
			if !seen || d.Version > prevVer {
				nextState[d.Key] = d.Value
				minorityApplied[d.Key] = d.Version
				mergedKeySet[d.Key] = struct{}{}
			}
		}
	}

	nextClock := r.unifiedClock.Clone()
	for _, branch := range branches {
		nextClock = nextClock.Merge(branch.PartitionVectorClock)
	}

	mergedKeys := make([]string, 0, len(mergedKeySet))
	for k := range mergedKeySet {
		mergedKeys = append(mergedKeys, k)
	}
	sort.Strings(mergedKeys)
	return nextState, mergedKeys, nextClock
}

func verifyBranchDigests(branch PartitionBranch) error {
	expectedStateHash, err := branch.ComputeStateHash()
	if err != nil {
		return err
	}
	if branch.StateHash != expectedStateHash {
		return ErrForgedBranchDigest
	}
	expectedDigest, err := branch.ComputeBranchDigest()
	if err != nil {
		return err
	}
	if branch.BranchDigest != expectedDigest {
		return ErrForgedBranchDigest
	}
	return nil
}

func validateUniqueVoters(voters []string) error {
	seen := make(map[string]struct{}, len(voters))
	for _, v := range voters {
		if strutil.IsBlank(v) {
			return fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionDuplicateVoterMsg)
		}
		if _, exists := seen[v]; exists {
			return fmt.Errorf("%w: %s", ErrInvalidPartitionConfig, errPartitionDuplicateVoterMsg)
		}
		seen[v] = struct{}{}
	}
	return nil
}

func sortedDeltas(deltas []StateDelta) []StateDelta {
	out := make([]StateDelta, len(deltas))
	copy(out, deltas)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		if out[i].OriginNode != out[j].OriginNode {
			return out[i].OriginNode < out[j].OriginNode
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func writeDeltaToHash(h hash.Hash, d StateDelta) error {
	if err := writeHashStrings(h, d.Key, d.Value, d.OriginNode); err != nil {
		return err
	}
	return writeHashUint64(h, d.Version)
}

func writeVectorClockToHash(h hash.Hash, vc PartitionVectorClock) error {
	keys := make([]string, 0, len(vc))
	for k := range vc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if err := writeHashUint64(h, uint64(len(keys))); err != nil {
		return err
	}
	for _, k := range keys {
		if err := writeHashString(h, k); err != nil {
			return err
		}
		if err := writeHashUint64(h, vc[k]); err != nil {
			return err
		}
	}
	return nil
}

func writeSortedStringsToHash(h hash.Hash, items []string) error {
	sorted := make([]string, len(items))
	copy(sorted, items)
	sort.Strings(sorted)
	if err := writeHashUint64(h, uint64(len(sorted))); err != nil {
		return err
	}
	for _, s := range sorted {
		if err := writeHashString(h, s); err != nil {
			return err
		}
	}
	return nil
}

func writeHashStrings(h hash.Hash, values ...string) error {
	for _, v := range values {
		if err := writeHashString(h, v); err != nil {
			return err
		}
	}
	return nil
}

func cloneStringMap(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneBranch(b PartitionBranch) PartitionBranch {
	voters := make([]string, len(b.VoterIDs))
	copy(voters, b.VoterIDs)
	deltas := make([]StateDelta, len(b.Deltas))
	copy(deltas, b.Deltas)
	return PartitionBranch{
		BranchID:             b.BranchID,
		ClusterID:            b.ClusterID,
		Epoch:                b.Epoch,
		PartitionVectorClock: b.PartitionVectorClock.Clone(),
		VoterIDs:             voters,
		Deltas:               deltas,
		StateHash:            b.StateHash,
		BranchDigest:         b.BranchDigest,
	}
}

func maxEpoch(branches []PartitionBranch) uint64 {
	var maxVal uint64
	for _, b := range branches {
		if b.Epoch > maxVal {
			maxVal = b.Epoch
		}
	}
	return maxVal
}
