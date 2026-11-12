package community

import (
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errPartitionBlankNodeID    = "node ID cannot be blank"
	errPartitionBlankClusterID = "cluster ID cannot be blank"
	errPartitionBlankPeerID    = "peer ID cannot be blank"
)

var (
	// ErrBlankPartitionNodeID indicates that the node ID is empty or whitespace.
	ErrBlankPartitionNodeID = errors.New(errPartitionBlankNodeID)

	// ErrBlankPartitionClusterID indicates that the cluster ID is empty or whitespace.
	ErrBlankPartitionClusterID = errors.New(errPartitionBlankClusterID)

	// ErrInvalidClusterSize indicates that the total cluster node count is non-positive.
	ErrInvalidClusterSize = errors.New("total cluster nodes must be at least 1")

	// ErrInvalidLeaseDuration indicates that lease duration is non-positive.
	ErrInvalidLeaseDuration = errors.New("lease duration must be positive")

	// ErrLeaseExpired indicates that the node lease has expired due to missed heartbeats.
	ErrLeaseExpired = errors.New("node lease expired: minority partition write fence active")

	// ErrQuorumNotReached indicates that active healthy nodes do not constitute a strict majority.
	ErrQuorumNotReached = errors.New("majority quorum not satisfied")
)

// PartitionStatus represents the quorum and connectivity health of a partition observer.
type PartitionStatus string

const (
	PartitionStatusQuorumHealthy  PartitionStatus = "quorum_healthy"
	PartitionStatusMinorityFenced PartitionStatus = "minority_fenced"
)

// LeaseFenceRecord represents an active lease protecting against split-brain mutations.
type LeaseFenceRecord struct {
	NodeID     string
	ClusterID  string
	LeaseEpoch uint64
	ExpiresAt  time.Time
}

// SwarmPartitionConfig parameterizes the SwarmPartitionDetector.
type SwarmPartitionConfig struct {
	NodeID            string
	ClusterID         string
	TotalClusterNodes int
	LeaseDuration     time.Duration
}

// SwarmPartitionDetector tracks live peer heartbeats, calculates strict majority quorums,
// and manages lease fences to prevent split-brain execution across network partitions.
type SwarmPartitionDetector struct {
	mu            sync.RWMutex
	cfg           SwarmPartitionConfig
	majorityCount int
	heartbeats    map[string]time.Time
	lease         LeaseFenceRecord
}

// NewSwarmPartitionDetector creates and initializes a new detector.
func NewSwarmPartitionDetector(cfg SwarmPartitionConfig) (*SwarmPartitionDetector, error) {
	if err := strutil.ErrorOnBlank(cfg.NodeID, errPartitionBlankNodeID); err != nil {
		return nil, ErrBlankPartitionNodeID
	}
	if err := strutil.ErrorOnBlank(cfg.ClusterID, errPartitionBlankClusterID); err != nil {
		return nil, ErrBlankPartitionClusterID
	}
	if cfg.TotalClusterNodes <= 0 {
		return nil, ErrInvalidClusterSize
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 5 * time.Second
	}

	majority := (cfg.TotalClusterNodes / 2) + 1

	return &SwarmPartitionDetector{
		cfg:           cfg,
		majorityCount: majority,
		heartbeats:    make(map[string]time.Time),
	}, nil
}

// RecordHeartbeat records an active heartbeat from a peer node in the cluster.
func (d *SwarmPartitionDetector) RecordHeartbeat(peerID string, timestamp time.Time) error {
	if err := strutil.ErrorOnBlank(peerID, errPartitionBlankPeerID); err != nil {
		return ErrBlankPartitionNodeID
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.heartbeats[peerID] = timestamp
	return nil
}

// EvaluateQuorum calculates active peer count within the lease window and determines partition status.
func (d *SwarmPartitionDetector) EvaluateQuorum(now time.Time) (PartitionStatus, int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Always count self as 1 active node
	activeCount := 1
	cutoff := now.Add(-d.cfg.LeaseDuration)

	for peerID, ts := range d.heartbeats {
		if peerID == d.cfg.NodeID {
			continue
		}
		if ts.After(cutoff) {
			activeCount++
		}
	}

	if activeCount >= d.majorityCount {
		return PartitionStatusQuorumHealthy, activeCount
	}
	return PartitionStatusMinorityFenced, activeCount
}

// AcquireOrRenewLease attempts to renew the fencing lease if majority quorum is intact.
func (d *SwarmPartitionDetector) AcquireOrRenewLease(now time.Time) (*LeaseFenceRecord, error) {
	status, activeCount := d.EvaluateQuorum(now)
	if status != PartitionStatusQuorumHealthy || activeCount < d.majorityCount {
		return nil, ErrQuorumNotReached
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.lease.LeaseEpoch++
	d.lease.NodeID = d.cfg.NodeID
	d.lease.ClusterID = d.cfg.ClusterID
	d.lease.ExpiresAt = now.Add(d.cfg.LeaseDuration)

	rec := d.lease
	return &rec, nil
}

// VerifyLease checks whether the local node holds a valid unexpired fencing lease.
func (d *SwarmPartitionDetector) VerifyLease(now time.Time) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.lease.ExpiresAt.IsZero() || now.After(d.lease.ExpiresAt) {
		return ErrLeaseExpired
	}
	return nil
}

// IsWritePermitted returns true only if majority quorum is verified and lease is unexpired.
func (d *SwarmPartitionDetector) IsWritePermitted(now time.Time) bool {
	if err := d.VerifyLease(now); err != nil {
		return false
	}
	status, activeCount := d.EvaluateQuorum(now)
	return status == PartitionStatusQuorumHealthy && activeCount >= d.majorityCount
}
