package integrity

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ErrContentMismatch indicates that the computed content hash does not match expected hash.
var ErrContentMismatch = errors.New("integrity: content SHA256 mismatch")

// ErrSealMismatch indicates that the seal hash does not match expected hash.
var ErrSealMismatch = errors.New("integrity: seal SHA256 mismatch")

// ErrIntegrityViolations indicates that one or more integrity violations were recorded.
var ErrIntegrityViolations = errors.New("integrity: check failed due to recorded violations")

// SealProof holds an expected SHA256 digest for an artifact and verifies content.
type SealProof struct {
	SkillID     string
	ExpectedSHA []byte
	Verified    bool
}

// NewSealProof creates a new SealProof for the given skill ID and expected digest.
func NewSealProof(skillID string, expectedSHA []byte) *SealProof {
	return &SealProof{
		SkillID:     skillID,
		ExpectedSHA: expectedSHA,
		Verified:    false,
	}
}

// Verify computes the SHA256 hash of content and compares it to ExpectedSHA.
func (sp *SealProof) Verify(content []byte) error {
	actual := sha256.Sum256(content)
	if !bytes.Equal(actual[:], sp.ExpectedSHA) {
		sp.Verified = false
		return ErrContentMismatch
	}
	sp.Verified = true
	return nil
}

// VerifiedContentProof represents an asserted content verification proof.
type VerifiedContentProof struct {
	SkillID       string
	SHA           string
	IntegrityPass bool
}

// NewVerifiedContentProof creates a proof with IntegrityPass=true by default.
func NewVerifiedContentProof(skillID, sha string) *VerifiedContentProof {
	return &VerifiedContentProof{
		SkillID:       skillID,
		SHA:           sha,
		IntegrityPass: true,
	}
}

// IntegrityCheck tracks verification operations and recorded violations.
type IntegrityCheck struct {
	SkillID     string
	ExpectedSHA string
	Violations  []string
}

// CheckSeal compares the actual SHA with the expected SHA.
func (ic *IntegrityCheck) CheckSeal(actualSHA string) error {
	if actualSHA != ic.ExpectedSHA {
		msg := fmt.Sprintf("seal mismatch: expected %s, got %s", ic.ExpectedSHA, actualSHA)
		ic.Violations = append(ic.Violations, msg)
		return ErrSealMismatch
	}
	return nil
}

// CheckCount returns an error if any violations have been recorded.
func (ic *IntegrityCheck) CheckCount() error {
	if len(ic.Violations) > 0 {
		return ErrIntegrityViolations
	}
	return nil
}

// ProveResult summarizes verification outcomes across a batch of skills.
type ProveResult struct {
	SkillsProved int
	SkillsFailed int
}
