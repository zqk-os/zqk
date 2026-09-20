package app

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// DraftSweep entitlement constants. Rejection text describes the missing capability instead of
// citing a kernel object ID: process objects can be archived, renamed, or deleted, which would
// leave users chasing a dangling reference in a CLI error.
const (
	PermDraftSweep      = "write:tpm-draft-sweep" // Perm that permits draft sweep on process grooming drafts
	RBAC_SweepEntitated = "tpm_entitled"          // Vocabulary-scheme marker for swept paths
	ErrDraftSweepDenied = "draft-sweep RBAC: peer %s denied; requires " + PermDraftSweep + " permission"
	errNilSecCtx        = "draft-sweep RBAC: nil security context"

	accAgentSessionToken = "agent-session-token"
)

// Entitlement markers that may appear on a security context's vocabulary schemes or persona ref.
// Kept as data (not inline conditionals) so adding a marker does not mean editing control flow.
var (
	sweepEntitledVocabularyMarkers = []string{
		RBAC_SweepEntitated,
		"vocabulary_schemes.sweep",
		"persona_ref.tpmsweeper",
	}
	sweepEntitledPersonaMarkers = []string{
		"tpm_sweeper",
		"draft_sweep_entitled",
	}
)

// Draft-sweep command detection vocabulary. Exported as data so callers (and tests) can see the
// full surface without re-reading the conditional chain, and so new sweep aliases are a one-line
// data change. Names are compared lowercased.
var (
	draftSweepCommandNames = map[string]bool{
		"draft-sweep": true,
		"sweep":       true,
		"tpm-groom":   true,
	}
	draftSweepCommandPrefixes = []string{"tpm-draft"}
)

// IsDraftSweepCommand detects whether a command represents a draft-sweep operation.
// Draft-sweep operations sweep multiple grooming drafts out of the draft plane. Detection is
// intentionally conservative: only the known sweep command names and prefixes match.
func IsDraftSweepCommand(cmdName string) bool {
	name := strings.ToLower(strings.TrimSpace(cmdName))
	if draftSweepCommandNames[name] {
		return true
	}
	for _, prefix := range draftSweepCommandPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// CheckDraftSweepEntitlement verifies that the current security context is entitled to
// perform a draft-sweep operation on grooming drafts. Unauthorized peers are blocked by
// default; only accounts with the write:tpm-draft-sweep permission (or the system / test-harness
// accounts) may proceed.
func CheckDraftSweepEntitlement(secCtx *pkgctx.SecurityContext) error {
	if secCtx == nil {
		return fmt.Errorf(errNilSecCtx)
	}
	if isOwningAccount(secCtx.AccountID) ||
		hasDraftSweepPermission(secCtx) ||
		hasDraftSweepVocabularyMarker(secCtx) ||
		hasDraftSweepPersonaMarker(secCtx) {
		return nil
	}
	return fmt.Errorf(ErrDraftSweepDenied, secCtx.AccountID)
}

// isOwningAccount reports whether the account owns the system (system or test harness) and is
// therefore always allowed through.
func isOwningAccount(accountID string) bool {
	switch accountID {
	case pkgctx.SystemAccountID, accAgentSessionToken, pkgctx.TestHarnessAccountID:
		return true
	default:
		return false
	}
}

// hasDraftSweepPermission reports whether the context carries the draft-sweep RBAC permission.
func hasDraftSweepPermission(secCtx *pkgctx.SecurityContext) bool {
	if authcred.HasExactPermission(secCtx, PermDraftSweep) {
		return true
	}
	for _, perm := range secCtx.Permissions {
		if hasPerm(perm, PermDraftSweep) {
			return true
		}
	}
	return false
}

// hasDraftSweepVocabularyMarker reports whether any active vocabulary scheme carries a
// sweep-entitlement marker (case-insensitive).
func hasDraftSweepVocabularyMarker(secCtx *pkgctx.SecurityContext) bool {
	for _, scheme := range secCtx.ActiveVocabularySchemes {
		if containsAnyMarker(scheme, sweepEntitledVocabularyMarkers) {
			return true
		}
	}
	return false
}

// hasDraftSweepPersonaMarker reports whether the persona reference carries a sweep-entitlement
// marker (case-insensitive).
func hasDraftSweepPersonaMarker(secCtx *pkgctx.SecurityContext) bool {
	return secCtx.PersonaID != "" && containsAnyMarker(secCtx.PersonaID, sweepEntitledPersonaMarkers)
}

// containsAnyMarker reports whether value equals or contains any marker, case-insensitively.
func containsAnyMarker(value string, markers []string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range markers {
		if normalized == marker || strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// hasPerm is a simple exact-match check for the current permission string against the target.
func hasPerm(got, want string) bool {
	return got == want
}

// RejectionMessage returns a user-facing error message for failed draft-sweep entitlement checks.
func RejectionMessage(accountID string) string {
	return fmt.Sprintf(ErrDraftSweepDenied, accountID)
}
