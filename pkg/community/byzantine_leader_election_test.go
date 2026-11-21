// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testBFTClusterID    = "cluster-bft-alpha"
	testForeignCluster  = "cluster-bft-foreign"
	testValOne          = "validator-1"
	testValTwo          = "validator-2"
	testValThree        = "validator-3"
	testValFour         = "validator-4"
	testDefaultTimeout  = 5 * time.Second
	testErrUnexpected   = "unexpected error: %v"
	testErrExpectedGot  = "expected error %v, got %v"
	testForgedDigestStr = "forged-stall-digest"
)

func defaultTestValidators() []string {
	return []string{testValOne, testValTwo, testValThree, testValFour}
}

func fixedTestTime() time.Time {
	return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
}

func mustNewCoordinator(t *testing.T, validators []string, faultTol int) *ElectionCoordinator {
	t.Helper()
	coord, err := NewElectionCoordinator(ElectionConfig{
		ClusterID:        testBFTClusterID,
		ValidatorIDs:     validators,
		HeartbeatTimeout: testDefaultTimeout,
		FaultTolerance:   faultTol,
	})
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	return coord
}

func mustBuildClaim(t *testing.T, coord *ElectionCoordinator, valID string, targetView uint64) ViewChangeClaim {
	t.Helper()
	claim, err := coord.BuildViewChangeClaim(valID, targetView, fixedTestTime())
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	return claim
}

func defaultQuorumVoters() []string {
	return []string{testValOne, testValTwo, testValThree}
}

func submitClaims(t *testing.T, coord *ElectionCoordinator, voters []string, targetView uint64) *NewViewCertificate {
	t.Helper()
	var finalCert *NewViewCertificate
	for _, v := range voters {
		claim := mustBuildClaim(t, coord, v, targetView)
		cert, err := coord.SubmitViewChangeClaim(claim)
		if err != nil {
			t.Fatalf(testErrUnexpected, err)
		}
		if cert != nil {
			finalCert = cert
		}
	}
	return finalCert
}

func mustAdvanceToView(t *testing.T, coord *ElectionCoordinator, voters []string, targetView uint64) *NewViewCertificate {
	t.Helper()
	finalCert := submitClaims(t, coord, voters, targetView)
	if finalCert == nil {
		t.Fatalf("expected NewViewCertificate for view %d, got nil", targetView)
	}
	return finalCert
}

func assertBFTErrorIs(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf(testErrExpectedGot, want, err)
	}
}

func TestByzantineLeaderElection_StaticFloorInvariant(t *testing.T) {
	t.Run("rejects invalid election configs", func(t *testing.T) {
		verifyInvalidElectionConfigs(t)
	})
	t.Run("computes 3f+1 and 2f+1 BFT quorum sizing", func(t *testing.T) {
		verifyBFTQuorumSizing(t)
	})
	t.Run("stall proof digest is deterministic and non-empty", func(t *testing.T) {
		d1 := ComputeStallProofDigest(testBFTClusterID, 0, 1, testValOne)
		d2 := ComputeStallProofDigest(testBFTClusterID, 0, 1, testValOne)
		if d1 == "" || d1 != d2 {
			t.Fatalf("expected deterministic non-empty stall digest, got %q vs %q", d1, d2)
		}
	})
}

