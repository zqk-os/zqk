package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrEmptyHash          = errors.New("hash cannot be empty")
	ErrEmptyPreviousHash  = errors.New("previous_hash cannot be empty for non-genesis entry")
	ErrEmptyProposedBy    = errors.New("proposed_by cannot be empty")
	ErrInvalidIndex       = errors.New("index must be greater than or equal to 0")
	ErrInvalidTimestamp   = errors.New("timestamp must be non-zero")
	ErrInvalidStatus      = errors.New("status must be proposed, committed, or rejected")
	ErrEntryAlreadyExists = errors.New("entry with given index or hash already exists")
	ErrHashMismatch       = errors.New("calculated hash does not match declared entry hash")
	ErrChainLinkageBroken = errors.New("previous_hash does not match head of ledger chain")
	ErrStaleEpoch         = errors.New("proposal epoch is older than current ledger epoch")
	ErrQuorumShortfall    = errors.New("proposal does not meet quorum threshold")
	ErrForkDetected       = errors.New("conflicting concurrent fork detected at index")
)

type SettlementStatus string

const (
	StatusProposed  SettlementStatus = "proposed"
	StatusCommitted SettlementStatus = "committed"
	StatusRejected  SettlementStatus = "rejected"
)

type LedgerEntry struct {
	Index        int64             `json:"index"`
	PreviousHash string            `json:"previous_hash"`
	Hash         string            `json:"hash"`
	Payload      []byte            `json:"payload"`
	ProposedBy   string            `json:"proposed_by"`
	Epoch        int64             `json:"epoch"`
	Timestamp    time.Time         `json:"timestamp"`
	Status       SettlementStatus  `json:"status"`
	Signatures   map[string]string `json:"signatures,omitempty"`
}

func CalculateEntryHash(index int64, prevHash string, payload []byte, proposedBy string, epoch int64, ts time.Time) string {
	h := sha256.New()
	_, _ = h.Write([]byte(fmt.Sprintf("%d:%s:%s:%d:%d:", index, prevHash, proposedBy, epoch, ts.UnixNano())))
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func (e *LedgerEntry) Validate() error {
	if e == nil {
		return errors.New("ledger entry cannot be nil")
	}
	if e.Index < 0 {
		return ErrInvalidIndex
	}
	if e.Index > 0 && e.PreviousHash == "" {
		return ErrEmptyPreviousHash
	}
	if e.Hash == "" {
		return ErrEmptyHash
	}
	if e.ProposedBy == "" {
		return ErrEmptyProposedBy
	}
	if e.Timestamp.IsZero() {
		return ErrInvalidTimestamp
	}
	switch e.Status {
	case StatusProposed, StatusCommitted, StatusRejected:
	default:
		return ErrInvalidStatus
	}
	return nil
}

type ConsensusLedger struct {
	mu      sync.RWMutex
	chain   []*LedgerEntry
	entries map[string]*LedgerEntry
	epoch   int64
}

func NewConsensusLedger() *ConsensusLedger {
	return &ConsensusLedger{
		chain:   make([]*LedgerEntry, 0),
		entries: make(map[string]*LedgerEntry),
		epoch:   1,
	}
}

func (l *ConsensusLedger) CurrentEpoch() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.epoch
}

func (l *ConsensusLedger) SetEpoch(epoch int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if epoch > l.epoch {
		l.epoch = epoch
	}
}

func (l *ConsensusLedger) Head() *LedgerEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.chain) == 0 {
		return nil
	}
	return l.chain[len(l.chain)-1]
}

func (l *ConsensusLedger) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.chain)
}

func (l *ConsensusLedger) AppendEntry(entry *LedgerEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.entries[entry.Hash]; exists {
		return ErrEntryAlreadyExists
	}

	if entry.Epoch < l.epoch {
		return ErrStaleEpoch
	}

	if len(l.chain) == 0 {
		if entry.Index != 0 {
			return errors.New("genesis entry must have index 0")
		}
	} else {
		head := l.chain[len(l.chain)-1]
		if entry.Index != head.Index+1 {
			return ErrInvalidIndex
		}
		if entry.PreviousHash != head.Hash {
			return ErrChainLinkageBroken
		}
	}

	calculated := CalculateEntryHash(entry.Index, entry.PreviousHash, entry.Payload, entry.ProposedBy, entry.Epoch, entry.Timestamp)
	if calculated != entry.Hash {
		return ErrHashMismatch
	}

	l.chain = append(l.chain, entry)
	l.entries[entry.Hash] = entry
	if entry.Epoch > l.epoch {
		l.epoch = entry.Epoch
	}
	return nil
}
