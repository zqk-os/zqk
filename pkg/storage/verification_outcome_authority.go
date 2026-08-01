package storage

import (
	"os"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// criteriaVerificationOutcomeStatuses are criteria lifecycle statuses that represent
// verification outcomes and must only be written with system security context (unless bypass env is set).
const criteriaStatusComplete = "complete"

func verificationOutcomeAuthorityBypassed() bool {
	v, ok := os.LookupEnv(zqkenv.BypassVerificationOutcomeAuthority())
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

// isCriteriaVerificationOutcomeStatus reports whether s is a verification-outcome status for criteria
// (after normalization). Accepts common casing variants before lifecycle normalization.
func isCriteriaVerificationOutcomeStatus(s string) bool {
	if s == "" {
		return false
	}
	ls := strings.ToLower(strings.TrimSpace(s))
	return ls == objects.ObjectStatusValidated || ls == criteriaStatusComplete
}

// checkCriteriaVerificationOutcomeAuthority enforces that only the system account (or test bypass)
// may set criteria status to validated or complete.
func checkCriteriaVerificationOutcomeAuthority(kind string, secCtx *pkgctx.SecurityContext, newStatus string) error {
	if kind != objects.KindCriteria || !isCriteriaVerificationOutcomeStatus(newStatus) {
		return nil
	}
	if secCtx != nil && secCtx.AccountID == pkgctx.SystemAccountID {
		return nil
	}
	if verificationOutcomeAuthorityBypassed() {
		return nil
	}
	return errfmt.Errorf(
		ConstMiscPermissionDeniedOnlySystemMaySetCriteria,
		strings.ToLower(strings.TrimSpace(newStatus)),
	)
}

// checkConvergenceSessionErrorStatusAuthority enforces that only the system account (or test bypass)
// may set convergence_session status to error (lifecycle marks this status as system-managed).
func checkConvergenceSessionErrorStatusAuthority(kind string, secCtx *pkgctx.SecurityContext, newStatus string) error {
	if kind != objects.KindConvergenceSession {
		return nil
	}
	ls := strings.ToLower(strings.TrimSpace(newStatus))
	if ls != objects.ObjectStatusError {
		return nil
	}
	if secCtx != nil && secCtx.AccountID == pkgctx.SystemAccountID {
		return nil
	}
	if verificationOutcomeAuthorityBypassed() {
		return nil
	}
	return errfmt.Errorf(
		ConstMiscPermissionDeniedOnlySystemMaySetConverge,
		ls,
	)
}

// checkVerificationOutcomeAuthority runs criteria outcome checks and convergence_session system-status checks.
func checkVerificationOutcomeAuthority(kind string, secCtx *pkgctx.SecurityContext, newStatus string) error {
	if err := checkCriteriaVerificationOutcomeAuthority(kind, secCtx, newStatus); err != nil {
		return err
	}
	return checkConvergenceSessionErrorStatusAuthority(kind, secCtx, newStatus)
}