func verifyInvalidElectionConfigs(t *testing.T) {
	t.Helper()
	cases := []struct {
		name string
		cfg  ElectionConfig
		want error
	}{
		{
			name: "blank cluster ID",
			cfg:  ElectionConfig{ClusterID: "   ", ValidatorIDs: defaultTestValidators(), HeartbeatTimeout: testDefaultTimeout},
			want: ErrBlankClusterID,
		},
		{
			name: "non-positive heartbeat timeout",
			cfg:  ElectionConfig{ClusterID: testBFTClusterID, ValidatorIDs: defaultTestValidators(), HeartbeatTimeout: 0},
			want: ErrInvalidHeartbeatTimeout,
		},
		{
			name: "negative fault tolerance",
			cfg:  ElectionConfig{ClusterID: testBFTClusterID, ValidatorIDs: defaultTestValidators(), HeartbeatTimeout: testDefaultTimeout, FaultTolerance: -1},
			want: ErrInvalidFaultTolerance,
		},
		{
			name: "below 3f+1 floor",
			cfg:  ElectionConfig{ClusterID: testBFTClusterID, ValidatorIDs: []string{testValOne, testValTwo, testValThree}, HeartbeatTimeout: testDefaultTimeout},
			want: ErrInsufficientValidatorSet,
		},
		{
			name: "explicit f exceeds validator count",
			cfg:  ElectionConfig{ClusterID: testBFTClusterID, ValidatorIDs: defaultTestValidators(), HeartbeatTimeout: testDefaultTimeout, FaultTolerance: 2},
			want: ErrInsufficientValidatorSet,
		},
		{
			name: "blank validator ID",
			cfg:  ElectionConfig{ClusterID: testBFTClusterID, ValidatorIDs: []string{testValOne, "  ", testValThree, testValFour}, HeartbeatTimeout: testDefaultTimeout},
			want: ErrBlankValidatorID,
		},
		{
			name: "duplicate validator ID",
			cfg:  ElectionConfig{ClusterID: testBFTClusterID, ValidatorIDs: []string{testValOne, testValTwo, testValOne, testValFour}, HeartbeatTimeout: testDefaultTimeout},
			want: ErrDuplicateValidatorID,
		},
	}
	for _, tc := range cases {
		coord, err := NewElectionCoordinator(tc.cfg)
		assertBFTErrorIs(t, err, tc.want)
		if coord != nil {
			t.Fatalf("%s: expected nil coordinator", tc.name)
		}
	}
}

func verifyBFTQuorumSizing(t *testing.T) {
	t.Helper()
	cases := []struct {
		nodes      int
		wantF      int
		wantQuorum int
	}{
		{nodes: 4, wantF: 1, wantQuorum: 3},
		{nodes: 7, wantF: 2, wantQuorum: 5},
		{nodes: 10, wantF: 3, wantQuorum: 7},
	}
	for _, tc := range cases {
		vals := make([]string, tc.nodes)
		for i := 0; i < tc.nodes; i++ {
			vals[i] = fmt.Sprintf("val-%d", i+1)
		}
		coord := mustNewCoordinator(t, vals, 0)
		if coord.FaultTolerance() != tc.wantF {
			t.Fatalf("nodes=%d: expected f=%d, got %d", tc.nodes, tc.wantF, coord.FaultTolerance())
		}
		if coord.QuorumSize() != tc.wantQuorum {
			t.Fatalf("nodes=%d: expected quorum=%d, got %d", tc.nodes, tc.wantQuorum, coord.QuorumSize())
		}
	}
}

func TestByzantineLeaderElection_OperationalDynamicProof(t *testing.T) {
	t.Run("deterministic round-robin leader rotation", func(t *testing.T) {
		coord := mustNewCoordinator(t, defaultTestValidators(), 1)
		expected := []string{testValOne, testValTwo, testValThree, testValFour, testValOne, testValTwo}
		for view, want := range expected {
			if got := coord.LeaderForView(uint64(view)); got != want {
				t.Fatalf("view %d: expected leader %s, got %s", view, want, got)
			}
		}
	})

	t.Run("supermajority view succession and lease issuance", func(t *testing.T) {
		verifySupermajoritySuccession(t)
	})

	t.Run("lagging partition converges via NewViewCertificate", func(t *testing.T) {
		verifyLaggingPartitionConvergence(t)
	})

	t.Run("concurrent view-change claim submission", func(t *testing.T) {
		verifyConcurrentClaimSubmission(t)
	})
}

