package evolution

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
)

func TestSkillBundle_BundleAndVerify(t *testing.T) {
	signer, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}

	skillData := []byte(`{"id": "skill-123", "instructions": "do something"}`)

	bundle, err := BundleSkill(skillData, signer)
	if err != nil {
		t.Fatalf("failed to bundle skill: %v", err)
	}

	err = bundle.Verify()
	if err != nil {
		t.Fatalf("failed to verify skill bundle: %v", err)
	}

	// Tamper with data
	bundle.SkillData = []byte(`{"id": "skill-123", "instructions": "do something malicious"}`)
	err = bundle.Verify()
	if err == nil {
		t.Fatalf("expected verification to fail after data tampering")
	}
}
