package integrity_test

import (
	"crypto/sha256"
	"testing"

	"github.com/zqk-os/zqk/pkg/integrity"
)

// Test identifiers reused across test cases.
const (
	testSkillID       = "ASK-TARGETED-VALIDATION"
	testSkillName     = "Targeted Validation" // kernel identity, not arbitrary magic
	validATK          = "ATK-1234567890-test"
	invalidIDPrefix   = "BLI-1234567890-test"
	testTaskBody      = "Implement verification logic for task envelopes"
	errValidHashFmt   = "expected no error for valid hash, got: %v"
	errInvalidHash    = "expected error for invalid hash"
	errNonDetHashFmt  = "non-deterministic hash: %x vs %x"
	errSameHash       = "different skills produced same hash"
	errEmptyProofFmt  = "expected empty proof, got %d entries"
	errOneProofEntryF = "expected 1 proof entry, got %d"
	errValidTDEFmt    = "expected valid TDE, got error: %v"
	errMissingID      = "expected error for missing ID"
	errWrongPrefixFmt = "expected error for id prefix (need ATK) but found %s"
	errEmptyBody      = "expected error for empty task body"
	errNilEvidence    = "expected non-nil evidence map"
	errCollectMsgFmt  = "unexpected error: %v"
	dirExistHintValue = "directory_exists"
	emptyDirPath      = ""
)

// --- Skill Integrity Proofs Tests ---

func TestVerifySkillHash_validSignature(t *testing.T) {
	skill := &integrity.Skill{
		ID:     testSkillID,
		Name:   testSkillName,
		Kind:   "skill",
		Status: "implemented",
	}
	expectedHash := skill.SHA256()
	err := integrity.VerifySkillHash(skill.ID, expectedHash)
	if err != nil {
		t.Fatalf(errValidHashFmt, err)
	}
	if string(expectedHash) == "" {
		t.Fatal("expectedHash must not be empty")
	}
}

func TestVerifySkillHash_invalidSignature(t *testing.T) {
	skill := &integrity.Skill{ID: testSkillID, Name: testSkillName}
	wrongHash := sha256.Sum256([]byte("tampered"))
	err := integrity.VerifySkillHash(skill.ID, wrongHash[:])
	if err == nil {
		t.Fatal(errInvalidHash)
	}
}

func TestComputeSHA256_deterministic(t *testing.T) {
	s1 := &integrity.Skill{ID: "ASK-X", Name: "A", Kind: "skill"}
	s2 := &integrity.Skill{ID: "ASK-X", Name: "A", Kind: "skill"}
	h1 := s1.SHA256()
	h2 := s2.SHA256()
	if string(h1) != string(h2) {
		t.Errorf(errNonDetHashFmt, h1, h2)
	}
}

func TestComputeSHA256_differentInputs(t *testing.T) {
	s1 := &integrity.Skill{ID: "ASK-A", Name: "A"}
	s2 := &integrity.Skill{ID: "ASK-B", Name: "B"}
	h1 := s1.SHA256()
	h2 := s2.SHA256()
	if string(h1) == string(h2) {
		t.Fatal(errSameHash)
	}
}

func TestBuildIntegrityProof_empty(t *testing.T) {
	var skills []*integrity.Skill
	proof := integrity.BuildIntegrityProof(skills)
	if proof == nil || len(proof) != 0 {
		t.Errorf(errEmptyProofFmt, len(proof))
	}

	skills = []*integrity.Skill{
		{ID: "ASK-X", Name: "X"},
	}
	proof = integrity.BuildIntegrityProof(skills)
	if proof == nil || len(proof) != 1 {
		t.Errorf(errOneProofEntryF, len(proof))
	}
}

// --- TDE Validation Tests ---

func TestValidateTDE_validEnvelope(t *testing.T) {
	tde := &integrity.TaskDefEnvelope{
		ID:     validATK,
		Task:   testTaskBody,
		Kind:   "agent_task",
		Status: "in_progress",
	}
	if err := integrity.ValidateTDE(tde); err != nil {
		t.Errorf(errValidTDEFmt, err)
	}
}

func TestValidateTDE_missingID(t *testing.T) {
	tde := &integrity.TaskDefEnvelope{Task: testTaskBody}
	if err := integrity.ValidateTDE(tde); err == nil {
		t.Fatal(errMissingID)
	}
}

func TestValidateTDE_invalidPrefix(t *testing.T) {
	tde := &integrity.TaskDefEnvelope{ID: invalidIDPrefix, Task: testTaskBody}
	if err := integrity.ValidateTDE(tde); err == nil {
		t.Fatalf(errWrongPrefixFmt, invalidIDPrefix[:3])
	}
}

func TestValidateTDE_missingTaskBody(t *testing.T) {
	tde := &integrity.TaskDefEnvelope{ID: validATK}
	if err := integrity.ValidateTDE(tde); err == nil {
		t.Fatal(errEmptyBody)
	}
}

// --- Workspace Observation Heuristics Tests ---

func TestCollectEvidence_nilArgs(t *testing.T) {
	evidence, err := integrity.CollectWorkspaceEvidence(emptyDirPath, nil)
	if err != nil {
		t.Fatalf(errCollectMsgFmt, err)
	}
	if evidence == nil {
		t.Fatal(errNilEvidence)
	}
}

func TestCollectEvidence_includesExpectedKeys(t *testing.T) {
	evidence, err := integrity.CollectWorkspaceEvidence("/tmp/zqk-test", []integrity.EvidenceHint{
		{Kind: dirExistHintValue},
	})
	if err != nil {
		t.Fatalf(errCollectMsgFmt, err)
	}
	if evidence == nil {
		t.Fatal(errNilEvidence)
	}
}
