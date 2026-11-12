package community

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrBlankID is returned when an identifier is empty or whitespace.
	ErrBlankID = errors.New("crdt: identifier cannot be blank")
	// ErrInvalidTimestamp is returned when a timestamp is non-positive.
	ErrInvalidTimestamp = errors.New("crdt: timestamp must be positive")
	// ErrNilDelta is returned when a delta payload is nil.
	ErrNilDelta = errors.New("crdt: delta cannot be nil")
	// ErrCorruptPayload is returned when a delta or register state is malformed.
	ErrCorruptPayload = errors.New("crdt: payload is corrupt or invalid")
)

// LWWRegister represents a Last-Write-Wins scalar register.
type LWWRegister struct {
	Value     string `json:"value"`
	Timestamp int64  `json:"timestamp"`
	WriterID  string `json:"writer_id"`
}

// NewLWWRegister constructs a valid LWWRegister with validation.
func NewLWWRegister(value string, timestamp int64, writerID string) (LWWRegister, error) {
	if err := strutil.ErrorOnBlank(writerID, "%w: writer_id is required", ErrBlankID); err != nil {
		return LWWRegister{}, err
	}
	if timestamp <= 0 {
		return LWWRegister{}, ErrInvalidTimestamp
	}
	return LWWRegister{
		Value:     value,
		Timestamp: timestamp,
		WriterID:  writerID,
	}, nil
}

// Merge returns a new LWWRegister merging r and other using LWW rules with writerID tie-breaking.
func (r LWWRegister) Merge(other LWWRegister) LWWRegister {
	if other.Timestamp > r.Timestamp {
		return other
	}
	if other.Timestamp == r.Timestamp && other.WriterID > r.WriterID {
		return other
	}
	return r
}

// ElementRecord tracks an element timestamp and writer for LWW-Element-Set.
type ElementRecord struct {
	Timestamp int64  `json:"timestamp"`
	WriterID  string `json:"writer_id"`
}

// LWWElementSet is an add-wins/remove-wins LWW element set.
type LWWElementSet struct {
	addSet    map[string]ElementRecord
	removeSet map[string]ElementRecord
}

// NewLWWElementSet instantiates an empty LWWElementSet.
func NewLWWElementSet() *LWWElementSet {
	return &LWWElementSet{
		addSet:    make(map[string]ElementRecord),
		removeSet: make(map[string]ElementRecord),
	}
}

// Add inserts or updates an element in the add set.
func (s *LWWElementSet) Add(element string, timestamp int64, writerID string) error {
	if err := strutil.ErrorOnBlank(element, "%w: element is required", ErrBlankID); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(writerID, "%w: writer_id is required", ErrBlankID); err != nil {
		return err
	}
	if timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	existing, ok := s.addSet[element]
	if !ok || timestamp > existing.Timestamp || (timestamp == existing.Timestamp && writerID > existing.WriterID) {
		s.addSet[element] = ElementRecord{Timestamp: timestamp, WriterID: writerID}
	}
	return nil
}

// Remove marks an element as removed in the remove set.
func (s *LWWElementSet) Remove(element string, timestamp int64, writerID string) error {
	if err := strutil.ErrorOnBlank(element, "%w: element is required", ErrBlankID); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(writerID, "%w: writer_id is required", ErrBlankID); err != nil {
		return err
	}
	if timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	existing, ok := s.removeSet[element]
	if !ok || timestamp > existing.Timestamp || (timestamp == existing.Timestamp && writerID > existing.WriterID) {
		s.removeSet[element] = ElementRecord{Timestamp: timestamp, WriterID: writerID}
	}
	return nil
}

// Contains evaluates whether the element is present in the set according to LWW semantics.
func (s *LWWElementSet) Contains(element string) bool {
	addRec, hasAdd := s.addSet[element]
	if !hasAdd {
		return false
	}
	remRec, hasRem := s.removeSet[element]
	if !hasRem {
		return true
	}
	if addRec.Timestamp > remRec.Timestamp {
		return true
	}
	if addRec.Timestamp == remRec.Timestamp && addRec.WriterID >= remRec.WriterID {
		return true
	}
	return false
}

