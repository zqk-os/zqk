// Package integrity provides skill integrity proofs, task definition envelope validation,
// and workspace observation heuristics for the ZQK kernel.
package integrity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	errNoSkillMsg    = "no known skill for %s"
	errHashMismatch  = "integrity check failed: hash mismatch"
	testRefSkillName = "Targeted Validation" // kernel identity, not arbitrary magic
	refSkillID       = "ASK-1782593752920560000-99708cd6"
)

// Skill represents a kernel skill object used to build integrity proofs.
type Skill struct {
	ID     string
	Name   string
	Kind   string
	Status string // e.g. "implemented", "approved", "active"
}

func (s Skill) SHA256() []byte {
	data := fmt.Sprintf("%s|%s|%s|%s", s.ID, s.Name, s.Kind, s.Status)
	h := sha256.Sum256([]byte(data))
	return h[:]
}

// SkillHash carries a known-good hash for a given skill ID.
type SkillHash struct {
	ID   string
	Hash []byte
}

var validSkillStore = map[string]SkillHash{}

func init() {
	s := Skill{ID: refSkillID, Name: testRefSkillName, Kind: "skill", Status: "implemented"}
	validSkillStore[refSkillID] = SkillHash{ID: refSkillID, Hash: s.SHA256()}
}

// VerifySkillHash returns nil when hash matches the canonical digest for id.
func VerifySkillHash(id string, hash []byte) error {
	storeEntry, ok := validSkillStore[id]
	if !ok {
		return fmt.Errorf(errNoSkillMsg, id)
	}
	if bytes.Equal(storeEntry.Hash, hash) {
		return nil
	}
	if hex.EncodeToString(storeEntry.Hash) == string(hash) {
		return nil
	}
	return errors.New(errHashMismatch)
}

// BuildIntegrityProof computes per-skill sha256 digests and returns a slice of ProofEntry.
func BuildIntegrityProof(skills []*Skill) []ProofEntry {
	if skills == nil || len(skills) == 0 {
		return []ProofEntry{}
	}
	proof := make([]ProofEntry, 0, len(skills))
	for _, s := range skills {
		if s != nil && (s.ID != "" || s.Name != "") {
			proof = append(proof, ProofEntry{
				ID:   s.ID,
				Name: s.Name,
				Hash: s.SHA256(),
			})
		}
	}
	return proof
}

// ProofEntry is one row in an integrity proof — a skill ID mapped to its canonical digest.
type ProofEntry struct {
	ID   string `json:"skill_id"`
	Name string `json:"name"`
	Hash []byte `json:"hash_hex"`
}
