package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// TransitionOp identifies a deterministic state machine mutation operation.
type TransitionOp string

// ReplicaStatus represents the replication health and participation state of a replica.
type ReplicaStatus string

const (
	// OpSet inserts or updates a key-value pair in the replicated state machine.
	OpSet TransitionOp = "set"
	// OpDelete removes a key from the replicated state machine.
	OpDelete TransitionOp = "delete"

	// ReplicaActive indicates the replica is synchronized and accepting entries.
	ReplicaActive ReplicaStatus = "active"
	// ReplicaLagging indicates the replica is behind and requires snapshot catch-up.
	ReplicaLagging ReplicaStatus = "lagging"
	// ReplicaQuarantined indicates the replica diverged and is isolated from the cluster.
	ReplicaQuarantined ReplicaStatus = "quarantined"

	// DefaultTailWindow is the default number of recent log entries retained after compaction.
	DefaultTailWindow = 16
	// DefaultCompactionThreshold is the default log length threshold that triggers compaction.
	DefaultCompactionThreshold = 64

	emptyMerkleInput   = "merkle:empty"
	errReplicaQuar     = "replica %s is quarantined"
	errBlankNodeID     = "node_id cannot be blank"
	errReplicaNotFound = "replica %s not found"
)

// ReplicationConfig defines cluster-wide log compaction and replication parameters.
type ReplicationConfig struct {
	ClusterID           string `json:"cluster_id"`
	TailWindow          int    `json:"tail_window"`
	CompactionThreshold int    `json:"compaction_threshold"`
}

// Validate checks that all ReplicationConfig fields satisfy safety invariants.
func (c ReplicationConfig) Validate() error {
	if err := strutil.ErrorOnBlank(c.ClusterID, "cluster_id cannot be blank"); err != nil {
		return err
	}
	if c.TailWindow < 1 {
		return fmt.Errorf("tail_window must be at least 1, got %d", c.TailWindow)
	}
	if c.CompactionThreshold < c.TailWindow {
		return fmt.Errorf("compaction_threshold (%d) must be >= tail_window (%d)", c.CompactionThreshold, c.TailWindow)
	}
	return nil
}

// LogEntry represents a single deterministic state transition in the replicated log.
type LogEntry struct {
	Sequence  uint64       `json:"sequence"`
	Op        TransitionOp `json:"op"`
	Key       string       `json:"key"`
	Value     string       `json:"value,omitempty"`
	Digest    string       `json:"digest"`
	Timestamp time.Time    `json:"timestamp"`
}

// ComputeEntryDigest computes the deterministic SHA-256 digest for a log entry payload.
func ComputeEntryDigest(seq uint64, op TransitionOp, key, value string) string {
	raw := fmt.Sprintf("%d|%s|%d:%s|%d:%s", seq, op, len(key), key, len(value), value)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewLogEntry constructs and validates a deterministic LogEntry with its computed digest.
func NewLogEntry(seq uint64, op TransitionOp, key, value string) (LogEntry, error) {
	entry := LogEntry{
		Sequence:  seq,
		Op:        op,
		Key:       key,
		Value:     value,
		Digest:    ComputeEntryDigest(seq, op, key, value),
		Timestamp: time.Now().UTC(),
	}
	if err := entry.Validate(); err != nil {
		return LogEntry{}, err
	}
	return entry, nil
}

// Validate checks sequence, operation, key, and digest integrity of a LogEntry.
func (e LogEntry) Validate() error {
	if e.Sequence < 1 {
		return fmt.Errorf("log entry sequence must be >= 1, got %d", e.Sequence)
	}
	if e.Op != OpSet && e.Op != OpDelete {
		return fmt.Errorf("unsupported transition op %q", e.Op)
	}
	if err := strutil.ErrorOnBlank(e.Key, "log entry key cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(e.Digest, "log entry digest cannot be blank"); err != nil {
		return err
	}
	expected := ComputeEntryDigest(e.Sequence, e.Op, e.Key, e.Value)
	if e.Digest != expected {
		return fmt.Errorf("log entry digest mismatch at seq %d: expected %s, got %s", e.Sequence, expected, e.Digest)
	}
	return nil
}

// ComputeMerkleRoot calculates a deterministic key-sorted binary Merkle root over state.
func ComputeMerkleRoot(state map[string]string) string {
	if len(state) == 0 {
		sum := sha256.Sum256([]byte(emptyMerkleInput))
		return hex.EncodeToString(sum[:])
	}
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	level := make([][32]byte, len(keys))
	for i, k := range keys {
		v := state[k]
		leaf := fmt.Sprintf("leaf|%d:%s|%d:%s", len(k), k, len(v), v)
		level[i] = sha256.Sum256([]byte(leaf))
	}
	return reduceMerkleLevel(level)
}

func reduceMerkleLevel(level [][32]byte) string {
	for len(level) > 1 {
		nextLen := (len(level) + 1) / 2
		next := make([][32]byte, nextLen)
		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left
			if i+1 < len(level) {
				right = level[i+1]
			}
			next[i/2] = hashMerklePair(left, right)
		}
		level = next
	}
	return hex.EncodeToString(level[0][:])
}