// Elements returns all currently active elements.
func (s *LWWElementSet) Elements() []string {
	var result []string
	for el := range s.addSet {
		if s.Contains(el) {
			result = append(result, el)
		}
	}
	return result
}

// Merge merges another LWWElementSet into s commutatively.
func (s *LWWElementSet) Merge(other *LWWElementSet) {
	if other == nil {
		return
	}
	for el, rec := range other.addSet {
		existing, ok := s.addSet[el]
		if !ok || rec.Timestamp > existing.Timestamp || (rec.Timestamp == existing.Timestamp && rec.WriterID > existing.WriterID) {
			s.addSet[el] = rec
		}
	}
	for el, rec := range other.removeSet {
		existing, ok := s.removeSet[el]
		if !ok || rec.Timestamp > existing.Timestamp || (rec.Timestamp == existing.Timestamp && rec.WriterID > existing.WriterID) {
			s.removeSet[el] = rec
		}
	}
}

// CRDTDelta represents an incremental state delta for low-bandwidth anti-entropy gossip.
type CRDTDelta struct {
	DocumentID string                   `json:"document_id"`
	Registers  map[string]LWWRegister   `json:"registers,omitempty"`
	Added      map[string]ElementRecord `json:"added,omitempty"`
	Removed    map[string]ElementRecord `json:"removed,omitempty"`
	Timestamp  int64                    `json:"timestamp"`
	SenderID   string                   `json:"sender_id"`
}

// Validate ensures delta fields satisfy invariant boundaries.
func (d *CRDTDelta) Validate() error {
	if d == nil {
		return ErrNilDelta
	}
	if err := strutil.ErrorOnBlank(d.DocumentID, "%w: document_id is required", ErrBlankID); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(d.SenderID, "%w: sender_id is required", ErrBlankID); err != nil {
		return err
	}
	if d.Timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	for k, reg := range d.Registers {
		if err := strutil.ErrorOnBlank(k, "%w: register key cannot be blank", ErrBlankID); err != nil {
			return err
		}
		if reg.Timestamp <= 0 {
			return ErrInvalidTimestamp
		}
	}
	for k, rec := range d.Added {
		if err := strutil.ErrorOnBlank(k, "%w: added key cannot be blank", ErrBlankID); err != nil {
			return err
		}
		if rec.Timestamp <= 0 {
			return ErrInvalidTimestamp
		}
	}
	for k, rec := range d.Removed {
		if err := strutil.ErrorOnBlank(k, "%w: removed key cannot be blank", ErrBlankID); err != nil {
			return err
		}
		if rec.Timestamp <= 0 {
			return ErrInvalidTimestamp
		}
	}
	return nil
}

// CRDTDocument encapsulates a composite CRDT state object with thread safety.
type CRDTDocument struct {
	mu         sync.RWMutex
	id         string
	registers  map[string]LWWRegister
	elementSet *LWWElementSet
	updatedAt  int64
}

// NewCRDTDocument creates a new CRDTDocument container.
func NewCRDTDocument(id string) (*CRDTDocument, error) {
	if err := strutil.ErrorOnBlank(id, "%w: document id is required", ErrBlankID); err != nil {
		return nil, err
	}
	return &CRDTDocument{
		id:         id,
		registers:  make(map[string]LWWRegister),
		elementSet: NewLWWElementSet(),
		updatedAt:  time.Now().UnixNano(),
	}, nil
}

// ID returns the document identifier.
func (doc *CRDTDocument) ID() string {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.id
}

// SetRegister updates a register value with a timestamp and writer ID.
func (doc *CRDTDocument) SetRegister(key, value string, timestamp int64, writerID string) error {
	if err := strutil.ErrorOnBlank(key, "%w: register key is required", ErrBlankID); err != nil {
		return err
	}
	reg, err := NewLWWRegister(value, timestamp, writerID)
	if err != nil {
		return err
	}

	doc.mu.Lock()
	defer doc.mu.Unlock()

	existing, exists := doc.registers[key]
	if !exists {
		doc.registers[key] = reg
	} else {
		doc.registers[key] = existing.Merge(reg)
	}
	if timestamp > doc.updatedAt {
		doc.updatedAt = timestamp
	}
	return nil
}

