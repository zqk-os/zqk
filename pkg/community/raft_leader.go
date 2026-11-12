package community

import (
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errRaftBlankCandidateID = "candidate ID cannot be blank"
	errRaftBlankClusterID   = "cluster ID cannot be blank"
	errRaftBlankPeerID      = "peer ID cannot be blank"
)

var (
	// ErrBlankCandidateID indicates that the candidate ID is empty or whitespace.
	ErrBlankCandidateID = errors.New(errRaftBlankCandidateID)

	// ErrBlankRaftClusterID indicates that the cluster ID is empty or whitespace.
	ErrBlankRaftClusterID = errors.New(errRaftBlankClusterID)

	// ErrZeroTerm indicates that term counter must be strictly positive.
	ErrZeroTerm = errors.New("raft term must be greater than zero")

	// ErrInvalidRaftClusterSize indicates that the cluster size is less than 1.
	ErrInvalidRaftClusterSize = errors.New("cluster size must be at least 1")

	// ErrInvalidHeartbeatLease indicates an unexpired or invalid lease operation.
	ErrInvalidHeartbeatLease = errors.New("invalid heartbeat lease renewal")

	// ErrHigherTermObserved indicates immediate step down on higher term discovery.
	ErrHigherTermObserved = errors.New("higher term observed: step-down triggered")
)

// RaftRole defines the node state in the consensus cycle.
type RaftRole string

const (
	RoleFollower  RaftRole = "follower"
	RoleCandidate RaftRole = "candidate"
	RoleLeader    RaftRole = "leader"
)

// RaftVoteRequest encapsulates a candidate's ballot petition.
type RaftVoteRequest struct {
	Term        uint64
	CandidateID string
	ClusterID   string
}

// RaftVoteResponse represents a peer's ballot outcome.
type RaftVoteResponse struct {
	Term        uint64
	VoteGranted bool
	VoterID     string
	Reason      string
}

// RaftHeartbeatLease represents the leader's active fencing lease.
type RaftHeartbeatLease struct {
	Term         uint64
	LeaderID     string
	ClusterID    string
	FencingToken uint64
	ExpiresAt    time.Time
}

// RaftLeaderConfig configures the RaftLeaderEngine.
type RaftLeaderConfig struct {
	NodeID        string
	ClusterID     string
	ClusterSize   int
	LeaseDuration time.Duration
}

// RaftLeaderEngine manages leader election, quorum validation, and lease fencing.
type RaftLeaderEngine struct {
	mu            sync.RWMutex
	nodeID        string
	clusterID     string
	clusterSize   int
	leaseDuration time.Duration
	currentTerm   uint64
	role          RaftRole
	votedFor      string
	fencingToken  uint64
	activeLease   *RaftHeartbeatLease
}

// NewRaftLeaderEngine initializes a validated Raft consensus engine.
func NewRaftLeaderEngine(cfg RaftLeaderConfig) (*RaftLeaderEngine, error) {
	if err := strutil.ErrorOnBlank(cfg.NodeID, errRaftBlankCandidateID); err != nil {
		return nil, ErrBlankCandidateID
	}
	if err := strutil.ErrorOnBlank(cfg.ClusterID, errRaftBlankClusterID); err != nil {
		return nil, ErrBlankRaftClusterID
	}
	if cfg.ClusterSize < 1 {
		return nil, ErrInvalidRaftClusterSize
	}
	leaseDur := cfg.LeaseDuration
	if leaseDur <= 0 {
		leaseDur = 3 * time.Second
	}
	return &RaftLeaderEngine{
		nodeID:        cfg.NodeID,
		clusterID:     cfg.ClusterID,
		clusterSize:   cfg.ClusterSize,
		leaseDuration: leaseDur,
		currentTerm:   1,
		role:          RoleFollower,
	}, nil
}

// MajorityQuorum computes the majority count required for election consensus.
func (e *RaftLeaderEngine) MajorityQuorum() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return (e.clusterSize / 2) + 1
}

// State returns current term, role, and node ID.
func (e *RaftLeaderEngine) State() (uint64, RaftRole, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentTerm, e.role, e.nodeID
}

// StartElection transitions the follower to candidate and requests a new term.
func (e *RaftLeaderEngine) StartElection() (RaftVoteRequest, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.currentTerm++
	e.role = RoleCandidate
	e.votedFor = e.nodeID

	return RaftVoteRequest{
		Term:        e.currentTerm,
		CandidateID: e.nodeID,
		ClusterID:   e.clusterID,
	}, nil
}

