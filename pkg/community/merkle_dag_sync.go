package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const sha256HexLength = 64

var (
	// ErrBlankRegionID indicates an empty or whitespace region identifier.
	ErrBlankRegionID = errors.New("region ID cannot be blank")

	// ErrBlankMerkleNodeID indicates an empty or whitespace Merkle DAG node identifier.
	ErrBlankMerkleNodeID = errors.New("merkle DAG node ID cannot be blank")

	// ErrInvalidKeyRange indicates a blank or inverted range proof key boundary.
	ErrInvalidKeyRange = errors.New("range proof start key must be non-blank and <= end key")

	// ErrInvalidSHA256Digest indicates a malformed or non-hex SHA-256 digest string.
	ErrInvalidSHA256Digest = errors.New("invalid SHA-256 hex digest")

	// ErrInvalidRangeProof indicates a structurally invalid or unverifiable range proof.
	ErrInvalidRangeProof = errors.New("invalid cryptographic range proof")

	// ErrTamperedMerkleRoot indicates a Merkle root or node digest mismatch.
	ErrTamperedMerkleRoot = errors.New("tampered Merkle root or node digest")

	// ErrMissingCausalAncestor indicates a DAG node references an unknown causal parent.
	ErrMissingCausalAncestor = errors.New("missing causal ancestor in Merkle DAG")

	// ErrNodeQuarantined indicates a region or peer branch has been quarantined for Byzantine behavior.
	ErrNodeQuarantined = errors.New("region branch is quarantined")
)

// MerkleDAGNode represents an immutable content-addressed vertex in a planetary Merkle DAG.
type MerkleDAGNode struct {
	NodeID       string    `json:"node_id"`
	RegionID     string    `json:"region_id"`
	Sequence     uint64    `json:"sequence"`
	ParentHashes []string  `json:"parent_hashes,omitempty"`
	ContentHash  string    `json:"content_hash"`
	MerkleHash   string    `json:"merkle_hash"`
	Payload      []byte    `json:"payload,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ComputeMerkleNodeHash deterministically computes the SHA-256 Merkle digest of a DAG node.
func ComputeMerkleNodeHash(nodeID, regionID string, seq uint64, contentHash string, parentHashes []string) string {
	sortedParents := append([]string(nil), parentHashes...)
	sort.Strings(sortedParents)

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|%s", regionID, nodeID, seq, contentHash)
	for _, p := range sortedParents {
		_, _ = fmt.Fprintf(h, "|%s", p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeSHA256Hex returns the lowercase hexadecimal SHA-256 digest of data.
func ComputeSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isValidSHA256Hex(digest string) bool {
	if len(digest) != sha256HexLength {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func validateDigestSlice(digests []string, fieldName string) error {
	for i, d := range digests {
		if !isValidSHA256Hex(d) {
			return fmt.Errorf("%w: %s[%d]", ErrInvalidSHA256Digest, fieldName, i)
		}
	}
	return nil
}

// Validate enforces static floor invariants on a MerkleDAGNode.
func (n MerkleDAGNode) Validate() error {
	if err := strutil.ErrorOnBlank(n.RegionID, "%w", ErrBlankRegionID); err != nil {
		return err
	}
	if err := strutil.ErrorOnBlank(n.NodeID, "%w", ErrBlankMerkleNodeID); err != nil {
		return err
	}
	if !isValidSHA256Hex(n.ContentHash) {
		return fmt.Errorf("%w: content_hash", ErrInvalidSHA256Digest)
	}
	if !isValidSHA256Hex(n.MerkleHash) {
		return fmt.Errorf("%w: merkle_hash", ErrInvalidSHA256Digest)
	}
	if err := validateDigestSlice(n.ParentHashes, "parent_hashes"); err != nil {
		return err
	}
	expected := ComputeMerkleNodeHash(n.NodeID, n.RegionID, n.Sequence, n.ContentHash, n.ParentHashes)
	if n.MerkleHash != expected {
		return fmt.Errorf("%w: node %s expected %s, got %s", ErrTamperedMerkleRoot, n.NodeID, expected, n.MerkleHash)
	}
	return nil
}

// RangeProof encodes a verifiable Merkle range proof over [StartKey, EndKey].
type RangeProof struct {
	RegionID     string   `json:"region_id"`
	StartKey     string   `json:"start_key"`
	EndKey       string   `json:"end_key"`
	ExpectedRoot string   `json:"expected_root"`
	LeafHashes   []string `json:"leaf_hashes"`
	BoundaryPath []string `json:"boundary_path,omitempty"`
}

// Validate enforces static floor invariants on a RangeProof.
func (p RangeProof) Validate() error {
	if err := strutil.ErrorOnBlank(p.RegionID, "%w", ErrBlankRegionID); err != nil {
		return err
	}
	if strutil.IsBlank(p.StartKey) || strutil.IsBlank(p.EndKey) || p.StartKey > p.EndKey {
		return ErrInvalidKeyRange
	}
	if !isValidSHA256Hex(p.ExpectedRoot) {
		return fmt.Errorf("%w: expected_root", ErrInvalidSHA256Digest)
	}
	if len(p.LeafHashes) == 0 {
		return fmt.Errorf("%w: leaf_hashes cannot be empty", ErrInvalidRangeProof)
	}
	if err := validateDigestSlice(p.LeafHashes, "leaf_hashes"); err != nil {
		return err
	}
	if err := validateDigestSlice(p.BoundaryPath, "boundary_path"); err != nil {
		return err
	}
	return nil
}

// RegionState summarizes the current Merkle DAG synchronization state of a region.
type RegionState struct {
	RegionID         string    `json:"region_id"`
	RootHash         string    `json:"root_hash"`
	HeadNodeID       string    `json:"head_node_id"`
	NodeCount        int       `json:"node_count"`
	Quarantined      bool      `json:"quarantined"`
	QuarantineReason string    `json:"quarantine_reason,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func merkleHashPair(left, right string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s", left, right)
	return hex.EncodeToString(h.Sum(nil))
}