func hashMerklePair(left, right [32]byte) [32]byte {
	var buf [4 + 64]byte
	copy(buf[:4], "node")
	copy(buf[4:36], left[:])
	copy(buf[36:68], right[:])
	return sha256.Sum256(buf[:])
}

// MerkleSnapshot captures a verifiable point-in-time state snapshot and its Merkle root.
type MerkleSnapshot struct {
	SnapshotID    string            `json:"snapshot_id"`
	Sequence      uint64            `json:"sequence"`
	State         map[string]string `json:"state"`
	MerkleRoot    string            `json:"merkle_root"`
	CreatedAt     time.Time         `json:"created_at"`
	SourceReplica string            `json:"source_replica"`
}

// Validate verifies snapshot metadata and ensures MerkleRoot matches the serialized state.
func (s MerkleSnapshot) Validate() error {
	if err := strutil.ErrorOnBlank(s.SnapshotID, "snapshot_id cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(s.SourceReplica, "source_replica cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(s.MerkleRoot, "merkle_root cannot be blank"); err != nil {
		return err
	}
	computed := ComputeMerkleRoot(s.State)
	if computed != s.MerkleRoot {
		return fmt.Errorf("corrupted snapshot %s: expected merkle root %s, got %s", s.SnapshotID, computed, s.MerkleRoot)
	}
	return nil
}

// ReplicationCompactionReport summarizes a sliding-window log compaction pass on a replica.
type ReplicationCompactionReport struct {
	ReplicaID        string `json:"replica_id"`
	CompactedUpTo    uint64 `json:"compacted_up_to"`
	RetainedFrom     uint64 `json:"retained_from"`
	RetainedCount    int    `json:"retained_count"`
	CompactedEntries int    `json:"compacted_entries"`
	SnapshotID       string `json:"snapshot_id"`
	MerkleRoot       string `json:"merkle_root"`
}

// DivergenceReport records a state root or digest divergence detected on a replica.
type DivergenceReport struct {
	ReplicaID     string    `json:"replica_id"`
	Sequence      uint64    `json:"sequence"`
	ExpectedRoot  string    `json:"expected_root"`
	ObservedRoot  string    `json:"observed_root"`
	Reason        string    `json:"reason"`
	QuarantinedAt time.Time `json:"quarantined_at"`
}

// StateReplica maintains a deterministic replicated state machine, log, and Merkle root.
type StateReplica struct {
	mu             sync.RWMutex
	nodeID         string
	cfg            ReplicationConfig
	status         ReplicaStatus
	appliedSeq     uint64
	compactedUpTo  uint64
	state          map[string]string
	log            []LogEntry
	entryBySeq     map[uint64]LogEntry
	latestSnapshot *MerkleSnapshot
	merkleRoot     string
}

// NewStateReplica initializes a clean StateReplica with validated node ID and config.
func NewStateReplica(nodeID string, cfg ReplicationConfig) (*StateReplica, error) {
	if err := strutil.ErrorOnBlank(nodeID, errBlankNodeID); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &StateReplica{
		nodeID:     nodeID,
		cfg:        cfg,
		status:     ReplicaActive,
		state:      make(map[string]string),
		log:        make([]LogEntry, 0),
		entryBySeq: make(map[uint64]LogEntry),
		merkleRoot: ComputeMerkleRoot(nil),
	}, nil
}

// Apply validates and applies a LogEntry to the replica, enforcing monotonicity and idempotency.
func (r *StateReplica) Apply(entry LogEntry) (bool, error) {
	if err := entry.Validate(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status == ReplicaQuarantined {
		return false, fmt.Errorf(errReplicaQuar, r.nodeID)
	}
	if entry.Sequence <= r.compactedUpTo {
		return false, fmt.Errorf("sequence %d is within compacted range <= %d", entry.Sequence, r.compactedUpTo)
	}
	if existing, ok := r.entryBySeq[entry.Sequence]; ok {
		if existing.Digest == entry.Digest {
			return false, nil
		}
		return false, fmt.Errorf("digest divergence at seq %d: expected %s, got %s", entry.Sequence, existing.Digest, entry.Digest)
	}
	if entry.Sequence != r.appliedSeq+1 {
		return false, fmt.Errorf("out-of-order sequence %d: expected %d", entry.Sequence, r.appliedSeq+1)
	}

	r.applyTransitionLocked(entry)
	return true, nil
}

func (r *StateReplica) applyTransitionLocked(entry LogEntry) {
	switch entry.Op {
	case OpSet:
		r.state[entry.Key] = entry.Value
	case OpDelete:
		delete(r.state, entry.Key)
	}
	r.log = append(r.log, entry)
	r.entryBySeq[entry.Sequence] = entry
	r.appliedSeq = entry.Sequence
	r.merkleRoot = ComputeMerkleRoot(r.state)
}

// Compact trims log entries older than the configured sliding TailWindow and records a snapshot.
func (r *StateReplica) Compact() (ReplicationCompactionReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status == ReplicaQuarantined {
		return ReplicationCompactionReport{}, fmt.Errorf(errReplicaQuar, r.nodeID)
	}
	if len(r.log) <= r.cfg.TailWindow {
		return r.buildNoopReplicationCompactionReportLocked(), nil
	}

	cutIdx := len(r.log) - r.cfg.TailWindow
	for _, old := range r.log[:cutIdx] {
		delete(r.entryBySeq, old.Sequence)
	}
	r.compactedUpTo = r.log[cutIdx-1].Sequence
	retained := make([]LogEntry, len(r.log)-cutIdx)
	copy(retained, r.log[cutIdx:])
	r.log = retained

	snap := r.buildSnapshotLocked()
	r.latestSnapshot = &snap
	return ReplicationCompactionReport{
		ReplicaID:        r.nodeID,
		CompactedUpTo:    r.compactedUpTo,
		RetainedFrom:     r.log[0].Sequence,
		RetainedCount:    len(r.log),
		CompactedEntries: cutIdx,
		SnapshotID:       snap.SnapshotID,
		MerkleRoot:       r.merkleRoot,
	}, nil
}

func (r *StateReplica) buildNoopReplicationCompactionReportLocked() ReplicationCompactionReport {
	var retainedFrom uint64
	if len(r.log) > 0 {
		retainedFrom = r.log[0].Sequence
	}
	return ReplicationCompactionReport{
		ReplicaID:     r.nodeID,
		CompactedUpTo: r.compactedUpTo,
		RetainedFrom:  retainedFrom,
		RetainedCount: len(r.log),
		MerkleRoot:    r.merkleRoot,
	}
}

func (r *StateReplica) buildSnapshotLocked() MerkleSnapshot {
	return MerkleSnapshot{
		SnapshotID:    fmt.Sprintf("snap-%s-%d", r.nodeID, r.appliedSeq),
		Sequence:      r.appliedSeq,
		State:         copyStringMap(r.state),
		MerkleRoot:    r.merkleRoot,
		CreatedAt:     time.Now().UTC(),
		SourceReplica: r.nodeID,
	}
}

// ExportSnapshot returns a verified deep-copy MerkleSnapshot of the replica's current state.
func (r *StateReplica) ExportSnapshot() (MerkleSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.status == ReplicaQuarantined {
		return MerkleSnapshot{}, fmt.Errorf(errReplicaQuar, r.nodeID)
	}
	return r.buildSnapshotLocked(), nil
}

// InstallSnapshot verifies and installs a MerkleSnapshot, fast-forwarding a lagging replica.
func (r *StateReplica) InstallSnapshot(snap MerkleSnapshot) error {
	if err := snap.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status == ReplicaQuarantined {
		return fmt.Errorf(errReplicaQuar, r.nodeID)
	}
	if snap.Sequence < r.appliedSeq {
		return fmt.Errorf("stale snapshot sequence %d is behind replica sequence %d", snap.Sequence, r.appliedSeq)
	}

	r.state = copyStringMap(snap.State)
	r.appliedSeq = snap.Sequence
	r.compactedUpTo = snap.Sequence
	r.merkleRoot = snap.MerkleRoot
	r.log = make([]LogEntry, 0)
	r.entryBySeq = make(map[uint64]LogEntry)
	snapCopy := snap
	snapCopy.State = copyStringMap(snap.State)
	r.latestSnapshot = &snapCopy
	r.status = ReplicaActive
	return nil
}

// Quarantine isolates the replica from further state transitions and snapshot installs.
func (r *StateReplica) Quarantine() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = ReplicaQuarantined
}

// MarkLagging marks the replica as lagging behind the cluster leader.
func (r *StateReplica) MarkLagging() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status != ReplicaQuarantined {
		r.status = ReplicaLagging
	}
}