// HandleVoteRequest processes an incoming vote petition from a peer candidate.
func (e *RaftLeaderEngine) HandleVoteRequest(req RaftVoteRequest) (RaftVoteResponse, error) {
	if err := strutil.ErrorOnBlank(req.CandidateID, errRaftBlankCandidateID); err != nil {
		return RaftVoteResponse{}, ErrBlankCandidateID
	}
	if err := strutil.ErrorOnBlank(req.ClusterID, errRaftBlankClusterID); err != nil {
		return RaftVoteResponse{}, ErrBlankRaftClusterID
	}
	if req.Term == 0 {
		return RaftVoteResponse{}, ErrZeroTerm
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if req.Term > e.currentTerm {
		e.currentTerm = req.Term
		e.role = RoleFollower
		e.votedFor = ""
	}

	if req.Term < e.currentTerm {
		return RaftVoteResponse{
			Term:        e.currentTerm,
			VoteGranted: false,
			VoterID:     e.nodeID,
			Reason:      "term is behind current term",
		}, nil
	}

	if e.votedFor == "" || e.votedFor == req.CandidateID {
		e.votedFor = req.CandidateID
		return RaftVoteResponse{
			Term:        e.currentTerm,
			VoteGranted: true,
			VoterID:     e.nodeID,
			Reason:      "ballot accepted",
		}, nil
	}

	return RaftVoteResponse{
		Term:        e.currentTerm,
		VoteGranted: false,
		VoterID:     e.nodeID,
		Reason:      "already voted for another candidate in this term",
	}, nil
}

// ClaimLeadership evaluates collected votes and secures the leader lease if quorum is satisfied.
func (e *RaftLeaderEngine) ClaimLeadership(votes []RaftVoteResponse, now time.Time) (*RaftHeartbeatLease, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	quorum := (e.clusterSize / 2) + 1
	grantedCount := 0

	for _, v := range votes {
		if v.Term > e.currentTerm {
			e.currentTerm = v.Term
			e.role = RoleFollower
			e.votedFor = ""
			e.activeLease = nil
			return nil, ErrHigherTermObserved
		}
		if v.VoteGranted && v.Term == e.currentTerm {
			grantedCount++
		}
	}

	// Candidate's own vote
	if e.votedFor == e.nodeID {
		grantedCount++
	}

	if grantedCount < quorum {
		return nil, errors.New("quorum not reached: insufficient votes")
	}

	e.role = RoleLeader
	e.fencingToken++
	lease := &RaftHeartbeatLease{
		Term:         e.currentTerm,
		LeaderID:     e.nodeID,
		ClusterID:    e.clusterID,
		FencingToken: e.fencingToken,
		ExpiresAt:    now.Add(e.leaseDuration),
	}
	e.activeLease = lease
	return lease, nil
}

// RenewHeartbeat extends the active leader lease upon receiving a successful heartbeat round.
func (e *RaftLeaderEngine) RenewHeartbeat(now time.Time, healthyPeerCount int) (*RaftHeartbeatLease, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.role != RoleLeader || e.activeLease == nil {
		return nil, ErrInvalidHeartbeatLease
	}

	// Total active nodes = healthy peers + leader itself
	quorum := (e.clusterSize / 2) + 1
	if healthyPeerCount+1 < quorum {
		// Quorum lost during partition, step down
		e.role = RoleFollower
		e.activeLease = nil
		return nil, errors.New("heartbeat quorum lost: stepping down")
	}

	e.fencingToken++
	e.activeLease.FencingToken = e.fencingToken
	e.activeLease.ExpiresAt = now.Add(e.leaseDuration)
	return e.activeLease, nil
}

// StepDown demotes the node to follower upon higher term observation or operator intervention.
func (e *RaftLeaderEngine) StepDown(observedTerm uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if observedTerm > e.currentTerm {
		e.currentTerm = observedTerm
	}
	e.role = RoleFollower
	e.votedFor = ""
	e.activeLease = nil
}

// ActiveLease retrieves the current active leader lease if unexpired.
func (e *RaftLeaderEngine) ActiveLease(now time.Time) (*RaftHeartbeatLease, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.role != RoleLeader || e.activeLease == nil {
		return nil, false
	}
	if now.After(e.activeLease.ExpiresAt) {
		return nil, false
	}
	return e.activeLease, true
}
