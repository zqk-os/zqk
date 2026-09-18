package app

import (
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// ===========================================================================
// Edge case: vocabulary scheme is case-insensitive per implementation
// ===========================================================================

func TestCheckDraftSweepEntitlement_VocabSchemeCaseInsensitive(t *testing.T) {
	secCtx := &pkgctx.SecurityContext{
		AccountID:               "ACC-CASE-002",
		Permissions:             []string{},
		ActiveVocabularySchemes: []string{"TPM_ENTITLED"}, // uppercase
	}
	err := CheckDraftSweepEntitlement(secCtx)
	if err != nil {
		t.Fatalf("expected case-insensitive vocab scheme check, got deny: %v", err)
	}
}

// ===========================================================================
// Edge case: permission check is case-sensitive (RBAC policy)
// ===========================================================================

func TestCheckDraftSweepEntitlement_PermCaseSensitive(t *testing.T) {
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "ACC-CASE-001",
		Permissions: []string{"WRITE:TPM-DRAFT-SWEEP"}, // uppercase - should NOT match
	}
	err := CheckDraftSweepEntitlement(secCtx)
	if err == nil {
		t.Fatalf("expected case-sensitive permission check, but got allow")
	}
}

// ===========================================================================
// Edge case: hasPerm exact-match only (no partial/contains)
// ===========================================================================

func TestHasPermExactMatch(t *testing.T) {
	got := hasPerm("write:tpm-draft-sweep", "write:tpm-draft-sweep")
	if !got {
		t.Error("hasPerm should return true for exact match")
	}

	partial := hasPerm("write:tpm-draft-sw", "write:tpm-draft-sweep")
	if partial {
		t.Error("hasPerm should NOT match partial strings")
	}

	caseSensitive := hasPerm("Write:tpm-draft-sweep", "write:tpm-draft-sweep")
	if caseSensitive {
		t.Error("hasPerm is case-sensitive - uppercase W should not match lowercase w")
	}
}

// ===========================================================================
// Edge case: peer with empty persona/perm/context still caught
// ===========================================================================

func TestCheckDraftSweepEntitlement_EmptyAccountIDDenied(t *testing.T) {
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "",
		Permissions: []string{},
		PersonaID:   "",
	}
	err := CheckDraftSweepEntitlement(secCtx)
	if err == nil {
		t.Fatal("expected empty account ID without persona to be denied")
	}
}

// ===========================================================================
// Edge case: IsDraftSweepCommand prefix detection
// ===========================================================================

func TestIsDraftSweepCommand_PrefixDetection(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"tpm-draft-list", true},
		{"tpm-draft-delete", true},
		{"tpm-draft-batch", true},
		{"tpsweeper", false},
	}
	for _, tc := range tests {
		got := IsDraftSweepCommand(tc.cmd)
		if got != tc.want {
			t.Errorf("IsDraftSweepCommand(%q) = %v; want %v", tc.cmd, got, tc.want)
		}
	}
}

// ===========================================================================
// Edge case: RejectionMessage names the missing capability, not an object ID
// ===========================================================================

func TestRejectionMessage_ReferencesRequirement(t *testing.T) {
	msg := RejectionMessage("ACC-VICTIM-001")
	mustContains := []string{
		"ACC-VICTIM-001",
		"draft-sweep",
		"RBAC",
		"denied",
		PermDraftSweep,
	}
	for _, field := range mustContains {
		if !strings.Contains(msg, field) {
			t.Errorf("rejection message missing required field: %s", field)
		}
	}
	// Object IDs must not leak into user-facing errors: kernel objects can be archived or
	// deleted, leaving a dangling reference in the message.
	if strings.Contains(msg, "CRIT-") || strings.Contains(msg, "REQ-") {
		t.Errorf("rejection message should not cite kernel object IDs, got: %s", msg)
	}
}

// Fixed: PersonaID updated from old typo "draf_" to corrected "draft_" spelling
func TestCheckDraftSweepEntitlement_PersonaSubstringMatch(t *testing.T) {
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "ACC-SUB-001",
		Permissions: []string{},
		PersonaID:   "PER-draft_sweep_entitled-and-more", // corrected from old typo
	}
	err := CheckDraftSweepEntitlement(secCtx)
	if err != nil {
		t.Fatalf("expected persona substring match to allow, got deny: %v", err)
	}
}
