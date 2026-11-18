// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestVerificationMesh_StaticFloorInvariant(t *testing.T) {
	mesh, err := NewVerificationMesh(100, 67)
	if err != nil {
		t.Fatalf("failed to initialize mesh: %v", err)
	}

	// 1. Stake and quorum percentage boundary checks
	if _, err := NewVerificationMesh(0, 67); err == nil {
		t.Fatal("expected error for zero minimum stake")
	}
	if _, err := NewVerificationMesh(100, 0); err == nil {
		t.Fatal("expected error for 0% quorum")
	}
	if _, err := NewVerificationMesh(100, 101); err == nil {
		t.Fatal("expected error for >100% quorum")
	}

	// 2. Peer registration with valid stake
	peer := PeerNode{
		ID:        "peer-node-1",
		PublicKey: "02abc12345def6789",
		Stake:     500,
	}
	if err := mesh.RegisterPeer(peer); err != nil {
		t.Fatalf("failed to register valid peer: %v", err)
	}

	// 3. Reject zero and below-minimum anti-sybil stake quota
	if err := mesh.RegisterPeer(PeerNode{ID: "zero-stake-node", PublicKey: "02zero", Stake: 0}); err == nil {
		t.Fatal("expected rejection of peer with zero stake")
	}
	sybilPeer := PeerNode{
		ID:        "sybil-node-1",
		PublicKey: "02sybil12345",
		Stake:     50, // minStake is 100
	}
	if err := mesh.RegisterPeer(sybilPeer); err == nil {
		t.Fatal("expected rejection of sybil peer with insufficient stake")
	}
}

func TestVerificationMesh_OperationalDynamicProof(t *testing.T) {
	mesh, err := NewVerificationMesh(100, 66)
	if err != nil {
		t.Fatalf("failed to initialize mesh: %v", err)
	}

	// Register 3 peers with 1000 stake each
	for i := 1; i <= 3; i++ {
		p := PeerNode{
			ID:        fmt.Sprintf("validator-%d", i),
			PublicKey: fmt.Sprintf("pubkey-%d", i),
			Stake:     1000,
		}
		if err := mesh.RegisterPeer(p); err != nil {
			t.Fatalf("failed to register validator %d: %v", i, err)
		}
	}

	claim := MeshClaim{
		ID:        "claim-ast-001",
		Author:    "validator-1",
		Payload:   []byte("evidence: AST floor invariant verified and certified"),
		Timestamp: time.Now().UTC(),
	}

	digest, err := mesh.SubmitClaim(claim)
	if err != nil {
		t.Fatalf("failed to submit claim: %v", err)
	}
	if digest == "" {
		t.Fatal("expected non-empty digest")
	}

	// Cast votes from validator-1 and validator-2 (2000 of 3000 stake = 66.7% -> >= 67% needed)
	for i := 1; i <= 2; i++ {
		vote := PeerVote{
			PeerID:    fmt.Sprintf("validator-%d", i),
			Signature: fmt.Sprintf("sig-%d", i),
			Approve:   true,
		}
		if err := mesh.CastVote("claim-ast-001", vote); err != nil {
			t.Fatalf("failed to cast vote for validator %d: %v", i, err)
		}
	}

	// Concurrent evaluation and vote casting
	var wg sync.WaitGroup
	for w := 0; w < 5; w++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_verification_mesh_worker", "concurrent mesh consensus evaluation").StartSimple(func() {
			defer wg.Done()
			res, err := mesh.EvaluateConsensus("claim-ast-001")
			if err != nil {
				t.Errorf("concurrent evaluate error: %v", err)
				return
			}
			if !res.QuorumMet {
				t.Errorf("expected quorum met, got false")
			}
		})
	}
	wg.Wait()
}