func computeMerkleTreeRoot(leaves []string) string {
	if len(leaves) == 0 {
		return ComputeSHA256Hex(nil)
	}
	level := append([]string(nil), leaves...)
	for len(level) > 1 {
		next := make([]string, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 < len(level) {
				next = append(next, merkleHashPair(level[i], level[i+1]))
			} else {
				next = append(next, merkleHashPair(level[i], level[i]))
			}
		}
		level = next
	}
	return level[0]
}

// BuildRangeProof constructs a verifiable RangeProof for a bounded key range and leaf hashes.
func BuildRangeProof(regionID, startKey, endKey string, leafHashes []string) (RangeProof, error) {
	if err := strutil.ErrorOnBlank(regionID, "%w", ErrBlankRegionID); err != nil {
		return RangeProof{}, err
	}
	if strutil.IsBlank(startKey) || strutil.IsBlank(endKey) || startKey > endKey {
		return RangeProof{}, ErrInvalidKeyRange
	}
	if len(leafHashes) == 0 {
		return RangeProof{}, fmt.Errorf("%w: leaf_hashes cannot be empty", ErrInvalidRangeProof)
	}
	if err := validateDigestSlice(leafHashes, "leaf_hashes"); err != nil {
		return RangeProof{}, err
	}

	subRoot := computeMerkleTreeRoot(leafHashes)
	boundaryTag := ComputeSHA256Hex([]byte(regionID + "|" + startKey + "|" + endKey))
	expectedRoot := merkleHashPair(subRoot, boundaryTag)

	return RangeProof{
		RegionID:     regionID,
		StartKey:     startKey,
		EndKey:       endKey,
		ExpectedRoot: expectedRoot,
		LeafHashes:   append([]string(nil), leafHashes...),
		BoundaryPath: []string{boundaryTag},
	}, nil
}

// VerifyRangeProof verifies structural validity and cryptographic root integrity of a RangeProof.
func VerifyRangeProof(proof RangeProof) error {
	if err := proof.Validate(); err != nil {
		return err
	}
	if len(proof.BoundaryPath) == 0 {
		return fmt.Errorf("%w: missing boundary path", ErrInvalidRangeProof)
	}
	expectedBoundaryTag := ComputeSHA256Hex([]byte(proof.RegionID + "|" + proof.StartKey + "|" + proof.EndKey))
	if proof.BoundaryPath[0] != expectedBoundaryTag {
		return fmt.Errorf("%w: boundary tag mismatch", ErrInvalidRangeProof)
	}

	acc := computeMerkleTreeRoot(proof.LeafHashes)
	for _, sibling := range proof.BoundaryPath {
		acc = merkleHashPair(acc, sibling)
	}
	if acc != proof.ExpectedRoot {
		return fmt.Errorf("%w: expected %s, computed %s", ErrTamperedMerkleRoot, proof.ExpectedRoot, acc)
	}
	return nil
}