// NodeID returns the replica identifier.
func (r *StateReplica) NodeID() string {
	return r.nodeID
}

// Status returns the current ReplicaStatus.
func (r *StateReplica) Status() ReplicaStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

// AppliedSequence returns the highest sequence number applied to the replica.
func (r *StateReplica) AppliedSequence() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.appliedSeq
}

// CompactedUpTo returns the highest sequence number compacted from the replica's log.
func (r *StateReplica) CompactedUpTo() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.compactedUpTo
}

// MerkleRoot returns the current deterministic Merkle root of the replica's state.
func (r *StateReplica) MerkleRoot() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.merkleRoot
}

// LogLen returns the number of uncompacted log entries currently retained in the replica.
func (r *StateReplica) LogLen() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.log)
}

// Get retrieves a value by key from the replica's current state.
func (r *StateReplica) Get(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	val, ok := r.state[key]
	return val, ok
}

// Snapshot returns a defensive copy of the replica's key-value state map.
func (r *StateReplica) Snapshot() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return copyStringMap(r.state)
}

// TailEntriesAfter returns retained log entries with Sequence strictly greater than seq.
func (r *StateReplica) TailEntriesAfter(seq uint64) []LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LogEntry, 0)
	for _, entry := range r.log {
		if entry.Sequence > seq {
			out = append(out, entry)
		}
	}
	return out
}