func verifySupermajoritySuccession(t *testing.T) {
	t.Helper()
	coord := mustNewCoordinator(t, defaultTestValidators(), 1)
	voters := []string{testValOne, testValTwo, testValThree}

	cert1 := mustAdvanceToView(t, coord, voters, 1)
	if cert1.LeaderID != testValTwo || cert1.QuorumCount != 3 || coord.CurrentView() != 1 {
		t.Fatalf("unexpected cert1 state: leader=%s quorum=%d view=%d", cert1.LeaderID, cert1.QuorumCount, coord.CurrentView())
	}

	cert2 := mustAdvanceToView(t, coord, voters, 2)
	if cert2.LeaderID != testValThree || coord.CurrentView() != 2 {
		t.Fatalf("unexpected cert2 state: leader=%s view=%d", cert2.LeaderID, coord.CurrentView())
	}

	lease, err := coord.IssueLeaderLease(2, testValThree, fixedTestTime())
	if err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	if lease.LeaderID != testValThree || lease.CertificateDigest != cert2.CertificateDigest {
		t.Fatalf("unexpected lease: %+v", lease)
	}
}

func mustNewCoordinatorPair(t *testing.T) (*ElectionCoordinator, *ElectionCoordinator) {
	t.Helper()
	return mustNewCoordinator(t, defaultTestValidators(), 1), mustNewCoordinator(t, defaultTestValidators(), 1)
}

func verifyLaggingPartitionConvergence(t *testing.T) {
	t.Helper()
	primary, lagging := mustNewCoordinatorPair(t)
	voters := defaultQuorumVoters()

	mustAdvanceToView(t, primary, voters, 1)
	cert2 := mustAdvanceToView(t, primary, voters, 2)

	if err := lagging.ConvergeToCertificate(*cert2); err != nil {
		t.Fatalf(testErrUnexpected, err)
	}
	if lagging.CurrentView() != 2 {
		t.Fatalf("expected lagging view 2, got %d", lagging.CurrentView())
	}
	activeCert := lagging.ActiveCertificate()
	if activeCert == nil || activeCert.CertificateDigest != cert2.CertificateDigest {
		t.Fatalf("lagging certificate mismatch: %+v", activeCert)
	}
	activeLease := lagging.ActiveLease()
	if activeLease == nil || activeLease.LeaderID != testValThree {
		t.Fatalf("lagging lease mismatch: %+v", activeLease)
	}
}

func verifyConcurrentClaimSubmission(t *testing.T) {
	t.Helper()
	vals := defaultTestValidators()
	coord := mustNewCoordinator(t, vals, 1)

	var mu sync.Mutex
	certs := make([]*NewViewCertificate, 0, len(vals))
	errs := make([]error, 0)

	wg := goroutinelabels.StartTestGoroutines(t, len(vals), "bft_view_claim", "submit view-change claim", func(id int) {
		claim := ViewChangeClaim{
			ClusterID:        testBFTClusterID,
			CurrentView:      0,
			TargetView:       1,
			ValidatorID:      vals[id],
			StallProofDigest: ComputeStallProofDigest(testBFTClusterID, 0, 1, vals[id]),
			Timestamp:        fixedTestTime(),
		}
		cert, err := coord.SubmitViewChangeClaim(claim)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, err)
			return
		}
		if cert != nil {
			certs = append(certs, cert)
		}
	})
	wg.Wait()

	if coord.CurrentView() != 1 {
		t.Fatalf("expected view 1 after concurrent quorum, got %d", coord.CurrentView())
	}
	if len(certs) == 0 {
		t.Fatalf("expected at least one NewViewCertificate, got 0 (errs=%v)", errs)
	}
	for _, err := range errs {
		assertBFTErrorIs(t, err, ErrStaleViewClaim)
	}
}