func TestVerificationMesh_NegativeAdversarialBoundary(t *testing.T) {
	mesh, err := NewVerificationMesh(100, 67)
	if err != nil {
		t.Fatalf("failed to initialize mesh: %v", err)
	}

	// 1. Unregistered author cannot submit claims
	claim := MeshClaim{
		ID:      "claim-bad-author",
		Author:  "stranger-node",
		Payload: []byte("unauthorized data"),
	}
	if _, err := mesh.SubmitClaim(claim); err == nil {
		t.Fatal("expected rejection of claim from unregistered author")
	}

	// 2. Register valid author and submit claim
	validAuthor := PeerNode{
		ID:        "author-1",
		PublicKey: "pub-author-1",
		Stake:     1000,
	}
	if err := mesh.RegisterPeer(validAuthor); err != nil {
		t.Fatal(err)
	}
	claim.Author = "author-1"
	if _, err := mesh.SubmitClaim(claim); err != nil {
		t.Fatal(err)
	}

	// 3. Unregistered voter cannot vote
	badVote := PeerVote{
		PeerID:    "unknown-voter",
		Signature: "sig-fake",
		Approve:   true,
	}
	if err := mesh.CastVote(claim.ID, badVote); err == nil {
		t.Fatal("expected rejection of vote from unregistered peer")
	}

	// 4. Voting on non-existent claim fails
	if err := mesh.CastVote("non-existent-claim", PeerVote{PeerID: "author-1", Signature: "sig"}); err == nil {
		t.Fatal("expected rejection of vote on non-existent claim")
	}

	// 5. Blank ID / Author / Signature validations fail
	if err := mesh.RegisterPeer(PeerNode{ID: "", PublicKey: "k", Stake: 500}); err == nil {
		t.Fatal("expected error on blank peer ID")
	}
	if err := mesh.RegisterPeer(PeerNode{ID: "node-1", PublicKey: "", Stake: 500}); err == nil {
		t.Fatal("expected error on blank public key")
	}
	if _, err := mesh.SubmitClaim(MeshClaim{ID: "", Author: "author-1", Payload: []byte("d")}); err == nil {
		t.Fatal("expected error on blank claim ID")
	}
	if _, err := mesh.SubmitClaim(MeshClaim{ID: "c1", Author: "author-1", Payload: nil}); err == nil {
		t.Fatal("expected error on empty payload")
	}
	if err := mesh.CastVote("c1", PeerVote{PeerID: "", Signature: "sig"}); err == nil {
		t.Fatal("expected error on blank peer ID in vote")
	}
	if err := mesh.CastVote("c1", PeerVote{PeerID: "author-1", Signature: ""}); err == nil {
		t.Fatal("expected error on blank signature in vote")
	}

	// 6. Insufficient threshold rejection: sub-threshold quorum and sub-threshold approval
	if _, err := mesh.EvaluateConsensus("non-existent-claim"); err == nil {
		t.Fatal("expected error when evaluating consensus for non-existent claim")
	}
	for i := 2; i <= 3; i++ {
		if err := mesh.RegisterPeer(PeerNode{
			ID:        fmt.Sprintf("author-%d", i),
			PublicKey: fmt.Sprintf("pub-author-%d", i),
			Stake:     1000,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Only author-1 votes (1000 / 3000 = 33.3% < 67% quorum) -> QuorumMet=false, Approved=false
	if err := mesh.CastVote(claim.ID, PeerVote{PeerID: "author-1", Signature: "sig-1", Approve: true}); err != nil {
		t.Fatal(err)
	}
	subQuorumRes, err := mesh.EvaluateConsensus(claim.ID)
	if err != nil {
		t.Fatalf("unexpected error evaluating sub-quorum claim: %v", err)
	}
	if subQuorumRes.QuorumMet || subQuorumRes.Approved {
		t.Fatalf("expected QuorumMet=false and Approved=false for sub-threshold quorum, got QuorumMet=%v Approved=%v", subQuorumRes.QuorumMet, subQuorumRes.Approved)
	}

	// author-2 rejects (2000 / 3000 = 66.7% < 67% quorum, and 1000/2000 = 50% approval < 67%)
	// Add author-3 rejecting so quorum is met (3000/3000 = 100%), but approval is 1000/3000 = 33.3% < 67%
	if err := mesh.CastVote(claim.ID, PeerVote{PeerID: "author-2", Signature: "sig-2", Approve: false}); err != nil {
		t.Fatal(err)
	}
	if err := mesh.CastVote(claim.ID, PeerVote{PeerID: "author-3", Signature: "sig-3", Approve: false}); err != nil {
		t.Fatal(err)
	}
	subApprovalRes, err := mesh.EvaluateConsensus(claim.ID)
	if err != nil {
		t.Fatalf("unexpected error evaluating sub-approval claim: %v", err)
	}
	if !subApprovalRes.QuorumMet {
		t.Fatalf("expected QuorumMet=true with 100%% participation, got false")
	}
	if subApprovalRes.Approved {
		t.Fatalf("expected Approved=false for sub-threshold approval (33.3%% < 67%%), got true")
	}

	// 7. Malicious node isolation and insufficient-stake re-registration rejection
	if err := mesh.IsolatePeer(""); err == nil {
		t.Fatal("expected error when isolating blank peer ID")
	}
	if err := mesh.IsolatePeer("unknown-peer"); err == nil {
		t.Fatal("expected error when isolating unknown peer")
	}
	if err := mesh.IsolatePeer("author-3"); err != nil {
		t.Fatalf("failed to isolate peer author-3: %v", err)
	}
	if _, err := mesh.SubmitClaim(MeshClaim{ID: "claim-isolated", Author: "author-3", Payload: []byte("malicious")}); err == nil {
		t.Fatal("expected rejection of claim from isolated peer")
	}
	if err := mesh.CastVote(claim.ID, PeerVote{PeerID: "author-3", Signature: "sig-3-retry", Approve: true}); err == nil {
		t.Fatal("expected rejection of vote from isolated peer")
	}
	isolatedRes, err := mesh.EvaluateConsensus(claim.ID)
	if err != nil {
		t.Fatalf("unexpected error evaluating consensus after isolation: %v", err)
	}
	if isolatedRes.TotalStakeVotes != 2000 {
		t.Fatalf("expected TotalStakeVotes=2000 after isolating author-3, got %d", isolatedRes.TotalStakeVotes)
	}
	if err := mesh.RegisterPeer(PeerNode{ID: "author-3", PublicKey: "pub-author-3", Stake: 50}); err == nil {
		t.Fatal("expected rejection when re-registering isolated peer with insufficient stake")
	}
}