// MerkleDAGSynchronizer manages multi-region Merkle DAG state, differential traversal,
// range proof verification, causal ancestor resolution, and Byzantine branch quarantine.
type MerkleDAGSynchronizer struct {
	mu            sync.RWMutex
	localRegionID string
	nodesByHash   map[string]MerkleDAGNode
	childrenIndex map[string]map[string]struct{}
	regions       map[string]*RegionState
}

// NewMerkleDAGSynchronizer creates a new multi-region Merkle DAG synchronizer.
func NewMerkleDAGSynchronizer(localRegionID string) (*MerkleDAGSynchronizer, error) {
	if err := strutil.ErrorOnBlank(localRegionID, "%w", ErrBlankRegionID); err != nil {
		return nil, err
	}
	return &MerkleDAGSynchronizer{
		localRegionID: localRegionID,
		nodesByHash:   make(map[string]MerkleDAGNode),
		childrenIndex: make(map[string]map[string]struct{}),
		regions:       make(map[string]*RegionState),
	}, nil
}

func (s *MerkleDAGSynchronizer) ensureRegionLocked(regionID string) *RegionState {
	st, ok := s.regions[regionID]
	if !ok {
		st = &RegionState{
			RegionID:  regionID,
			UpdatedAt: time.Now().UTC(),
		}
		s.regions[regionID] = st
	}
	return st
}

func (s *MerkleDAGSynchronizer) quarantineLocked(regionID, reason string) {
	st := s.ensureRegionLocked(regionID)
	st.Quarantined = true
	st.QuarantineReason = reason
	st.UpdatedAt = time.Now().UTC()
}

