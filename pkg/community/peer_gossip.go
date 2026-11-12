package community

import (
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errGossipBlankPeerIDMsg    = "peer ID cannot be blank"
	errGossipBlankClusterIDMsg = "cluster ID cannot be blank"
	errGossipBlankLocalPeerID  = "local peer ID cannot be blank"
)

var (
	// ErrBlankPeerID indicates that the peer ID is empty or whitespace.
	ErrBlankPeerID = errors.New(errGossipBlankPeerIDMsg)

	// ErrBlankClusterID indicates that the cluster ID is empty or whitespace.
	ErrBlankClusterID = errors.New(errGossipBlankClusterIDMsg)

	// ErrPeerNotFound indicates that the queried peer is not registered in the gossip membership.
	ErrPeerNotFound = errors.New("peer not registered in gossip membership")

	// ErrPeerAlreadyRegistered indicates that a peer with the given ID already exists.
	ErrPeerAlreadyRegistered = errors.New("peer already registered in gossip membership")

	// ErrClusterMismatch indicates that the peer belongs to a foreign cluster namespace.
	ErrClusterMismatch = errors.New("cluster ID mismatch")

	// ErrStaleEpoch indicates that the update has an epoch lower than or equal to current peer state.
	ErrStaleEpoch = errors.New("incoming epoch is stale")
)

// PeerNodeStatus represents the health and connectivity state of a gossip member.
type PeerNodeStatus string

const (
	PeerStatusActive  PeerNodeStatus = "active"
	PeerStatusSuspect PeerNodeStatus = "suspect"
	PeerStatusDead    PeerNodeStatus = "dead"
)

// PeerDescriptor represents membership metadata and sync state for a peer node.
type PeerDescriptor struct {
	PeerID       string
	ClusterID    string
	Address      string
	Epoch        uint64
	LastObserved time.Time
	Status       PeerNodeStatus
}

// Validate verifies structural integrity of the peer descriptor.
func (p *PeerDescriptor) Validate() error {
	if err := strutil.ErrorOnBlank(p.PeerID, errGossipBlankPeerIDMsg); err != nil {
		return ErrBlankPeerID
	}
	if err := strutil.ErrorOnBlank(p.ClusterID, errGossipBlankClusterIDMsg); err != nil {
		return ErrBlankClusterID
	}
	if p.Status == "" {
		p.Status = PeerStatusActive
	}
	if p.LastObserved.IsZero() {
		p.LastObserved = time.Now()
	}
	return nil
}

// GossipDigest summarizes the vector of peer epochs known to a node for anti-entropy exchange.
type GossipDigest struct {
	ClusterID string
	Epochs    map[string]uint64
}

// PeerGossipConfig parameterizes the gossip synchronization engine.
type PeerGossipConfig struct {
	LocalPeerID    string
	ClusterID      string
	SuspectTimeout time.Duration
}

// PeerGossipSyncEngine manages multi-cluster peer membership and anti-entropy reconciliation.
type PeerGossipSyncEngine struct {
	mu    sync.RWMutex
	cfg   PeerGossipConfig
	peers map[string]PeerDescriptor
}

// NewPeerGossipSyncEngine instantiates a new gossip engine.
func NewPeerGossipSyncEngine(cfg PeerGossipConfig) (*PeerGossipSyncEngine, error) {
	if err := strutil.ErrorOnBlank(cfg.LocalPeerID, errGossipBlankLocalPeerID); err != nil {
		return nil, ErrBlankPeerID
	}
	if err := strutil.ErrorOnBlank(cfg.ClusterID, errGossipBlankClusterIDMsg); err != nil {
		return nil, ErrBlankClusterID
	}
	if cfg.SuspectTimeout <= 0 {
		cfg.SuspectTimeout = 15 * time.Second
	}

	engine := &PeerGossipSyncEngine{
		cfg:   cfg,
		peers: make(map[string]PeerDescriptor),
	}

	// Register local peer as active epoch 1
	engine.peers[cfg.LocalPeerID] = PeerDescriptor{
		PeerID:       cfg.LocalPeerID,
		ClusterID:    cfg.ClusterID,
		Address:      "local",
		Epoch:        1,
		LastObserved: time.Now(),
		Status:       PeerStatusActive,
	}

	return engine, nil
}

