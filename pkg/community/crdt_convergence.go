package community

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errBlankDocIDMsg      = "document id is required"
	errBlankWriterIDMsg   = "writer_id is required"
	errBlankSenderIDMsg   = "sender_id is required"
	errBlankElementMsg    = "element is required"
	errBlankRegKeyMsg     = "register key is required"
	errBlankNodeIDMsg     = "node_id is required"
	digestDomainRegister  = "reg"
	digestDomainAdd       = "add"
	digestDomainRemove    = "rem"
	digestDomainVecClock  = "vc"
	digestDomainDeltaHead = "crdt-delta-v1"
	digestDomainStateHead = "crdt-state-v1"
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
	// ErrDigestMismatch is returned when a delta digest is missing or fails verification.
	ErrDigestMismatch = errors.New("crdt: delta digest mismatch")
)

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func crdtWriteHashBytes(h hash.Hash, b []byte) {
	n, err := h.Write(b)
	if err != nil || n != len(b) {
		panic(fmt.Sprintf("crdt: sha256 write failed: %v", err))
	}
}

func crdtWriteHashString(h hash.Hash, s string) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(s)))
	crdtWriteHashBytes(h, lenBuf[:])
	crdtWriteHashBytes(h, []byte(s))
}

func crdtWriteHashInt64(h hash.Hash, v int64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	crdtWriteHashBytes(h, buf[:])
}

func validateVectorClockMap(vc map[string]uint64) error {
	for nodeID, counter := range vc {
		if strutil.IsBlank(nodeID) {
			return fmt.Errorf("%w: %s", ErrBlankID, errBlankNodeIDMsg)
		}
		if counter == 0 {
			return ErrInvalidTimestamp
		}
	}
	return nil
}

// LWWRegister represents a Last-Write-Wins scalar register.
type LWWRegister struct {
	Value       string `json:"value"`
	Timestamp   int64  `json:"timestamp"`
	WriterID    string `json:"writer_id"`
	ValueDigest string `json:"value_digest"`
}

// NewLWWRegister constructs a valid LWWRegister with validation and deterministic value digest.
func NewLWWRegister(value string, timestamp int64, writerID string) (LWWRegister, error) {
	if strutil.IsBlank(writerID) {
		return LWWRegister{}, fmt.Errorf("%w: %s", ErrBlankID, errBlankWriterIDMsg)
	}
	if timestamp <= 0 {
		return LWWRegister{}, ErrInvalidTimestamp
	}
	return LWWRegister{
		Value:       value,
		Timestamp:   timestamp,
		WriterID:    writerID,
		ValueDigest: sha256Hex(value),
	}, nil
}

func (r LWWRegister) effectiveDigest() string {
	if r.ValueDigest != "" {
		return r.ValueDigest
	}
	return sha256Hex(r.Value)
}

// Merge returns a new LWWRegister merging r and other using deterministic
// tie-breaking on (Timestamp, WriterID, ValueDigest).
func (r LWWRegister) Merge(other LWWRegister) LWWRegister {
	if other.Timestamp != r.Timestamp {
		if other.Timestamp > r.Timestamp {
			return other
		}
		return r
	}
	if other.WriterID != r.WriterID {
		if other.WriterID > r.WriterID {
			return other
		}
		return r
	}
	if other.effectiveDigest() > r.effectiveDigest() {
		return other
	}
	return r
}

// ElementRecord tracks an element timestamp and writer for LWW-Element-Set.
type ElementRecord struct {
	Timestamp int64  `json:"timestamp"`
	WriterID  string `json:"writer_id"`
}

func shouldReplaceRecord(existing ElementRecord, hasExisting bool, candidate ElementRecord) bool {
	if !hasExisting {
		return true
	}
	if candidate.Timestamp != existing.Timestamp {
		return candidate.Timestamp > existing.Timestamp
	}
	return candidate.WriterID > existing.WriterID
}

