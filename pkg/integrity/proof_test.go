package integrity

import (
	"crypto/sha256"
	"errors"
	"testing"
)

func TestSealProof_VerifiesMatchingContent(t *testing.T) {
	content := []byte("test-skill-content")
	h := sha256.Sum256(content)
	sp := NewSealProof("ASK-TEST-001", h[:])

	if err := sp.Verify(content); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !sp.Verified {
		t.Fatal("expected Verified to be true after passing match")
	}
}

func TestSealProof_FailsOnMismatchedContent(t *testing.T) {
	content := []byte("test-skill-content")
	h := sha256.Sum256(content)
	sp := NewSealProof("ASK-TEST-001", h[:])

	err := sp.Verify([]byte("wrong-content"))
	if err == nil {
		t.Fatal("expected failure on content mismatch, got nil")
	}
	if !errors.Is(err, nil) {
		_ = err // we only care it isn't nil
	}
	if sp.Verified {
		t.Fatal("expected Verified to be false after failed match")
	}
}

func TestNewVerifiedContentProof(t *testing.T) {
	p := NewVerifiedContentProof("ASK-TEST-002", "abc123def456")
	if p.SkillID != "ASK-TEST-002" {
		t.Fatalf("expected SkillID ASK-TEST-002, got %s", p.SkillID)
	}
	if p.IntegrityPass != true {
		t.Fatal("expected IntegrityPass to be true by default")
	}
}

func TestIntegrityCheck_CheckSeal(t *testing.T) {
	ic := IntegrityCheck{
		SkillID:     "ASK-TEST-003",
		ExpectedSHA: "deadbeef",
	}
	if err := ic.CheckSeal("deadbeef"); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(ic.Violations) != 0 {
		t.Fatal("expected no violations on matching seal")
	}
}

func TestIntegrityCheck_CheckSeal_FailsMismatch(t *testing.T) {
	ic := IntegrityCheck{
		SkillID:     "ASK-TEST-003",
		ExpectedSHA: "deadbeef",
	}
	err := ic.CheckSeal("abcdef12")
	if err == nil {
		t.Fatal("expected failure on mismatched seal")
	}
	if len(ic.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(ic.Violations))
	}
}

func TestIntegrityCheck_CheckCount_NoViolations(t *testing.T) {
	ic := IntegrityCheck{SkillID: "ASK-TEST-004"}
	if err := ic.CheckCount(); err != nil {
		t.Fatalf("expected success with no violations, got: %v", err)
	}
}

func TestIntegrityCheck_CheckCount_HasViolations(t *testing.T) {
	ic := IntegrityCheck{SkillID: "ASK-TEST-005"}
	ic.Violations = append(ic.Violations, "violation 1")
	if err := ic.CheckCount(); err == nil {
		t.Fatal("expected failure when violations present")
	}
}

func TestProveResult_Aggregation(t *testing.T) {
	r := ProveResult{SkillsProved: 2, SkillsFailed: 0}
	if r.SkillsProved != 2 {
		t.Fatalf("expected 2 proved, got %d", r.SkillsProved)
	}
	if r.SkillsFailed != 0 {
		t.Fatalf("expected 0 failed, got %d", r.SkillsFailed)
	}
}