// ReplicationCluster coordinates a leader replica, follower replicas, compaction, and divergence detection.
type ReplicationCluster struct {
	mu          sync.RWMutex
	cfg         ReplicationConfig
	leaderID    string
	replicas    map[string]*StateReplica
	quarantined map[string]DivergenceReport
	nextSeq     uint64
}

// NewReplicationCluster creates a cluster with a validated config and initial leader replica.
func NewReplicationCluster(cfg ReplicationConfig, leaderID string) (*ReplicationCluster, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(leaderID, "leader_id cannot be blank"); err != nil {
		return nil, err
	}
	leader, err := NewStateReplica(leaderID, cfg)
	if err != nil {
		return nil, err
	}
	replicas := map[string]*StateReplica{leaderID: leader}
	return &ReplicationCluster{
		cfg:         cfg,
		leaderID:    leaderID,
		replicas:    replicas,
		quarantined: make(map[string]DivergenceReport),
		nextSeq:     1,
	}, nil
}

// RegisterReplica adds a new follower replica to the cluster.
func (c *ReplicationCluster) RegisterReplica(nodeID string) (*StateReplica, error) {
	if err := strutil.ErrorOnBlank(nodeID, errBlankNodeID); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.replicas[nodeID]; exists {
		return nil, fmt.Errorf("replica %s is already registered", nodeID)
	}
	replica, err := NewStateReplica(nodeID, c.cfg)
	if err != nil {
		return nil, err
	}
	c.replicas[nodeID] = replica
	return replica, nil
}