func validateElementArgs(element string, timestamp int64, writerID string) error {
	if strutil.IsBlank(element) {
		return fmt.Errorf("%w: %s", ErrBlankID, errBlankElementMsg)
	}
	if strutil.IsBlank(writerID) {
		return fmt.Errorf("%w: %s", ErrBlankID, errBlankWriterIDMsg)
	}
	if timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	return nil
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

func applyElementOp(target map[string]ElementRecord, element string, timestamp int64, writerID string) error {
	if err := validateElementArgs(element, timestamp, writerID); err != nil {
		return err
	}
	candidate := ElementRecord{Timestamp: timestamp, WriterID: writerID}
	existing, ok := target[element]
	if shouldReplaceRecord(existing, ok, candidate) {
		target[element] = candidate
	}
	return nil
}

// Add inserts or updates an element in the add set.
func (s *LWWElementSet) Add(element string, timestamp int64, writerID string) error {
	return applyElementOp(s.addSet, element, timestamp, writerID)
}

// Remove marks an element as removed in the remove set.
func (s *LWWElementSet) Remove(element string, timestamp int64, writerID string) error {
	return applyElementOp(s.removeSet, element, timestamp, writerID)
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
	if addRec.Timestamp != remRec.Timestamp {
		return addRec.Timestamp > remRec.Timestamp
	}
	return addRec.WriterID >= remRec.WriterID
}

// Elements returns all currently active elements in sorted order.
func (s *LWWElementSet) Elements() []string {
	var result []string
	for el := range s.addSet {
		if s.Contains(el) {
			result = append(result, el)
		}
	}
	sort.Strings(result)
	return result
}

func mergeRecordMap(dst, src map[string]ElementRecord) {
	for el, rec := range src {
		existing, ok := dst[el]
		if shouldReplaceRecord(existing, ok, rec) {
			dst[el] = rec
		}
	}
}

// Merge merges another LWWElementSet into s commutatively.
func (s *LWWElementSet) Merge(other *LWWElementSet) {
	if other == nil {
		return
	}
	mergeRecordMap(s.addSet, other.addSet)
	mergeRecordMap(s.removeSet, other.removeSet)
}

// CRDTDelta represents an incremental state delta for low-bandwidth anti-entropy gossip.
type CRDTDelta struct {
	ClusterID   string                   `json:"cluster_id"`
	DocumentID  string                   `json:"document_id"`
	SenderID    string                   `json:"sender_id"`
	Timestamp   int64                    `json:"timestamp"`
	VectorClock map[string]uint64        `json:"vector_clock,omitempty"`
	Registers   map[string]LWWRegister   `json:"registers,omitempty"`
	Added       map[string]ElementRecord `json:"added,omitempty"`
	Removed     map[string]ElementRecord `json:"removed,omitempty"`
	Digest      string                   `json:"digest"`
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func hashRegisters(h hash.Hash, regs map[string]LWWRegister) {
	for _, k := range sortedKeys(regs) {
		reg := regs[k]
		crdtWriteHashString(h, digestDomainRegister)
		crdtWriteHashString(h, k)
		crdtWriteHashString(h, reg.Value)
		crdtWriteHashInt64(h, reg.Timestamp)
		crdtWriteHashString(h, reg.WriterID)
		crdtWriteHashString(h, reg.effectiveDigest())
	}
}

func hashElementRecords(h hash.Hash, domain string, recs map[string]ElementRecord) {
	for _, k := range sortedKeys(recs) {
		rec := recs[k]
		crdtWriteHashString(h, domain)
		crdtWriteHashString(h, k)
		crdtWriteHashInt64(h, rec.Timestamp)
		crdtWriteHashString(h, rec.WriterID)
	}
}

func hashVectorClockMap(h hash.Hash, vc map[string]uint64) {
	for _, nodeID := range sortedKeys(vc) {
		crdtWriteHashString(h, digestDomainVecClock)
		crdtWriteHashString(h, nodeID)
		crdtWriteHashInt64(h, int64(vc[nodeID]))
	}
}

// ComputeDigest calculates the deterministic SHA-256 digest of the delta payload.
func (d *CRDTDelta) ComputeDigest() string {
	h := sha256.New()
	crdtWriteHashString(h, digestDomainDeltaHead)
	crdtWriteHashString(h, d.ClusterID)
	crdtWriteHashString(h, d.DocumentID)
	crdtWriteHashString(h, d.SenderID)
	crdtWriteHashInt64(h, d.Timestamp)
	hashVectorClockMap(h, d.VectorClock)
	hashRegisters(h, d.Registers)
	hashElementRecords(h, digestDomainAdd, d.Added)
	hashElementRecords(h, digestDomainRemove, d.Removed)
	return hex.EncodeToString(h.Sum(nil))
}

// Seal computes and assigns the canonical SHA-256 Digest on the delta.
func (d *CRDTDelta) Seal() {
	if d != nil {
		d.Digest = d.ComputeDigest()
	}
}

func validateElementRecordMap(records map[string]ElementRecord) error {
	for k, rec := range records {
		if err := validateElementArgs(k, rec.Timestamp, rec.WriterID); err != nil {
			return err
		}
	}
	return nil
}

// Validate ensures delta fields and digest satisfy invariant boundaries.
func (d *CRDTDelta) Validate() error {
	if d == nil {
		return ErrNilDelta
	}
	if strutil.IsBlank(d.ClusterID) {
		return ErrBlankClusterID
	}
	if strutil.IsBlank(d.DocumentID) {
		return fmt.Errorf("%w: %s", ErrBlankID, errBlankDocIDMsg)
	}
	if strutil.IsBlank(d.SenderID) {
		return fmt.Errorf("%w: %s", ErrBlankID, errBlankSenderIDMsg)
	}
	if d.Timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	if err := validateVectorClockMap(d.VectorClock); err != nil {
		return err
	}
	for k, reg := range d.Registers {
		if strutil.IsBlank(k) {
			return fmt.Errorf("%w: %s", ErrBlankID, errBlankRegKeyMsg)
		}
		if strutil.IsBlank(reg.WriterID) {
			return fmt.Errorf("%w: %s", ErrBlankID, errBlankWriterIDMsg)
		}
		if reg.Timestamp <= 0 {
			return ErrInvalidTimestamp
		}
	}
	if err := validateElementRecordMap(d.Added); err != nil {
		return err
	}
	if err := validateElementRecordMap(d.Removed); err != nil {
		return err
	}
	if strutil.IsBlank(d.Digest) || d.Digest != d.ComputeDigest() {
		return ErrDigestMismatch
	}
	return nil
}

// CRDTDocument encapsulates a composite CRDT state object with thread safety.
type CRDTDocument struct {
	mu          sync.RWMutex
	clusterID   string
	id          string
	registers   map[string]LWWRegister
	elementSet  *LWWElementSet
	vectorClock *VectorClock
	updatedAt   int64
}

// NewCRDTDocument creates a new CRDTDocument container bound to a cluster and document ID.
func NewCRDTDocument(clusterID, id string) (*CRDTDocument, error) {
	if strutil.IsBlank(clusterID) {
		return nil, ErrBlankClusterID
	}
	if strutil.IsBlank(id) {
		return nil, fmt.Errorf("%w: %s", ErrBlankID, errBlankDocIDMsg)
	}
	return &CRDTDocument{
		clusterID:   clusterID,
		id:          id,
		registers:   make(map[string]LWWRegister),
		elementSet:  NewLWWElementSet(),
		vectorClock: NewVectorClock(),
	}, nil
}

// ClusterID returns the cluster namespace identifier.
func (doc *CRDTDocument) ClusterID() string {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.clusterID
}

// ID returns the document identifier.
func (doc *CRDTDocument) ID() string {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.id
}

// VectorClock returns a snapshot copy of the document vector clock.
func (doc *CRDTDocument) VectorClock() *VectorClock {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.vectorClock.Clone()
}

// SetRegister updates a register value with a timestamp and writer ID.
func (doc *CRDTDocument) SetRegister(key, value string, timestamp int64, writerID string) error {
	if strutil.IsBlank(key) {
		return fmt.Errorf("%w: %s", ErrBlankID, errBlankRegKeyMsg)
	}
	reg, err := NewLWWRegister(value, timestamp, writerID)
	if err != nil {
		return err
	}

	doc.mu.Lock()
	defer doc.mu.Unlock()

	if err := doc.vectorClock.Tick(writerID); err != nil {
		return err
	}
	doc.mergeRegisterLocked(key, reg)
	if timestamp > doc.updatedAt {
		doc.updatedAt = timestamp
	}
	return nil
}

func (doc *CRDTDocument) mergeRegisterLocked(key string, reg LWWRegister) {
	existing, exists := doc.registers[key]
	if !exists {
		doc.registers[key] = reg
		return
	}
	doc.registers[key] = existing.Merge(reg)
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
	return doc.mutateElement(element, timestamp, writerID, doc.elementSet.Add)
}

// RemoveElement marks an element as removed in the internal LWW-Element-Set.
func (doc *CRDTDocument) RemoveElement(element string, timestamp int64, writerID string) error {
	return doc.mutateElement(element, timestamp, writerID, doc.elementSet.Remove)
}

func (doc *CRDTDocument) mutateElement(
	element string,
	timestamp int64,
	writerID string,
	apply func(string, int64, string) error,
) error {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	if err := apply(element, timestamp, writerID); err != nil {
		return err
	}
	if err := doc.vectorClock.Tick(writerID); err != nil {
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

// Elements returns a sorted slice of currently active elements.
func (doc *CRDTDocument) Elements() []string {
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	return doc.elementSet.Elements()
}

// StateDigest returns a deterministic SHA-256 digest of the converged CRDT state.
func (doc *CRDTDocument) StateDigest() string {
	doc.mu.RLock()
	defer doc.mu.RUnlock()

	h := sha256.New()
	crdtWriteHashString(h, digestDomainStateHead)
	crdtWriteHashString(h, doc.clusterID)
	crdtWriteHashString(h, doc.id)
	hashVectorClockMap(h, doc.vectorClock.Map())
	hashRegisters(h, doc.registers)
	hashElementRecords(h, digestDomainAdd, doc.elementSet.addSet)
	hashElementRecords(h, digestDomainRemove, doc.elementSet.removeSet)
	return hex.EncodeToString(h.Sum(nil))
}

func cloneRecordMap(src map[string]ElementRecord) map[string]ElementRecord {
	dst := make(map[string]ElementRecord, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// BuildDelta constructs a sealed CRDTDelta representing the current document state.
func (doc *CRDTDocument) BuildDelta(senderID string, timestamp int64) (*CRDTDelta, error) {
	if strutil.IsBlank(senderID) {
		return nil, fmt.Errorf("%w: %s", ErrBlankID, errBlankSenderIDMsg)
	}
	if timestamp <= 0 {
		return nil, ErrInvalidTimestamp
	}

	doc.mu.RLock()
	regs := make(map[string]LWWRegister, len(doc.registers))
	for k, v := range doc.registers {
		regs[k] = v
	}
	delta := &CRDTDelta{
		ClusterID:   doc.clusterID,
		DocumentID:  doc.id,
		SenderID:    senderID,
		Timestamp:   timestamp,
		VectorClock: doc.vectorClock.Map(),
		Registers:   regs,
		Added:       cloneRecordMap(doc.elementSet.addSet),
		Removed:     cloneRecordMap(doc.elementSet.removeSet),
	}
	doc.mu.RUnlock()

	delta.Seal()
	return delta, nil
}

// ApplyDelta atomically validates and applies a delta update to the document.
func (doc *CRDTDocument) ApplyDelta(delta *CRDTDelta) error {
	if err := delta.Validate(); err != nil {
		return err
	}

	doc.mu.Lock()
	defer doc.mu.Unlock()

	if delta.ClusterID != doc.clusterID {
		return fmt.Errorf("%w: delta cluster %q does not match %q", ErrClusterMismatch, delta.ClusterID, doc.clusterID)
	}
	if delta.DocumentID != doc.id {
		return fmt.Errorf("%w: delta document_id %q does not match document %q", ErrCorruptPayload, delta.DocumentID, doc.id)
	}

	for k, reg := range delta.Registers {
		doc.mergeRegisterLocked(k, reg)
	}
	mergeRecordMap(doc.elementSet.addSet, delta.Added)
	mergeRecordMap(doc.elementSet.removeSet, delta.Removed)
	if len(delta.VectorClock) > 0 {
		if err := doc.vectorClock.Merge(NewVectorClockFromMap(delta.VectorClock)); err != nil {
			return err
		}
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
	otherClusterID := other.clusterID
	otherID := other.id
	otherRegs := make(map[string]LWWRegister, len(other.registers))
	for k, v := range other.registers {
		otherRegs[k] = v
	}
	otherSetCopy := &LWWElementSet{
		addSet:    cloneRecordMap(other.elementSet.addSet),
		removeSet: cloneRecordMap(other.elementSet.removeSet),
	}
	otherVC := other.vectorClock.Clone()
	otherUpdated := other.updatedAt
	other.mu.RUnlock()

	doc.mu.Lock()
	defer doc.mu.Unlock()

	if otherClusterID != doc.clusterID {
		return fmt.Errorf("%w: cannot merge cluster %q into %q", ErrClusterMismatch, otherClusterID, doc.clusterID)
	}
	if otherID != doc.id {
		return fmt.Errorf("%w: cannot merge documents with different IDs (%s vs %s)", ErrCorruptPayload, doc.id, otherID)
	}

	for k, reg := range otherRegs {
		doc.mergeRegisterLocked(k, reg)
	}
	doc.elementSet.Merge(otherSetCopy)
	if err := doc.vectorClock.Merge(otherVC); err != nil {
		return err
	}
	if otherUpdated > doc.updatedAt {
		doc.updatedAt = otherUpdated
	}
	return nil
}