// QuarantineRegion explicitly quarantines a peer region branch.
func (s *MerkleDAGSynchronizer) QuarantineRegion(regionID, reason string) error {
	if err := strutil.ErrorOnBlank(regionID, "%w", ErrBlankRegionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quarantineLocked(regionID, reason)
	return nil
}

// RegionState returns a snapshot of the synchronization state for regionID.
func (s *MerkleDAGSynchronizer) RegionState(regionID string) (RegionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.regions[regionID]
	if !ok {
		return RegionState{}, false
	}
	return *st, true
}

// AppendNode validates and appends a single MerkleDAGNode, enforcing causal ancestor presence.
func (s *MerkleDAGSynchronizer) AppendNode(node MerkleDAGNode) error {
	if err := node.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.ensureRegionLocked(node.RegionID)
	if st.Quarantined {
		return fmt.Errorf("%w: region %s (%s)", ErrNodeQuarantined, node.RegionID, st.QuarantineReason)
	}
	if _, exists := s.nodesByHash[node.MerkleHash]; exists {
		return nil
	}

	for _, parentHash := range node.ParentHashes {
		if _, ok := s.nodesByHash[parentHash]; !ok {
			s.quarantineLocked(node.RegionID, "missing causal ancestor "+parentHash)
			return fmt.Errorf("%w: parent %s for node %s", ErrMissingCausalAncestor, parentHash, node.NodeID)
		}
	}

	s.insertNodeLocked(node, st)
	s.reRootRegionLocked(node.RegionID, st)
	return nil
}

func (s *MerkleDAGSynchronizer) insertNodeLocked(node MerkleDAGNode, st *RegionState) {
	copied := node
	if len(node.Payload) > 0 {
		copied.Payload = append([]byte(nil), node.Payload...)
	}
	copied.ParentHashes = append([]string(nil), node.ParentHashes...)
	s.nodesByHash[copied.MerkleHash] = copied

	for _, parentHash := range copied.ParentHashes {
		children, ok := s.childrenIndex[parentHash]
		if !ok {
			children = make(map[string]struct{})
			s.childrenIndex[parentHash] = children
		}
		children[copied.MerkleHash] = struct{}{}
	}

	st.NodeCount++
	st.HeadNodeID = copied.NodeID
	st.UpdatedAt = time.Now().UTC()
}

// reRootRegionLocked deterministically computes the region's Merkle root across all active DAG tips
// so concurrent region syncs converge to identical roots regardless of node arrival order.
func (s *MerkleDAGSynchronizer) reRootRegionLocked(regionID string, st *RegionState) {
	var tips []string
	for hash, node := range s.nodesByHash {
		if node.RegionID != regionID {
			continue
		}
		if len(s.childrenIndex[hash]) == 0 {
			tips = append(tips, hash)
		}
	}
	sort.Strings(tips)
	st.RootHash = computeMerkleTreeRoot(tips)
}

// ReRootRegion recomputes and returns the deterministic Merkle root for regionID.
func (s *MerkleDAGSynchronizer) ReRootRegion(regionID string) (string, error) {
	if err := strutil.ErrorOnBlank(regionID, "%w", ErrBlankRegionID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.ensureRegionLocked(regionID)
	s.reRootRegionLocked(regionID, st)
	return st.RootHash, nil
}

// SyncDelta verifies a peer region's range proof, resolves causal ancestors across the incoming
// batch, appends unseen nodes, and deterministically re-roots the region branch.
// Any invalid proof, tampered digest, or missing causal ancestor quarantines sourceRegionID.
func (s *MerkleDAGSynchronizer) SyncDelta(sourceRegionID string, nodes []MerkleDAGNode, proof RangeProof) ([]MerkleDAGNode, error) {
	if err := strutil.ErrorOnBlank(sourceRegionID, "%w", ErrBlankRegionID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.ensureRegionLocked(sourceRegionID)
	if st.Quarantined {
		return nil, fmt.Errorf("%w: region %s (%s)", ErrNodeQuarantined, sourceRegionID, st.QuarantineReason)
	}

	if err := VerifyRangeProof(proof); err != nil {
		s.quarantineLocked(sourceRegionID, err.Error())
		return nil, err
	}
	if proof.RegionID != sourceRegionID {
		err := fmt.Errorf("%w: proof region %s does not match source %s", ErrInvalidRangeProof, proof.RegionID, sourceRegionID)
		s.quarantineLocked(sourceRegionID, err.Error())
		return nil, err
	}

	ordered := append([]MerkleDAGNode(nil), nodes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Sequence != ordered[j].Sequence {
			return ordered[i].Sequence < ordered[j].Sequence
		}
		return ordered[i].MerkleHash < ordered[j].MerkleHash
	})

	batchHashes := make(map[string]struct{}, len(ordered))
	for _, n := range ordered {
		if err := n.Validate(); err != nil {
			s.quarantineLocked(sourceRegionID, err.Error())
			return nil, err
		}
		batchHashes[n.MerkleHash] = struct{}{}
	}

	for _, n := range ordered {
		for _, p := range n.ParentHashes {
			_, inStore := s.nodesByHash[p]
			_, inBatch := batchHashes[p]
			if !inStore && !inBatch {
				s.quarantineLocked(sourceRegionID, "missing causal ancestor "+p)
				return nil, fmt.Errorf("%w: parent %s for node %s", ErrMissingCausalAncestor, p, n.NodeID)
			}
		}
	}

	var applied []MerkleDAGNode
	for _, n := range ordered {
		if _, exists := s.nodesByHash[n.MerkleHash]; exists {
			continue
		}
		s.insertNodeLocked(n, st)
		applied = append(applied, n)
	}
	s.reRootRegionLocked(sourceRegionID, st)
	return applied, nil
}

// ComputeDifferentialNodes returns all nodes for regionID whose MerkleHash is not in knownHashes,
// ordered deterministically by sequence and hash.
func (s *MerkleDAGSynchronizer) ComputeDifferentialNodes(regionID string, knownHashes map[string]struct{}) ([]MerkleDAGNode, error) {
	if err := strutil.ErrorOnBlank(regionID, "%w", ErrBlankRegionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var delta []MerkleDAGNode
	for hash, node := range s.nodesByHash {
		if node.RegionID != regionID {
			continue
		}
		if _, known := knownHashes[hash]; !known {
			delta = append(delta, node)
		}
	}
	sort.Slice(delta, func(i, j int) bool {
		if delta[i].Sequence != delta[j].Sequence {
			return delta[i].Sequence < delta[j].Sequence
		}
		return delta[i].MerkleHash < delta[j].MerkleHash
	})
	return delta, nil
}
