package inbox

import (
	"errors"
	"sync"
)

var (
	ErrEnvelopeNotFound = errors.New("envelope not found")
	ErrEnvelopeExists   = errors.New("envelope already exists")
	ErrNotPending       = errors.New("envelope is not pending")
)

// EnvelopeStatus represents the lifecycle state of a TDE Envelope.
type EnvelopeStatus string

const (
	StatusPending  EnvelopeStatus = "pending"
	StatusApproved EnvelopeStatus = "approved"
	StatusRejected EnvelopeStatus = "rejected"
)

// TDEEnvelope represents a Time-Delayed Execution request awaiting approval.
type TDEEnvelope struct {
	ID             string
	AgentID        string
	Intent         string
	CapabilityRefs []string
	Payload        []byte
	Status         EnvelopeStatus
	RejectReason   string
}

// Inbox provides the interface for managing TDE Envelopes.
type Inbox interface {
	Submit(env TDEEnvelope) error
	Get(id string) (TDEEnvelope, error)
	ListPending() []TDEEnvelope
	Approve(id string) error
	Reject(id string, reason string) error
}

type memoryInbox struct {
	mu        sync.RWMutex
	envelopes map[string]*TDEEnvelope
}

// NewMemoryInbox creates a new, thread-safe, in-memory Autonomy Inbox.
func NewMemoryInbox() Inbox {
	return &memoryInbox{
		envelopes: make(map[string]*TDEEnvelope),
	}
}

func (i *memoryInbox) Submit(env TDEEnvelope) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, exists := i.envelopes[env.ID]; exists {
		return ErrEnvelopeExists
	}

	env.Status = StatusPending
	i.envelopes[env.ID] = &env
	return nil
}

func (i *memoryInbox) Get(id string) (TDEEnvelope, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if env, ok := i.envelopes[id]; ok {
		return *env, nil
	}
	return TDEEnvelope{}, ErrEnvelopeNotFound
}

func (i *memoryInbox) ListPending() []TDEEnvelope {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var pending []TDEEnvelope
	for _, env := range i.envelopes {
		if env.Status == StatusPending {
			pending = append(pending, *env)
		}
	}
	return pending
}

func (i *memoryInbox) getPendingEnvelopeLocked(id string) (*TDEEnvelope, error) {
	env, ok := i.envelopes[id]
	if !ok {
		return nil, ErrEnvelopeNotFound
	}
	if env.Status != StatusPending {
		return nil, ErrNotPending
	}
	return env, nil
}

func (i *memoryInbox) updatePendingEnvelopeStatus(id string, updateFn func(*TDEEnvelope)) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	env, err := i.getPendingEnvelopeLocked(id)
	if err != nil {
		return err
	}
	updateFn(env)
	return nil
}

func (i *memoryInbox) Approve(id string) error {
	return i.updatePendingEnvelopeStatus(id, func(env *TDEEnvelope) {
		env.Status = StatusApproved
	})
}

func (i *memoryInbox) Reject(id string, reason string) error {
	return i.updatePendingEnvelopeStatus(id, func(env *TDEEnvelope) {
		env.Status = StatusRejected
		env.RejectReason = reason
	})
}