// GetReplica returns a registered replica by nodeID.
func (c *ReplicationCluster) GetReplica(nodeID string) (*StateReplica, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	replica, ok := c.replicas[nodeID]
	if !ok {
		return nil, fmt.Errorf(errReplicaNotFound, nodeID)
	}
	return replica, nil
}

// ProposeAndReplicate mints the next monotonic LogEntry, applies it to the leader, and replicates to followers.
func (c *ReplicationCluster) ProposeAndReplicate(op TransitionOp, key, value string) (LogEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, err := NewLogEntry(c.nextSeq, op, key, value)
	if err != nil {
		return LogEntry{}, err
	}
	leader := c.replicas[c.leaderID]
	if _, err := leader.Apply(entry); err != nil {
		return LogEntry{}, err
	}
	c.nextSeq++

	for id, replica := range c.replicas {
		if id == c.leaderID || replica.Status() == ReplicaQuarantined {
			continue
		}
		if _, err := replica.Apply(entry); err != nil {
			replica.MarkLagging()
		}
	}
	return entry, nil
}

// CompactAll runs sliding-window compaction on all active replicas whose log exceeds CompactionThreshold.
func (c *ReplicationCluster) CompactAll() ([]ReplicationCompactionReport, error) {
	c.mu.RLock()
	ids := make([]string, 0, len(c.replicas))
	for id := range c.replicas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	c.mu.RUnlock()

	reports := make([]ReplicationCompactionReport, 0, len(ids))
	for _, id := range ids {
		replica, err := c.GetReplica(id)
		if err != nil || replica.Status() == ReplicaQuarantined {
			continue
		}
		if replica.LogLen() <= c.cfg.CompactionThreshold {
			continue
		}
		report, err := replica.Compact()
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// CatchUpReplica catches up a lagging replica using the leader's Merkle snapshot and tail entries.
func (c *ReplicationCluster) CatchUpReplica(nodeID string) error {
	c.mu.RLock()
	leader := c.replicas[c.leaderID]
	target, ok := c.replicas[nodeID]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errReplicaNotFound, nodeID)
	}
	snap, err := leader.ExportSnapshot()
	if err != nil {
		return err
	}
	if err := target.InstallSnapshot(snap); err != nil {
		return err
	}
	for _, entry := range leader.TailEntriesAfter(snap.Sequence) {
		if _, err := target.Apply(entry); err != nil {
			return err
		}
	}
	return nil
}

// VerifyConsistency checks all non-quarantined followers against the leader and quarantines diverging nodes.
func (c *ReplicationCluster) VerifyConsistency() []DivergenceReport {
	c.mu.Lock()
	defer c.mu.Unlock()

	leader := c.replicas[c.leaderID]
	leaderSeq := leader.AppliedSequence()
	leaderRoot := leader.MerkleRoot()

	reports := make([]DivergenceReport, 0)
	for id, replica := range c.replicas {
		if id == c.leaderID || replica.Status() == ReplicaQuarantined {
			continue
		}
		if replica.AppliedSequence() != leaderSeq || replica.MerkleRoot() == leaderRoot {
			continue
		}
		report := DivergenceReport{
			ReplicaID:     id,
			Sequence:      leaderSeq,
			ExpectedRoot:  leaderRoot,
			ObservedRoot:  replica.MerkleRoot(),
			Reason:        "merkle state root mismatch at identical sequence",
			QuarantinedAt: time.Now().UTC(),
		}
		replica.Quarantine()
		c.quarantined[id] = report
		reports = append(reports, report)
	}
	return reports
}

// QuarantinedReports returns all recorded divergence reports keyed by replica ID.
func (c *ReplicationCluster) QuarantinedReports() map[string]DivergenceReport {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]DivergenceReport, len(c.quarantined))
	for k, v := range c.quarantined {
		out[k] = v
	}
	return out
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
