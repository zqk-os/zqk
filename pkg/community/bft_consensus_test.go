package community

import (
	"fmt"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testBFTDigestAlpha = "sha256-digest-alpha"
	testBFTDigestBeta  = "sha256-digest-beta"
	testBFTValOne      = "val-us-east-1"
	testBFTValTwo      = "val-eu-west-1"
	testBFTValThree    = "val-ap-south-1"
	testBFTValFour     = "val-sa-east-1"
	testBFTSigPrefix   = "sig-ed25519"
)

func testBFTValidators() []Validator {
	return []Validator{
		{ID: testBFTValOne, Region: "us-east", PublicKey: "pk-1"},
		{ID: testBFTValTwo, Region: "eu-west", PublicKey: "pk-2"},
		{ID: testBFTValThree, Region: "ap-south", PublicKey: "pk-3"},
		{ID: testBFTValFour, Region: "sa-east", PublicKey: "pk-4"},
	}
}

func testBFTSlices() []QuorumSlice {
	return []QuorumSlice{
		{ID: "slice-atlantic", Members: []string{testBFTValOne, testBFTValTwo, testBFTValThree}, Threshold: 3},
		{ID: "slice-pacific", Members: []string{testBFTValTwo, testBFTValThree, testBFTValFour}, Threshold: 3},
	}
}

func mustNewTestBFTEngine(t *testing.T) *BFTEngine {
	t.Helper()
	engine, err := NewBFTEngine(BFTConfig{
		Validators: testBFTValidators(),
		Slices:     testBFTSlices(),
		MaxFaults:  1,
	})
	if err != nil {
		t.Fatalf("NewBFTEngine failed: %v", err)
	}
	return engine
}

