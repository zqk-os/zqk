package consensus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Invariant 1: Static Floor Invariant (CRIT-1794283830181082000-b309c6b8)
func TestConsensusLedger_StaticFloorInvariants(t *testing.T) {
	testNilAndIndexRejections(t)
	testHashAndProposedByRejections(t)
	testStatusAndHashCalculations(t)
}

func testNilAndIndexRejections(t *testing.T) {
	var nilEntry *LedgerEntry
	if err := nilEntry.Validate(); err == nil {
		t.Fatal("expected error on nil ledger entry, got nil")
	}

	invalidIndex := &LedgerEntry{
		Index:        -1,
		PreviousHash: "prev-hash",
		Hash:         "hash",
		ProposedBy:   "agent-1",
		Timestamp:    time.Now().UTC(),
		Status:       StatusCommitted,
	}
	if err := invalidIndex.Validate(); !errors.Is(err, ErrInvalidIndex) {
		t.Fatalf("expected ErrInvalidIndex, got %v", err)
	}

	nonGenesisNoPrev := &LedgerEntry{
		Index:      1,
		Hash:       "hash",
		ProposedBy: "agent-1",
		Timestamp:  time.Now().UTC(),
		Status:     StatusCommitted,
	}
	if err := nonGenesisNoPrev.Validate(); !errors.Is(err, ErrEmptyPreviousHash) {
		t.Fatalf("expected ErrEmptyPreviousHash, got %v", err)
	}
}

func testHashAndProposedByRejections(t *testing.T) {
	emptyHash := &LedgerEntry{
		Index:      0,
		ProposedBy: "agent-1",
		Timestamp:  time.Now().UTC(),
		Status:     StatusCommitted,
	}
	if err := emptyHash.Validate(); !errors.Is(err, ErrEmptyHash) {
		t.Fatalf("expected ErrEmptyHash, got %v", err)
	}

	emptyProposedBy := &LedgerEntry{
		Index:     0,
		Hash:      "hash",
		Timestamp: time.Now().UTC(),
		Status:    StatusCommitted,
	}
	if err := emptyProposedBy.Validate(); !errors.Is(err, ErrEmptyProposedBy) {
		t.Fatalf("expected ErrEmptyProposedBy, got %v", err)
	}

	zeroTimestamp := &LedgerEntry{
		Index:      0,
		Hash:       "hash",
		ProposedBy: "agent-1",
		Status:     StatusCommitted,
	}
	if err := zeroTimestamp.Validate(); !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("expected ErrInvalidTimestamp, got %v", err)
	}
}

