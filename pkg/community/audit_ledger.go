package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// AuditEntry records an atomic execution event within the swarm.
type AuditEntry struct {
	EntryID   string    `json:"entry_id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// AuditBlock encapsulates a cryptographically linked batch of audit entries.
type AuditBlock struct {
	Index        uint64       `json:"index"`
	PreviousHash string       `json:"previous_hash"`
	MerkleRoot   string       `json:"merkle_root"`
	Entries      []AuditEntry `json:"entries"`
	Timestamp    time.Time    `json:"timestamp"`
}

// AuditPathElement contains a sibling hash and direction for Merkle proof traversal.
type AuditPathElement struct {
	Hash    string `json:"hash"`
	IsRight bool   `json:"is_right"`
}

// InclusionProof verifies the existence of an entry within a block's Merkle tree.
type InclusionProof struct {
	BlockIndex uint64             `json:"block_index"`
	EntryHash  string             `json:"entry_hash"`
	MerkleRoot string             `json:"merkle_root"`
	AuditPath  []AuditPathElement `json:"audit_path"`
}

// AuditLedger manages an append-only verifiable cryptographic ledger.
type AuditLedger struct {
	mu     sync.RWMutex
	blocks []AuditBlock
}

// NewAuditLedger constructs an empty audit ledger.
func NewAuditLedger() *AuditLedger {
	return &AuditLedger{
		blocks: make([]AuditBlock, 0),
	}
}

// BlockCount returns the total number of committed blocks in the ledger.
func (l *AuditLedger) BlockCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.blocks)
}

// GetBlock returns a block by sequential index.
func (l *AuditLedger) GetBlock(index uint64) (*AuditBlock, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if int(index) >= len(l.blocks) {
		return nil, fmt.Errorf("block index %d out of range (count=%d)", index, len(l.blocks))
	}
	blk := l.blocks[index]
	return &blk, nil
}

// ComputeEntryHash calculates deterministic SHA-256 leaf digest for an audit entry.
func ComputeEntryHash(entry AuditEntry) string {
	raw := fmt.Sprintf("%s:%s:%s:%s", entry.EntryID, entry.ActorID, entry.Action, entry.Payload)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ComputeBlockHash calculates deterministic SHA-256 digest over block header.
func ComputeBlockHash(block AuditBlock) string {
	raw := fmt.Sprintf("%d:%s:%s:%d", block.Index, block.PreviousHash, block.MerkleRoot, block.Timestamp.UnixNano())
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// BuildMerkleTree constructs a balanced binary Merkle tree over the entries, returning root hash and tree levels.
func BuildMerkleTree(entries []AuditEntry) (string, [][]string) {
	if len(entries) == 0 {
		empty := sha256.Sum256([]byte("empty_tree"))
		return hex.EncodeToString(empty[:]), [][]string{}
	}

	leaves := make([]string, len(entries))
	for i, e := range entries {
		leaves[i] = ComputeEntryHash(e)
	}

	levels := [][]string{leaves}
	current := leaves

	for len(current) > 1 {
		var next []string
		for i := 0; i < len(current); i += 2 {
			left := current[i]
			right := left
			if i+1 < len(current) {
				right = current[i+1]
			}
			combined := sha256.Sum256([]byte(left + right))
			next = append(next, hex.EncodeToString(combined[:]))
		}
		levels = append(levels, next)
		current = next
	}

	return current[0], levels
}

// AppendBlock validates entries, calculates the Merkle root, chains to the previous block, and commits.
func (l *AuditLedger) AppendBlock(entries []AuditEntry) (*AuditBlock, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("cannot append empty block without audit entries")
	}

	for i, e := range entries {
		if err := strutil.ErrorOnBlank(e.EntryID, "entry identifier at index %d cannot be blank", i); err != nil {
			return nil, err
		}
		if err := strutil.ErrorOnBlank(e.ActorID, "actor identifier at index %d cannot be blank", i); err != nil {
			return nil, err
		}
		if err := strutil.ErrorOnBlank(e.Action, "action at index %d cannot be blank", i); err != nil {
			return nil, err
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var prevHash string
	if len(l.blocks) > 0 {
		prevHash = ComputeBlockHash(l.blocks[len(l.blocks)-1])
	} else {
		genesis := sha256.Sum256([]byte("zqk:genesis:audit_block:v0"))
		prevHash = hex.EncodeToString(genesis[:])
	}

	merkleRoot, _ := BuildMerkleTree(entries)
	newBlock := AuditBlock{
		Index:        uint64(len(l.blocks)),
		PreviousHash: prevHash,
		MerkleRoot:   merkleRoot,
		Entries:      entries,
		Timestamp:    time.Now().UTC(),
	}

	l.blocks = append(l.blocks, newBlock)
	return &newBlock, nil
}

// GenerateInclusionProof produces an inclusion proof for a target entry index in a committed block.
func (l *AuditLedger) GenerateInclusionProof(blockIndex uint64, entryIndex int) (*InclusionProof, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if int(blockIndex) >= len(l.blocks) {
		return nil, fmt.Errorf("block index %d not found in ledger", blockIndex)
	}

	block := l.blocks[blockIndex]
	if entryIndex < 0 || entryIndex >= len(block.Entries) {
		return nil, fmt.Errorf("entry index %d out of bounds (len=%d)", entryIndex, len(block.Entries))
	}

	merkleRoot, levels := BuildMerkleTree(block.Entries)
	targetHash := ComputeEntryHash(block.Entries[entryIndex])
	var path []AuditPathElement

	idx := entryIndex
	for lvl := 0; lvl < len(levels)-1; lvl++ {
		levelNodes := levels[lvl]
		isRight := (idx % 2) != 0
		var siblingIdx int
		if isRight {
			siblingIdx = idx - 1
		} else {
			siblingIdx = idx + 1
			if siblingIdx >= len(levelNodes) {
				siblingIdx = idx // duplicated leaf
			}
		}

		path = append(path, AuditPathElement{
			Hash:    levelNodes[siblingIdx],
			IsRight: !isRight, // sibling is to the right if target is to the left
		})

		idx = idx / 2
	}

	return &InclusionProof{
		BlockIndex: blockIndex,
		EntryHash:  targetHash,
		MerkleRoot: merkleRoot,
		AuditPath:  path,
	}, nil
}

// VerifyInclusionProof verifies that the entry hash resolves to the expected Merkle root.
func VerifyInclusionProof(proof InclusionProof) (bool, error) {
	if err := strutil.ErrorOnBlank(proof.EntryHash, "entry hash cannot be blank"); err != nil {
		return false, err
	}
	if err := strutil.ErrorOnBlank(proof.MerkleRoot, "merkle root cannot be blank"); err != nil {
		return false, err
	}

	currentHash := proof.EntryHash
	for _, element := range proof.AuditPath {
		if err := strutil.ErrorOnBlank(element.Hash, "path element hash cannot be blank"); err != nil {
			return false, err
		}

		var combined [32]byte
		if element.IsRight {
			combined = sha256.Sum256([]byte(currentHash + element.Hash))
		} else {
			combined = sha256.Sum256([]byte(element.Hash + currentHash))
		}
		currentHash = hex.EncodeToString(combined[:])
	}

	if currentHash != proof.MerkleRoot {
		return false, fmt.Errorf("merkle proof verification failed: computed %s, expected %s", currentHash, proof.MerkleRoot)
	}

	return true, nil
}