func (e *PeerGossipSyncEngine) mutatePeer(desc PeerDescriptor, fn func(existing PeerDescriptor, exists bool) error) error {
	if err := desc.Validate(); err != nil {
		return err
	}
	if desc.ClusterID != e.cfg.ClusterID {
		return ErrClusterMismatch
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	existing, exists := e.peers[desc.PeerID]
	return fn(existing, exists)
}

// RegisterPeer registers an external peer descriptor.
func (e *PeerGossipSyncEngine) RegisterPeer(desc PeerDescriptor) error {
	return e.mutatePeer(desc, func(_ PeerDescriptor, exists bool) error {
		if exists {
			return ErrPeerAlreadyRegistered
		}
		e.peers[desc.PeerID] = desc
		return nil
	})
}

// GetPeer retrieves the current descriptor for the given peer.
func (e *PeerGossipSyncEngine) GetPeer(peerID string) (PeerDescriptor, error) {
	if err := strutil.ErrorOnBlank(peerID, errGossipBlankPeerIDMsg); err != nil {
		return PeerDescriptor{}, ErrBlankPeerID
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	desc, exists := e.peers[peerID]
	if !exists {
		return PeerDescriptor{}, ErrPeerNotFound
	}
	return desc, nil
}

// ListActivePeers returns all peers currently in active status.
func (e *PeerGossipSyncEngine) ListActivePeers() []PeerDescriptor {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var active []PeerDescriptor
	for _, p := range e.peers {
		if p.Status == PeerStatusActive {
			active = append(active, p)
		}
	}
	return active
}

// GenerateDigest generates a compact anti-entropy digest for gossip exchange.
func (e *PeerGossipSyncEngine) GenerateDigest() GossipDigest {
	e.mu.RLock()
	defer e.mu.RUnlock()

	epochs := make(map[string]uint64, len(e.peers))
	for id, p := range e.peers {
		epochs[id] = p.Epoch
	}

	return GossipDigest{
		ClusterID: e.cfg.ClusterID,
		Epochs:    epochs,
	}
}

// ReconcileDigest inspects a remote digest and identifies peer IDs where local state is stale.
func (e *PeerGossipSyncEngine) ReconcileDigest(remote GossipDigest) ([]string, error) {
	if err := strutil.ErrorOnBlank(remote.ClusterID, errGossipBlankClusterIDMsg); err != nil {
		return nil, ErrBlankClusterID
	}
	if remote.ClusterID != e.cfg.ClusterID {
		return nil, ErrClusterMismatch
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var staleLocals []string
	for remotePeerID, remoteEpoch := range remote.Epochs {
		localDesc, exists := e.peers[remotePeerID]
		if !exists || localDesc.Epoch < remoteEpoch {
			staleLocals = append(staleLocals, remotePeerID)
		}
	}
	return staleLocals, nil
}

// ApplyPeerUpdate updates or inserts a peer with an advanced epoch.
func (e *PeerGossipSyncEngine) ApplyPeerUpdate(desc PeerDescriptor) error {
	return e.mutatePeer(desc, func(existing PeerDescriptor, exists bool) error {
		if exists && desc.Epoch <= existing.Epoch {
			return ErrStaleEpoch
		}

		desc.LastObserved = time.Now()
		e.peers[desc.PeerID] = desc
		return nil
	})
}

// MarkSuspect flags a peer as suspect upon missing scheduled gossip rounds.
func (e *PeerGossipSyncEngine) MarkSuspect(peerID string) error {
	if err := strutil.ErrorOnBlank(peerID, errGossipBlankPeerIDMsg); err != nil {
		return ErrBlankPeerID
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	desc, exists := e.peers[peerID]
	if !exists {
		return ErrPeerNotFound
	}

	desc.Status = PeerStatusSuspect
	e.peers[peerID] = desc
	return nil
}

// EvictDeadPeers marks peers dead whose last observation exceeds suspect timeout.
func (e *PeerGossipSyncEngine) EvictDeadPeers(now time.Time) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	deadCount := 0
	for id, p := range e.peers {
		if id == e.cfg.LocalPeerID {
			continue
		}
		if now.Sub(p.LastObserved) > e.cfg.SuspectTimeout {
			p.Status = PeerStatusDead
			e.peers[id] = p
			deadCount++
		}
	}
	return deadCount
}

// PruneDeadPeers deletes suspect or dead peers from the local peer registry.
func (e *PeerGossipSyncEngine) PruneDeadPeers(now time.Time) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	prunedCount := 0
	for id, p := range e.peers {
		if id == e.cfg.LocalPeerID {
			continue
		}
		if p.Status == PeerStatusDead || p.Status == PeerStatusSuspect {
			delete(e.peers, id)
			prunedCount++
		}
	}
	return prunedCount
}
