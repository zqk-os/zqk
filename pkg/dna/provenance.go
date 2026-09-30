package dna

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)


// Actor returns the canonical agent or actor identifier, preferring AgentURN if populated.
func (p Provenance) Actor() string {
	if !p.AgentURN.IsZero() {
		return p.AgentURN.String()
	}
	return p.AgentID
}

// Attest signs/records an attestation for a payload by an agent or actor.
// agentRef may be a URN, *URN, or string identifier. If a string is given that parses
// as a valid URN, AgentURN will be populated as well.
func (p *Provenance) Attest(agentRef any, payload []byte) {
	switch a := agentRef.(type) {
	case URN:
		p.AgentURN = a
		p.AgentID = a.ID
	case *URN:
		if a != nil {
			p.AgentURN = *a
			p.AgentID = a.ID
		}
	case string:
		trimmed := strings.TrimSpace(a)
		if u, err := ParseURN(trimmed); err == nil {
			p.AgentURN = u
			p.AgentID = u.ID
		} else {
			p.AgentID = trimmed
		}
	}

	hash := sha256.Sum256(payload)
	p.Hash = hex.EncodeToString(hash[:])
	p.Signature = "attested:" + p.Hash
}

// ComputeDigest calculates a deterministic SHA-256 hash of the provenance block.
func (p Provenance) ComputeDigest() string {
	payload, _ := json.Marshal(p)
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

// GetProvenance returns the embedded provenance vector.
func (a *Auditable) GetProvenance() Provenance {
	return a.Provenance
}

// ComputeDigest returns the cryptographic digest of the embedded provenance.
func (a *Auditable) ComputeDigest() (string, error) {
	return a.Provenance.ComputeDigest(), nil
}
