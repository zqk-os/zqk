// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// PeerNode represents a registered peer in the verification mesh.
type PeerNode struct {
	ID        string    `json:"id"`
	PublicKey string    `json:"public_key"`
	Stake     uint64    `json:"stake"`
	Active    bool      `json:"active"`
	JoinedAt  time.Time `json:"joined_at"`
}

// MeshClaim represents a unit of work or evidence submitted for multi-party verification.
type MeshClaim struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Payload   []byte    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// PeerVote represents an endorsement vote signed by a peer node.
type PeerVote struct {
	PeerID    string    `json:"peer_id"`
	Signature string    `json:"signature"`
	Approve   bool      `json:"approve"`
	Timestamp time.Time `json:"timestamp"`
}

// VerificationResult summarizes the outcome of mesh consensus evaluation.
type VerificationResult struct {
	ClaimID         string    `json:"claim_id"`
	Digest          string    `json:"digest"`
	TotalStakeVotes uint64    `json:"total_stake_votes"`
	ApprovalStake   uint64    `json:"approval_stake"`
	QuorumMet       bool      `json:"quorum_met"`
	Approved        bool      `json:"approved"`
	Voters          []string  `json:"voters"`
	EvaluatedAt     time.Time `json:"evaluated_at"`
}

// VerificationMesh provides decentralized peer-to-peer verification and anti-sybil staking quotas.
type VerificationMesh struct {
	mu         sync.RWMutex
	minStake   uint64
	quorumPct  uint32 // e.g. 67 for 67%
	peers      map[string]*PeerNode
	claimVotes map[string]map[string]*PeerVote
	claims     map[string]*MeshClaim
}

// NewVerificationMesh constructs an active peer-to-peer verification mesh.
func NewVerificationMesh(minStake uint64, quorumPct uint32) (*VerificationMesh, error) {
	if quorumPct == 0 || quorumPct > 100 {
		return nil, fmt.Errorf("quorum percentage must be between 1 and 100, got %d", quorumPct)
	}
	return &VerificationMesh{
		minStake:   minStake,
		quorumPct:  quorumPct,
		peers:      make(map[string]*PeerNode),
		claimVotes: make(map[string]map[string]*PeerVote),
		claims:     make(map[string]*MeshClaim),
	}, nil
}

// RegisterPeer registers or updates a peer node, enforcing the minimum anti-sybil stake quota.
func (m *VerificationMesh) RegisterPeer(node PeerNode) error {
	if err := strutil.ErrorOnBlank(node.ID, "peer ID cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(node.PublicKey, "peer public key cannot be blank"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if node.Stake < m.minStake {
		return fmt.Errorf("peer stake %d below minimum anti-sybil quota %d", node.Stake, m.minStake)
	}

	if node.JoinedAt.IsZero() {
		node.JoinedAt = time.Now().UTC()
	}
	node.Active = true

	m.peers[node.ID] = &node
	return nil
}

// SubmitClaim enters an attestation claim into the mesh for peer endorsement.
func (m *VerificationMesh) SubmitClaim(claim MeshClaim) (string, error) {
	if err := strutil.ErrorOnBlank(claim.ID, "claim ID cannot be blank"); err != nil {
		return "", err
	}
	if err := strutil.ErrorOnBlank(claim.Author, "claim author cannot be blank"); err != nil {
		return "", err
	}
	if len(claim.Payload) == 0 {
		return "", fmt.Errorf("claim payload cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	authorNode, exists := m.peers[claim.Author]
	if !exists || !authorNode.Active {
		return "", fmt.Errorf("author %q is not an active registered peer", claim.Author)
	}

	if claim.Timestamp.IsZero() {
		claim.Timestamp = time.Now().UTC()
	}

	m.claims[claim.ID] = &claim
	if _, ok := m.claimVotes[claim.ID]; !ok {
		m.claimVotes[claim.ID] = make(map[string]*PeerVote)
	}

	h := sha256.Sum256(claim.Payload)
	return hex.EncodeToString(h[:]), nil
}

// CastVote records an endorsement or rejection vote from a registered peer.
func (m *VerificationMesh) CastVote(claimID string, vote PeerVote) error {
	if err := strutil.ErrorOnBlank(claimID, "claim ID cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(vote.PeerID, "voter peer ID cannot be blank"); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(vote.Signature, "signature cannot be blank"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	peer, exists := m.peers[vote.PeerID]
	if !exists || !peer.Active {
		return fmt.Errorf("peer %q is not authorized to vote", vote.PeerID)
	}

	if _, exists := m.claims[claimID]; !exists {
		return fmt.Errorf("claim %q not found", claimID)
	}

	if vote.Timestamp.IsZero() {
		vote.Timestamp = time.Now().UTC()
	}

	m.claimVotes[claimID][vote.PeerID] = &vote
	return nil
}

// EvaluateConsensus audits all votes submitted for a claim and calculates stake-weighted consensus.
func (m *VerificationMesh) EvaluateConsensus(claimID string) (*VerificationResult, error) {
	if err := strutil.ErrorOnBlank(claimID, "claim ID cannot be blank"); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	claim, exists := m.claims[claimID]
	if !exists {
		return nil, fmt.Errorf("claim %q not found", claimID)
	}

	var totalNetworkStake uint64
	for _, p := range m.peers {
		if p.Active {
			totalNetworkStake += p.Stake
		}
	}

	if totalNetworkStake == 0 {
		return nil, fmt.Errorf("no active stake in verification mesh")
	}

	votes := m.claimVotes[claimID]
	var totalVotedStake uint64
	var approvalStake uint64
	voters := make([]string, 0, len(votes))

	for peerID, vote := range votes {
		peer, ok := m.peers[peerID]
		if !ok || !peer.Active {
			continue
		}
		totalVotedStake += peer.Stake
		if vote.Approve {
			approvalStake += peer.Stake
		}
		voters = append(voters, peerID)
	}

	h := sha256.Sum256(claim.Payload)
	digest := hex.EncodeToString(h[:])

	// Quorum check: total voted stake must be at least quorumPct of total network stake
	quorumStakeNeeded := (totalNetworkStake*uint64(m.quorumPct) + 99) / 100
	quorumMet := totalVotedStake >= quorumStakeNeeded

	// Approval check: approval stake must be >= quorumPct of total voted stake
	var approved bool
	if quorumMet && totalVotedStake > 0 {
		approvalNeeded := (totalVotedStake*uint64(m.quorumPct) + 99) / 100
		approved = approvalStake >= approvalNeeded
	}

	return &VerificationResult{
		ClaimID:         claimID,
		Digest:          digest,
		TotalStakeVotes: totalVotedStake,
		ApprovalStake:   approvalStake,
		QuorumMet:       quorumMet,
		Approved:        approved,
		Voters:          voters,
		EvaluatedAt:     time.Now().UTC(),
	}, nil
}