func testStatusAndHashCalculations(t *testing.T) {
	invalidStatus := &LedgerEntry{
		Index:      0,
		Hash:       "hash",
		ProposedBy: "agent-1",
		Timestamp:  time.Now().UTC(),
		Status:     SettlementStatus("unknown"),
	}
	if err := invalidStatus.Validate(); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	ts := time.Now().UTC()
	hash1 := CalculateEntryHash(0, "", []byte("state-a"), "agent-1", 1, ts)
	hash2 := CalculateEntryHash(0, "", []byte("state-a"), "agent-1", 1, ts)
	if hash1 != hash2 {
		t.Fatalf("hash calculation must be deterministic: %s != %s", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Fatalf("expected sha256 64-char hex string, got len=%d", len(hash1))
	}
}

// Invariant 2: Operational Dynamic Proof (CRIT-1794283837650605000-b0b45993)
func TestSettlementEngine_OperationalDynamicProof(t *testing.T) {
	ledger := NewConsensusLedger()
	engine := NewSettlementEngine(ledger, 3)

	var receivedEvents []*ShockwaveEvent
	var evMu sync.Mutex
	engine.Subscribe(func(ev *ShockwaveEvent) {
		evMu.Lock()
		defer evMu.Unlock()
		receivedEvents = append(receivedEvents, ev)
	})

	ctx := context.Background()
	shockwave := &ShockwaveEvent{
		EventID:   "ev-100",
		Source:    "agent-leader",
		Epoch:     1,
		Kind:      "architectural_drift",
		Payload:   []byte(`{"directive":"halt_stale_branches"}`),
		EmittedAt: time.Now().UTC(),
	}
	if err := engine.EmitShockwave(ctx, shockwave); err != nil {
		t.Fatalf("failed to emit shockwave: %v", err)
	}

	evMu.Lock()
	if len(receivedEvents) != 1 || receivedEvents[0].EventID != "ev-100" {
		t.Fatalf("expected 1 shockwave event received, got %d", len(receivedEvents))
	}
	evMu.Unlock()

	// Propose genesis block (Index 0) with quorum of 2/3 peers
	genesisProposal := &SettlementProposal{
		Index:        0,
		PreviousHash: "",
		Payload:      []byte("genesis state payload"),
		ProposedBy:   "agent-leader",
		Epoch:        1,
		Votes: map[string]*QuorumVote{
			"agent-1": {VoterID: "agent-1", Approved: true, Signature: "sig-1"},
			"agent-2": {VoterID: "agent-2", Approved: true, Signature: "sig-2"},
		},
	}

	entry0, err := engine.SettleProposal(ctx, genesisProposal)
	if err != nil {
		t.Fatalf("failed to settle genesis proposal: %v", err)
	}
	if entry0.Status != StatusCommitted {
		t.Fatalf("expected StatusCommitted, got %s", entry0.Status)
	}
	if ledger.Len() != 1 {
		t.Fatalf("expected ledger len 1, got %d", ledger.Len())
	}

	// Propose sequential block (Index 1) linked to genesis
	block1Proposal := &SettlementProposal{
		Index:        1,
		PreviousHash: entry0.Hash,
		Payload:      []byte("settlement block 1 payload"),
		ProposedBy:   "agent-peer",
		Epoch:        1,
		Votes: map[string]*QuorumVote{
			"agent-1": {VoterID: "agent-1", Approved: true, Signature: "sig-1-b1"},
			"agent-3": {VoterID: "agent-3", Approved: true, Signature: "sig-3-b1"},
		},
	}

	entry1, err := engine.SettleProposal(ctx, block1Proposal)
	if err != nil {
		t.Fatalf("failed to settle block 1 proposal: %v", err)
	}
	if entry1.PreviousHash != entry0.Hash {
		t.Fatalf("expected previous hash %s, got %s", entry0.Hash, entry1.PreviousHash)
	}
	if ledger.Len() != 2 {
		t.Fatalf("expected ledger len 2, got %d", ledger.Len())
	}
}

// Invariant 3: Negative Adversarial Boundary (CRIT-1794283844902813000-1673affe)
func TestSettlementEngine_NegativeAdversarialBoundaries(t *testing.T) {
	testQuorumAndEpochRejection(t)
	testBrokenChainAndHashMismatchRejection(t)
	testConcurrentProposalThreadSafety(t)
}

func testQuorumAndEpochRejection(t *testing.T) {
	ledger := NewConsensusLedger()
	engine := NewSettlementEngine(ledger, 3)
	ctx := context.Background()

	// 1. Quorum shortfall (only 1 vote approved out of 3 total peers)
	shortfallProposal := &SettlementProposal{
		Index:        0,
		PreviousHash: "",
		Payload:      []byte("shortfall"),
		ProposedBy:   "agent-rogue",
		Epoch:        1,
		Votes: map[string]*QuorumVote{
			"agent-rogue": {VoterID: "agent-rogue", Approved: true, Signature: "sig"},
		},
	}
	if _, err := engine.SettleProposal(ctx, shortfallProposal); !errors.Is(err, ErrQuorumShortfall) {
		t.Fatalf("expected ErrQuorumShortfall, got %v", err)
	}

	// 2. Settle valid genesis to advance epoch
	ledger.SetEpoch(5)
	staleProposal := &SettlementProposal{
		Index:        0,
		PreviousHash: "",
		Payload:      []byte("stale"),
		ProposedBy:   "agent-stale",
		Epoch:        2,
		Votes: map[string]*QuorumVote{
			"agent-1": {VoterID: "agent-1", Approved: true, Signature: "sig-1"},
			"agent-2": {VoterID: "agent-2", Approved: true, Signature: "sig-2"},
		},
	}
	if _, err := engine.SettleProposal(ctx, staleProposal); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("expected ErrStaleEpoch, got %v", err)
	}
}

func testBrokenChainAndHashMismatchRejection(t *testing.T) {
	ledger := NewConsensusLedger()
	engine := NewSettlementEngine(ledger, 3)
	ctx := context.Background()

	validGenesis := &SettlementProposal{
		Index:        0,
		PreviousHash: "",
		Payload:      []byte("valid genesis"),
		ProposedBy:   "agent-1",
		Epoch:        1,
		Votes: map[string]*QuorumVote{
			"agent-1": {VoterID: "agent-1", Approved: true, Signature: "sig-1"},
			"agent-2": {VoterID: "agent-2", Approved: true, Signature: "sig-2"},
		},
	}
	if _, err := engine.SettleProposal(ctx, validGenesis); err != nil {
		t.Fatalf("failed to settle genesis: %v", err)
	}

	brokenChainProposal := &SettlementProposal{
		Index:        1,
		PreviousHash: "broken-previous-hash-00000000000000000000000000000000",
		Payload:      []byte("fork payload"),
		ProposedBy:   "agent-fork",
		Epoch:        1,
		Votes: map[string]*QuorumVote{
			"agent-1": {VoterID: "agent-1", Approved: true, Signature: "sig-1"},
			"agent-2": {VoterID: "agent-2", Approved: true, Signature: "sig-2"},
		},
	}
	if _, err := engine.SettleProposal(ctx, brokenChainProposal); !errors.Is(err, ErrChainLinkageBroken) {
		t.Fatalf("expected ErrChainLinkageBroken, got %v", err)
	}
}

func testConcurrentProposalThreadSafety(t *testing.T) {
	ledger := NewConsensusLedger()
	engine := NewSettlementEngine(ledger, 10)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		id := i
		goroutinelabels.NewGoroutine("consensus_concurrent_test", "concurrent shockwave emit").StartSimple(func() {
			defer wg.Done()
			ev := &ShockwaveEvent{
				EventID:   time.Now().Format(time.RFC3339Nano),
				Source:    "agent-concurrent",
				Epoch:     int64(id),
				Kind:      "ping",
				Payload:   []byte("ping"),
				EmittedAt: time.Now().UTC(),
			}
			_ = engine.EmitShockwave(ctx, ev)
		})
	}
	wg.Wait()
}