func mustSubmitTestProposal(t *testing.T, engine *BFTEngine, height, view uint64, digest, proposer string) {
	t.Helper()
	err := engine.SubmitProposal(Proposal{
		Height:     height,
		View:       view,
		Digest:     digest,
		ProposerID: proposer,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SubmitProposal(h=%d,v=%d) failed: %v", height, view, err)
	}
}

func mustCastTestVotes(t *testing.T, engine *BFTEngine, height, view uint64, phase ConsensusPhase, digest string, voters []string) ConsensusPhase {
	t.Helper()
	var lastPhase ConsensusPhase
	for _, id := range voters {
		nextPhase, err := engine.CastVote(Vote{
			Height:         height,
			View:           view,
			Phase:          phase,
			ProposalDigest: digest,
			ValidatorID:    id,
			Signature:      fmt.Sprintf("%s-%s-%s", testBFTSigPrefix, id, phase),
			Timestamp:      time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("CastVote(voter=%s,phase=%s) failed: %v", id, phase, err)
		}
		lastPhase = nextPhase
	}
	return lastPhase
}

func TestBFTConsensus_QuorumSliceAndSupermajorityInvariants(t *testing.T) {
	cases := []struct {
		name string
		cfg  BFTConfig
	}{
		{
			name: "empty_validators",
			cfg:  BFTConfig{Validators: nil, MaxFaults: 0},
		},
		{
			name: "below_3f_plus_1",
			cfg:  BFTConfig{Validators: testBFTValidators()[:3], MaxFaults: 1},
		},
		{
			name: "duplicate_validator_id",
			cfg: BFTConfig{
				Validators: []Validator{{ID: testBFTValOne}, {ID: testBFTValOne}, {ID: testBFTValTwo}, {ID: testBFTValThree}},
				MaxFaults:  1,
			},
		},
		{
			name: "blank_validator_id",
			cfg: BFTConfig{
				Validators: []Validator{{ID: "  "}, {ID: testBFTValTwo}, {ID: testBFTValThree}, {ID: testBFTValFour}},
				MaxFaults:  1,
			},
		},
		{
			name: "below_2f_plus_1_threshold",
			cfg:  BFTConfig{Validators: testBFTValidators(), MaxFaults: 1, Threshold: 2},
		},
		{
			name: "disjoint_quorum_slices",
			cfg: BFTConfig{
				Validators: testBFTValidators(),
				MaxFaults:  1,
				Slices: []QuorumSlice{
					{ID: "slice-a", Members: []string{testBFTValOne, testBFTValTwo}, Threshold: 2},
					{ID: "slice-b", Members: []string{testBFTValThree, testBFTValFour}, Threshold: 2},
				},
			},
		},
		{
			name: "insufficient_pigeonhole_overlap",
			cfg: BFTConfig{
				Validators: testBFTValidators(),
				MaxFaults:  1,
				Slices: []QuorumSlice{
					{ID: "slice-a", Members: []string{testBFTValOne, testBFTValTwo, testBFTValThree}, Threshold: 2},
					{ID: "slice-b", Members: []string{testBFTValTwo, testBFTValThree, testBFTValFour}, Threshold: 2},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, err := NewBFTEngine(tc.cfg)
			if err == nil || engine != nil {
				t.Fatalf("expected NewBFTEngine to fail for %s, got nil error", tc.name)
			}
		})
	}
}

func TestBFTConsensus_MultiPhaseCommitFinalization(t *testing.T) {
	engine := mustNewTestBFTEngine(t)
	mustSubmitTestProposal(t, engine, 10, 0, testBFTDigestAlpha, testBFTValOne)

	phase, ok := engine.CurrentPhase(10, 0)
	if !ok || phase != PhasePrepare {
		t.Fatalf("expected initial phase %s, got %s (ok=%v)", PhasePrepare, phase, ok)
	}

	quorum := []string{testBFTValOne, testBFTValTwo, testBFTValThree}
	if got := mustCastTestVotes(t, engine, 10, 0, PhasePrepare, testBFTDigestAlpha, quorum); got != PhasePreCommit {
		t.Fatalf("expected transition to %s, got %s", PhasePreCommit, got)
	}
	if got := mustCastTestVotes(t, engine, 10, 0, PhasePreCommit, testBFTDigestAlpha, quorum); got != PhaseCommit {
		t.Fatalf("expected transition to %s, got %s", PhaseCommit, got)
	}
	if got := mustCastTestVotes(t, engine, 10, 0, PhaseCommit, testBFTDigestAlpha, quorum); got != PhaseFinalized {
		t.Fatalf("expected transition to %s, got %s", PhaseFinalized, got)
	}

	digest, finalized := engine.FinalizedDigest(10)
	if !finalized || digest != testBFTDigestAlpha {
		t.Fatalf("expected finalized digest %s, got %s (finalized=%v)", testBFTDigestAlpha, digest, finalized)
	}
}

func TestBFTConsensus_ConcurrentMultiHeightFinalization(t *testing.T) {
	engine := mustNewTestBFTEngine(t)
	const heightCount = 6
	quorum := []string{testBFTValOne, testBFTValTwo, testBFTValThree}

	wg := goroutinelabels.StartTestGoroutines(t, heightCount, "bft_height_worker", "finalize bft height", func(idx int) {
		height := uint64(idx + 1)
		digest := fmt.Sprintf("%s-%d", testBFTDigestAlpha, height)
		mustSubmitTestProposal(t, engine, height, 0, digest, testBFTValOne)
		mustCastTestVotes(t, engine, height, 0, PhasePrepare, digest, quorum)
		mustCastTestVotes(t, engine, height, 0, PhasePreCommit, digest, quorum)
		mustCastTestVotes(t, engine, height, 0, PhaseCommit, digest, quorum)
	})
	wg.Wait()

	for i := 0; i < heightCount; i++ {
		height := uint64(i + 1)
		expected := fmt.Sprintf("%s-%d", testBFTDigestAlpha, height)
		got, ok := engine.FinalizedDigest(height)
		if !ok || got != expected {
			t.Fatalf("height %d: expected finalized %s, got %s (ok=%v)", height, expected, got, ok)
		}
	}
}

func TestBFTConsensus_EquivocationSlashingAndRejection(t *testing.T) {
	engine := mustNewTestBFTEngine(t)
	mustSubmitTestProposal(t, engine, 42, 0, testBFTDigestAlpha, testBFTValOne)

	mustCastTestVotes(t, engine, 42, 0, PhasePrepare, testBFTDigestAlpha, []string{testBFTValOne})

	_, err := engine.CastVote(Vote{
		Height:         42,
		View:           0,
		Phase:          PhasePrepare,
		ProposalDigest: testBFTDigestBeta,
		ValidatorID:    testBFTValOne,
		Signature:      testBFTSigPrefix + "-equivocate",
	})
	if err == nil {
		t.Fatal("expected equivocation error on conflicting vote digest")
	}
	if !engine.IsSlashed(testBFTValOne) {
		t.Fatalf("expected validator %s to be slashed", testBFTValOne)
	}
	if engine.ActiveValidatorCount() != 3 {
		t.Fatalf("expected 3 active validators after slash, got %d", engine.ActiveValidatorCount())
	}
	evidence := engine.Equivocations()
	if len(evidence) != 1 || evidence[0].FirstDigest != testBFTDigestAlpha || evidence[0].SecondDigest != testBFTDigestBeta {
		t.Fatalf("unexpected equivocation evidence: %+v", evidence)
	}

	_, slashedErr := engine.CastVote(Vote{
		Height:         42,
		View:           0,
		Phase:          PhasePrepare,
		ProposalDigest: testBFTDigestAlpha,
		ValidatorID:    testBFTValOne,
		Signature:      testBFTSigPrefix + "-after-slash",
	})
	if slashedErr == nil {
		t.Fatal("expected slashed validator vote to be rejected")
	}
}

func TestBFTConsensus_MaliciousProposalAndInsufficientQuorum(t *testing.T) {
	engine := mustNewTestBFTEngine(t)
	mustSubmitTestProposal(t, engine, 99, 1, testBFTDigestAlpha, testBFTValFour)

	err := engine.SubmitProposal(Proposal{
		Height:     99,
		View:       1,
		Digest:     testBFTDigestBeta,
		ProposerID: testBFTValFour,
	})
	if err == nil || !engine.IsSlashed(testBFTValFour) {
		t.Fatalf("expected conflicting proposal to slash %s, err=%v", testBFTValFour, err)
	}

	_, unknownErr := engine.CastVote(Vote{
		Height:         99,
		View:           1,
		Phase:          PhasePrepare,
		ProposalDigest: testBFTDigestAlpha,
		ValidatorID:    "val-unregistered",
		Signature:      testBFTSigPrefix + "-unknown",
	})
	if unknownErr == nil {
		t.Fatal("expected unregistered validator vote to fail")
	}

	// Only 2 votes (< 2f+1 = 3) must not advance phase or finalize.
	gotPhase := mustCastTestVotes(t, engine, 99, 1, PhasePrepare, testBFTDigestAlpha, []string{testBFTValOne, testBFTValTwo})
	if gotPhase != PhasePrepare {
		t.Fatalf("expected phase to remain %s with insufficient quorum, got %s", PhasePrepare, gotPhase)
	}
	if _, finalized := engine.FinalizedDigest(99); finalized {
		t.Fatal("expected height 99 not to finalize with insufficient quorum")
	}
}