// GetRegister retrieves a register value if present.
func (doc *CRDTDocument) GetRegister(key string) (LWWRegister, bool) {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	reg, ok := doc.registers[key]
	return reg, ok
}

// AddElement adds an element to the internal LWW-Element-Set.
func (doc *CRDTDocument) AddElement(element string, timestamp int64, writerID string) error {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	if err := doc.elementSet.Add(element, timestamp, writerID); err != nil {
		return err
	}
	if timestamp > doc.updatedAt {
		doc.updatedAt = timestamp
	}
	return nil
}

// RemoveElement marks an element as removed in the internal LWW-Element-Set.
func (doc *CRDTDocument) RemoveElement(element string, timestamp int64, writerID string) error {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	if err := doc.elementSet.Remove(element, timestamp, writerID); err != nil {
		return err
	}
	if timestamp > doc.updatedAt {
		doc.updatedAt = timestamp
	}
	return nil
}

// HasElement checks if an element is currently in the set.
func (doc *CRDTDocument) HasElement(element string) bool {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.elementSet.Contains(element)
}

// Elements returns a slice of currently active elements.
func (doc *CRDTDocument) Elements() []string {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.elementSet.Elements()
}

// ApplyDelta applies a delta update validating boundary conditions.
func (doc *CRDTDocument) ApplyDelta(delta *CRDTDelta) error {
	if err := delta.Validate(); err != nil {
		return err
	}

	doc.mu.Lock()
	defer doc.mu.Unlock()

	if delta.DocumentID != doc.id {
		return fmt.Errorf("%w: delta document_id %q does not match document %q", ErrCorruptPayload, delta.DocumentID, doc.id)
	}

	for k, reg := range delta.Registers {
		existing, exists := doc.registers[k]
		if !exists {
			doc.registers[k] = reg
		} else {
			doc.registers[k] = existing.Merge(reg)
		}
	}

	for k, rec := range delta.Added {
		_ = doc.elementSet.Add(k, rec.Timestamp, rec.WriterID)
	}
	for k, rec := range delta.Removed {
		_ = doc.elementSet.Remove(k, rec.Timestamp, rec.WriterID)
	}

	if delta.Timestamp > doc.updatedAt {
		doc.updatedAt = delta.Timestamp
	}
	return nil
}

// Merge converges another CRDTDocument state into this document.
func (doc *CRDTDocument) Merge(other *CRDTDocument) error {
	if other == nil {
		return nil
	}

	other.mu.RLock()
	otherID := other.id
	otherRegs := make(map[string]LWWRegister, len(other.registers))
	for k, v := range other.registers {
		otherRegs[k] = v
	}
	otherSetCopy := &LWWElementSet{
		addSet:    make(map[string]ElementRecord, len(other.elementSet.addSet)),
		removeSet: make(map[string]ElementRecord, len(other.elementSet.removeSet)),
	}
	for k, v := range other.elementSet.addSet {
		otherSetCopy.addSet[k] = v
	}
	for k, v := range other.elementSet.removeSet {
		otherSetCopy.removeSet[k] = v
	}
	otherUpdated := other.updatedAt
	other.mu.RUnlock()

	doc.mu.Lock()
	defer doc.mu.Unlock()

	if otherID != doc.id {
		return fmt.Errorf("%w: cannot merge documents with different IDs (%s vs %s)", ErrCorruptPayload, doc.id, otherID)
	}

	for k, reg := range otherRegs {
		existing, exists := doc.registers[k]
		if !exists {
			doc.registers[k] = reg
		} else {
			doc.registers[k] = existing.Merge(reg)
		}
	}

	doc.elementSet.Merge(otherSetCopy)
	if otherUpdated > doc.updatedAt {
		doc.updatedAt = otherUpdated
	}
	return nil
}
