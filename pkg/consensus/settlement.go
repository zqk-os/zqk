package consensus

import (
	"context"
	"errors"
	"sync"
	"time"
)

type ShockwaveEvent struct {
	EventID   string    `json:"event_id"`
	Source    string    `json:"source"`
	Epoch     int64     `json:"epoch"`
	Kind      string    `json:"kind"`
	Payload   []byte    `json:"payload"`
	EmittedAt time.Time `json:"emitted_at"`
}

type QuorumVote struct {
	VoterID   string `json:"voter_id"`
	Approved  bool   `json:"approved"`
	Signature string `json:"signature"`
}

type SettlementProposal struct {
	Index        int64                  `json:"index"`
	PreviousHash string                 `json:"previous_hash"`
	Payload      []byte                 `json:"payload"`
	ProposedBy   string                 `json:"proposed_by"`
	Epoch        int64                  `json:"epoch"`
	Votes        map[string]*QuorumVote `json:"votes"`
}

type SettlementEngine struct {
	mu          sync.RWMutex
	ledger      *ConsensusLedger
	totalPeers  int
	quorumPct   float64
	shockwaves  []*ShockwaveEvent
	subscribers []func(*ShockwaveEvent)
}

func NewSettlementEngine(ledger *ConsensusLedger, totalPeers int) *SettlementEngine {
	if totalPeers <= 0 {
		totalPeers = 1
	}
	return &SettlementEngine{
		ledger:      ledger,
		totalPeers:  totalPeers,
		quorumPct:   0.66,
		shockwaves:  make([]*ShockwaveEvent, 0),
		subscribers: make([]func(*ShockwaveEvent), 0),
	}
}

func (s *SettlementEngine) Subscribe(listener func(*ShockwaveEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribers = append(s.subscribers, listener)
}

func (s *SettlementEngine) EmitShockwave(ctx context.Context, ev *ShockwaveEvent) error {
	if ev == nil || ev.EventID == "" || ev.Source == "" {
		return errors.New("invalid shockwave event")
	}

	s.mu.Lock()
	s.shockwaves = append(s.shockwaves, ev)
	subs := make([]func(*ShockwaveEvent), len(s.subscribers))
	copy(subs, s.subscribers)
	s.mu.Unlock()

	for _, sub := range subs {
		sub(ev)
	}
	return nil
}

func (s *SettlementEngine) SettleProposal(ctx context.Context, prop *SettlementProposal) (*LedgerEntry, error) {
	if prop == nil {
		return nil, errors.New("proposal cannot be nil")
	}

	s.mu.RLock()
	total := s.totalPeers
	quorumReq := int(float64(total)*s.quorumPct) + 1
	if quorumReq > total {
		quorumReq = total
	}
	s.mu.RUnlock()

	approvedVotes := 0
	signatures := make(map[string]string)
	for voter, vote := range prop.Votes {
		if vote.Approved {
			approvedVotes++
			signatures[voter] = vote.Signature
		}
	}

	if approvedVotes < quorumReq {
		return nil, ErrQuorumShortfall
	}

	ts := time.Now().UTC()
	hash := CalculateEntryHash(prop.Index, prop.PreviousHash, prop.Payload, prop.ProposedBy, prop.Epoch, ts)

	entry := &LedgerEntry{
		Index:        prop.Index,
		PreviousHash: prop.PreviousHash,
		Hash:         hash,
		Payload:      prop.Payload,
		ProposedBy:   prop.ProposedBy,
		Epoch:        prop.Epoch,
		Timestamp:    ts,
		Status:       StatusCommitted,
		Signatures:   signatures,
	}

	if err := s.ledger.AppendEntry(entry); err != nil {
		return nil, err
	}

	return entry, nil
}
