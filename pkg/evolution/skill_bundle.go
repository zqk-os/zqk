package evolution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
)

// SkillBundle represents a Merkle-proofed package for an agent skill.
type SkillBundle struct {
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
	SkillData []byte `json:"skill_data"`
}

// BundleSkill cryptographically signs and packages skill data into a verifiable bundle.
func BundleSkill(skillData []byte, signer crypto.Signer) (*SkillBundle, error) {
	hashBytes := sha256.Sum256(skillData)
	hash := hex.EncodeToString(hashBytes[:])

	sig, err := signer.Sign(hashBytes[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign skill: %v", err)
	}

	return &SkillBundle{
		Hash:      hash,
		Signature: sig,
		PublicKey: signer.PublicKey(),
		SkillData: skillData,
	}, nil
}

// Verify ensures the skill bundle's hash and signature are valid.
func (b *SkillBundle) Verify() error {
	// 1. Verify Hash
	hashBytes := sha256.Sum256(b.SkillData)
	computedHash := hex.EncodeToString(hashBytes[:])
	if computedHash != b.Hash {
		return fmt.Errorf("skill data hash mismatch: expected %s, got %s", b.Hash, computedHash)
	}

	// 2. Verify Signature
	valid, err := crypto.GlobalVerifier.Verify(hashBytes[:], b.Signature, b.PublicKey)
	if err != nil {
		return fmt.Errorf("signature verification failed: %v", err)
	}
	if !valid {
		return fmt.Errorf("invalid skill signature")
	}

	return nil
}