func TestByzantineLeaderElection_NegativeAdversarialBoundary(t *testing.T) {
	t.Run("rejects minority partition view change", func(t *testing.T) {
		coord := mustNewCoordinator(t, defaultTestValidators(), 1)
		if cert := submitClaims(t, coord, []string{testValOne, testValTwo}, 1); cert != nil {
			t.Fatalf("expected nil cert on minority quorum, got %+v", cert)
		}
		cert, err := coord.TryFormNewViewCertificate(1)
		assertBFTErrorIs(t, err, ErrInsufficientViewQuorum)
		if cert != nil {
			t.Fatalf("expected nil cert, got %+v", cert)
		}
		if coord.CurrentView() != 0 {
			t.Fatalf("expected view to remain 0, got %d", coord.CurrentView())
		}
	})

	t.Run("prevents split-brain dual leaders in same view", func(t *testing.T) {
		coord := mustNewCoordinator(t, defaultTestValidators(), 1)
		mustAdvanceToView(t, coord, []string{testValOne, testValTwo, testValThree}, 1)

		assertBFTErrorIs(t, coord.AssertLeader(1, testValOne), ErrSplitBrainLeaderConflict)
		lease, err := coord.IssueLeaderLease(1, testValFour, fixedTestTime())
		assertBFTErrorIs(t, err, ErrSplitBrainLeaderConflict)
		if lease != nil {
			t.Fatalf("expected nil lease on split-brain conflict, got %+v", lease)
		}
	})

	t.Run("rejects stale view claims and forged stall proofs", func(t *testing.T) {
		verifyStaleAndForgedClaims(t)
	})

	t.Run("rejects tampered or minority certificates during convergence", func(t *testing.T) {
		verifyInvalidConvergenceCertificates(t)
	})
}

func assertClaimRejected(t *testing.T, coord *ElectionCoordinator, claim ViewChangeClaim, want error) {
	t.Helper()
	_, err := coord.SubmitViewChangeClaim(claim)
	assertBFTErrorIs(t, err, want)
}

func verifyStaleAndForgedClaims(t *testing.T) {
	t.Helper()
	coord := mustNewCoordinator(t, defaultTestValidators(), 1)
	mustAdvanceToView(t, coord, []string{testValOne, testValTwo, testValThree}, 2)

	staleClaim := ViewChangeClaim{
		ClusterID:        testBFTClusterID,
		CurrentView:      0,
		TargetView:       1,
		ValidatorID:      testValFour,
		StallProofDigest: ComputeStallProofDigest(testBFTClusterID, 0, 1, testValFour),
	}
	assertClaimRejected(t, coord, staleClaim, ErrStaleViewClaim)

	emptyProof := ViewChangeClaim{
		ClusterID:        testBFTClusterID,
		CurrentView:      2,
		TargetView:       3,
		ValidatorID:      testValOne,
		StallProofDigest: "   ",
	}
	assertClaimRejected(t, coord, emptyProof, ErrInvalidStallProof)

	forgedProof := emptyProof
	forgedProof.StallProofDigest = testForgedDigestStr
	assertClaimRejected(t, coord, forgedProof, ErrInvalidStallProof)

	foreignClaim := emptyProof
	foreignClaim.ClusterID = testForeignCluster
	foreignClaim.StallProofDigest = ComputeStallProofDigest(testForeignCluster, 2, 3, testValOne)
	assertClaimRejected(t, coord, foreignClaim, ErrClusterMismatch)
}

func verifyInvalidConvergenceCertificates(t *testing.T) {
	t.Helper()
	primary, lagging := mustNewCoordinatorPair(t)
	cert := mustAdvanceToView(t, primary, defaultQuorumVoters(), 1)

	cases := []struct {
		mutate func(*NewViewCertificate)
		want   error
	}{
		{
			mutate: func(c *NewViewCertificate) { c.CertificateDigest = testForgedDigestStr },
			want:   ErrInvalidCertificate,
		},
		{
			mutate: func(c *NewViewCertificate) { c.LeaderID = testValFour },
			want:   ErrSplitBrainLeaderConflict,
		},
		{
			mutate: func(c *NewViewCertificate) { c.Claims = cert.Claims[:2] },
			want:   ErrInsufficientViewQuorum,
		},
		{
			mutate: func(c *NewViewCertificate) { c.ClusterID = testForeignCluster },
			want:   ErrClusterMismatch,
		},
	}
	for _, tc := range cases {
		tampered := *cert
		tc.mutate(&tampered)
		assertBFTErrorIs(t, lagging.ConvergeToCertificate(tampered), tc.want)
	}
}
